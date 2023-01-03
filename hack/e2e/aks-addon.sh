#!/usr/bin/env bash

# To test a latest version of the code, you need to build+push the image somewhere.
# Then you can set `IMG_URL`, `IMG_REPO`, and `IMG_TAG` accordingly

set -e -u -x

if ! type helm > /dev/null; then
  exit 1
fi

if ! type go > /dev/null; then
  exit 1
fi

: "${RANDOM_NUM:=${PR_RAND}}"

if [ "$PR_RAND" = "" ]; then
    RANDOM_NUM=$RANDOM
fi

: "${RESOURCE_GROUP:=aks-addon-aci-test-$RANDOM_NUM}"
: "${LOCATION:=eastus2}"
: "${CLUSTER_NAME:=${RESOURCE_GROUP}}"
: "${NODE_COUNT:=1}"
: "${CHART_NAME:=aks-addon--test}"
: "${WIN_CHART_NAME:=vk-aci-test-win-aks}"
: "${TEST_NODE_NAME:=vk-aci-test-aks}"
: "${TEST_WINDOWS_NODE_NAME:=vk-aci-test-win-aks}"
: "${IMG_REPO:=oss/virtual-kubelet/virtual-kubelet}"
: "${IMG_URL:=mcr.microsoft.com}"
: "${VNET_RANGE=10.0.0.0/8}"
: "${CLUSTER_SUBNET_CIDR=10.240.0.0/16}"
: "${ACI_SUBNET_CIDR=10.241.0.0/16}"
: "${VNET_NAME=aksAddonVN}"
: "${CLUSTER_SUBNET_NAME=aksAddonsubnet}"
: "${ACI_SUBNET_NAME=acisubnet}"
: "${ACR_NAME=aksaddonacr$RANDOM_NUM}"
: "${CSI_DRIVER_STORAGE_ACCOUNT_NAME=aksaddonvk$RANDOM_NUM}"
: "${CSI_DRIVER_SHARE_NAME=vncsidriversharename}"

error() {
    echo "$@" >&2
    exit 1
}

if [ ! -v IMG_TAG ] || [ -z "$IMG_TAG" ]; then
    IMG_TAG="$(git describe --abbrev=0 --tags)"
    IMG_TAG="${IMG_TAG#v}"
fi

TMPDIR=""

cleanup() {
  az group delete --name "$RESOURCE_GROUP" --yes --no-wait || true
  if [ -n "$TMPDIR" ]; then
      rm -rf "$TMPDIR"
  fi
}
trap 'cleanup' EXIT


check_aci_registered() {
    az provider list --query "[?contains(namespace,'Microsoft.ContainerInstance')]" -o json | jq -r '.[0].registrationState'
}

if [ ! "$(check_aci_registered)" = "Registered" ]; then
    az provider register --namespace Microsoft.ContainerInstance
    while [ ! "$(check_aci_registered)" = "Registered" ]; do
        echo "Waiting for ACI to be registered..."
        sleep 5
    done
fi

az group create --name "$RESOURCE_GROUP" --location "$LOCATION"

if [ "$E2E_TARGET" = "pr" ]; then
  az acr create --resource-group "$RESOURCE_GROUP" \
    --name "$ACR_NAME" --sku Basic

  az acr login --name "$ACR_NAME"
  IMG_URL=$ACR_NAME.azurecr.io
  IMG_REPO="virtual-kubelet"
  OUTPUT_TYPE=type=registry IMG_TAG=$IMG_TAG  IMAGE=$ACR_NAME.azurecr.io/$IMG_REPO make docker-build-image
fi

TMPDIR="$(mktemp -d)"

az network vnet create \
    --resource-group $RESOURCE_GROUP \
    --name $VNET_NAME \
    --address-prefixes $VNET_RANGE \
    --subnet-name $CLUSTER_SUBNET_NAME \
    --subnet-prefix $CLUSTER_SUBNET_CIDR

aci_subnet_id="$(az network vnet subnet create \
    --resource-group $RESOURCE_GROUP \
    --vnet-name $VNET_NAME \
    --name $ACI_SUBNET_NAME \
    --address-prefix $ACI_SUBNET_CIDR \
    --query id -o tsv)"

cluster_subnet_id="$(az network vnet subnet show \
    --resource-group $RESOURCE_GROUP \
    --vnet-name $VNET_NAME \
    --name $CLUSTER_SUBNET_NAME \
    --query id -o tsv)"

if [ "$E2E_TARGET" = "pr" ]; then
az aks create \
    -g "$RESOURCE_GROUP" \
    -l "$LOCATION" \
    -c "$NODE_COUNT" \
    --node-vm-size standard_d8_v3 \
    -n "$CLUSTER_NAME" \
    --network-plugin azure \
    --vnet-subnet-id "$cluster_subnet_id" \
    --enable-addons virtual-node \
    --aci-subnet-name "$ACI_SUBNET_NAME" \
    --generate-ssh-keys \
    --attach-acr "$ACR_NAME"
else

az aks create \
    -g "$RESOURCE_GROUP" \
    -l "$LOCATION" \
    -c "$NODE_COUNT" \
    --node-vm-size standard_d8_v3 \
    -n "$CLUSTER_NAME" \
    --network-plugin azure \
    --vnet-subnet-id "$cluster_subnet_id" \
    --enable-addons virtual-node \
    --aci-subnet-name "$ACI_SUBNET_NAME" \
    --generate-ssh-keys \

fi

az aks get-credentials -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" -f "$TMPDIR/kubeconfig"
export KUBECONFIG="$TMPDIR/kubeconfig"

MASTER_URI="$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")"

ACI_USER_IDENTITY="$(az aks show  -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" --query addonProfiles.aciConnectorLinux.identity.clientId -o tsv)"
KUBE_DNS_IP="$(az aks show  -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" --query networkProfile.dnsServiceIp -o tsv)"
CLUSTER_RESOURCE_ID="$(az aks show  -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" --query "id" -o tsv)"
MC_RESOURCE_GROUP="$(az aks show  -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" --query "nodeResourceGroup" -o tsv)"
SUB_ID="$(az account show --query "id" -o tsv)"

kubectl create configmap test-vars -n kube-system \
  --from-literal=master_uri="$MASTER_URI" \
  --from-literal=aci_user_identity="$ACI_USER_IDENTITY" \
  --from-literal=kube_dns_ip="$KUBE_DNS_IP" \
  --from-literal=cluster_subnet_cidr="$CLUSTER_SUBNET_CIDR" \
  --from-literal=aci_subnet_name="$ACI_SUBNET_NAME"

sed -e "s|TEST_IMAGE|$ACR_NAME.azurecr.io/$IMG_REPO:$IMG_TAG|g" deploy/deployment.yaml | kubectl apply -n kube-system -f -

kubectl wait --for=condition=available deploy "virtual-kubelet-azure-aci" -n kube-system --timeout=300s

while true; do
    kubectl get node "$TEST_NODE_NAME" &> /dev/null && break
    sleep 3
done

kubectl wait --for=condition=Ready --timeout=300s node "$TEST_NODE_NAME"

export TEST_NODE_NAME

## Windows VK
helm install \
    --kubeconfig="${KUBECONFIG}" \
    --set nodeOsType=Windows \
    --set "image.repository=${IMG_URL}"  \
    --set "image.name=${IMG_REPO}" \
    --set "image.tag=${IMG_TAG}" \
    --set "nodeName=${TEST_WINDOWS_NODE_NAME}" \
    --set "providers.azure.masterUri=$MASTER_URI" \
    --set "providers.azure.managedIdentityID=$ACI_USER_IDENTITY" \
    "$WIN_CHART_NAME" \
    ./charts/virtual-kubelet

kubectl wait --for=condition=available deploy "${TEST_WINDOWS_NODE_NAME}-virtual-kubelet-azure-aci" -n vk-azure-aci --timeout=300s

while true; do
    kubectl get node "$TEST_WINDOWS_NODE_NAME" &> /dev/null && break
    sleep 3
done

kubectl wait --for=condition=Ready --timeout=300s node "$TEST_WINDOWS_NODE_NAME"

export TEST_WINDOWS_NODE_NAME=$TEST_WINDOWS_NODE_NAME

## CSI Driver test
az storage account create -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g $RESOURCE_GROUP -l $LOCATION --sku Standard_LRS
export AZURE_STORAGE_CONNECTION_STRING=$(az storage account show-connection-string -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g "$RESOURCE_GROUP" -o tsv)

az storage share create -n "$CSI_DRIVER_SHARE_NAME" --connection-string "$AZURE_STORAGE_CONNECTION_STRING"
CSI_DRIVER_STORAGE_ACCOUNT_KEY=$(az storage account keys list --resource-group "$RESOURCE_GROUP" --account-name "$CSI_DRIVER_STORAGE_ACCOUNT_NAME" --query "[0].value" -o tsv)

export CSI_DRIVER_STORAGE_ACCOUNT_NAME=$CSI_DRIVER_STORAGE_ACCOUNT_NAME
export CSI_DRIVER_STORAGE_ACCOUNT_KEY=$CSI_DRIVER_STORAGE_ACCOUNT_KEY

$@
