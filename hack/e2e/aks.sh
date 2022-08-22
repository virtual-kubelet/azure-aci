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

: "${RANDOM_NUM:=$RANDOM}"
: "${RESOURCE_GROUP:=vk-aci-test-$RANDOM_NUM}"
: "${LOCATION:=westus2}"
: "${CLUSTER_NAME:=${RESOURCE_GROUP}}"
: "${NODE_COUNT:=1}"
: "${CHART_NAME:=vk-aci-test-aks}"
: "${TEST_NODE_NAME:=vk-aci-test-aks}"
: "${IMG_REPO:=oss/virtual-kubelet/virtual-kubelet}"
: "${IMG_URL:=mcr.microsoft.com}"

: "${VNET_RANGE=10.0.0.0/8}"
: "${CLUSTER_SUBNET_RANGE=10.240.0.0/16}"
: "${ACI_SUBNET_RANGE=10.241.0.0/16}"
: "${VNET_NAME=myAKSVNet}"
: "${CLUSTER_SUBNET_NAME=myAKSSubnet}"
: "${ACI_SUBNET_NAME=myACISubnet}"

: "${CSI_DRIVER_STORAGE_ACCOUNT_NAME=vncsidrivers$RANDOM_NUM}"
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
  az group delete --name "$RESOURCE_GROUP" --yes --no-wait
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


KUBE_DNS_IP=10.0.0.10

az network vnet create \
    --resource-group $RESOURCE_GROUP \
    --name $VNET_NAME \
    --address-prefixes $VNET_RANGE \
    --subnet-name $CLUSTER_SUBNET_NAME \
    --subnet-prefix $CLUSTER_SUBNET_RANGE

aci_subnet_id="$(az network vnet subnet create \
    --resource-group $RESOURCE_GROUP \
    --vnet-name $VNET_NAME \
    --name $ACI_SUBNET_NAME \
    --address-prefix $ACI_SUBNET_RANGE \
    --query id -o tsv)"


vnet_id="$(az network vnet show --resource-group $RESOURCE_GROUP --name $VNET_NAME --query id -o tsv)"
aks_subnet_id="$(az network vnet subnet show --resource-group $RESOURCE_GROUP --vnet-name $VNET_NAME --name $CLUSTER_SUBNET_NAME --query id -o tsv)"

TMPDIR="$(mktemp -d)"
cluster_identity="$(az identity create --name "${RESOURCE_GROUP}-aks-identity" --resource-group "${RESOURCE_GROUP}" --query principalId -o tsv)"
node_identity="$(az identity create --name "${RESOURCE_GROUP}-node-identity" --resource-group "${RESOURCE_GROUP}" --query principalId -o tsv)"

node_identity_id="$(az identity show --name ${RESOURCE_GROUP}-node-identity --resource-group ${RESOURCE_GROUP} --query id -o tsv)"
cluster_identity_id="$(az identity show --name ${RESOURCE_GROUP}-aks-identity --resource-group ${RESOURCE_GROUP} --query id -o tsv)"
az aks create \
    -g "$RESOURCE_GROUP" \
    -l "$LOCATION" \
    -c "$NODE_COUNT" \
    --node-vm-size standard_d8_v3 \
    -n "$CLUSTER_NAME" \
    --network-plugin azure \
    --vnet-subnet-id "$aks_subnet_id" \
    --dns-service-ip "$KUBE_DNS_IP" \
    --assign-kubelet-identity "$node_identity_id" \
    --assign-identity "$cluster_identity_id" \
    --node-vm-size standard_d8_v3 \
    --generate-ssh-keys

az role assignment create \
    --role "Network Contributor" \
    --assignee-object-id "$node_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "$vnet_id"
az role assignment create \
    --role "Network Contributor" \
    --assignee-object-id "$cluster_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "$vnet_id"
az role assignment create \
    --role "Network Contributor" \
    --assignee-object-id "$node_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "$aci_subnet_id"

# Make sure ACI can create containers in the AKS RG.
# Note, this is not wonderful since it gives a lot of permissions to the identity which is also shared with the kubelet (which it doesn't need).
# Unfortunately there is no way to scope this down (AFIACT) currently.
az role assignment create \
    --role "Contributor" \
    --assignee-object-id "$node_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "/subscriptions/$(az account show --query id -o tsv)/resourceGroups/MC_${RESOURCE_GROUP}_${RESOURCE_GROUP}_${LOCATION}"

az aks get-credentials -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" -f "${TMPDIR}/kubeconfig"
export KUBECONFIG="${TMPDIR}/kubeconfig"

MASTER_URI="$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")"

helm install \
    --kubeconfig="${KUBECONFIG}" \
    --set "image.repository=${IMG_URL}"  \
    --set "image.name=${IMG_REPO}" \
    --set "image.tag=${IMG_TAG}" \
    --set "nodeName=${TEST_NODE_NAME}" \
    --set providers.azure.vnet.enabled=true \
    --set "providers.azure.vnet.subnetName=$ACI_SUBNET_NAME" \
    --set "providers.azure.vnet.subnetCidr=$ACI_SUBNET_RANGE" \
    --set "providers.azure.vnet.clusterCidr=$CLUSTER_SUBNET_RANGE" \
    --set "providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP" \
    --set "providers.azure.masterUri=$MASTER_URI" \
    "$CHART_NAME" \
    ./charts/virtual-kubelet

kubectl wait --for=condition=available deploy "${TEST_NODE_NAME}-virtual-kubelet-aci-for-aks" --timeout=300s

while true; do
    kubectl get node "$TEST_NODE_NAME" &> /dev/null && break
    sleep 3
done

kubectl wait --for=condition=Ready --timeout=300s node "$TEST_NODE_NAME"

export TEST_NODE_NAME

## CSI Driver test
az storage account create -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g $RESOURCE_GROUP -l $LOCATION --sku Standard_LRS
export AZURE_STORAGE_CONNECTION_STRING=$(az storage account show-connection-string -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g $RESOURCE_GROUP -o tsv)

az storage share create -n $CSI_DRIVER_SHARE_NAME --connection-string $AZURE_STORAGE_CONNECTION_STRING
CSI_DRIVER_STORAGE_ACCOUNT_KEY=$(az storage account keys list --resource-group $RESOURCE_GROUP --account-name $CSI_DRIVER_STORAGE_ACCOUNT_NAME --query "[0].value" -o tsv)

export CSI_DRIVER_STORAGE_ACCOUNT_NAME=$CSI_DRIVER_STORAGE_ACCOUNT_NAME
export CSI_DRIVER_STORAGE_ACCOUNT_KEY=$CSI_DRIVER_STORAGE_ACCOUNT_KEY

$@
