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

: "${RESOURCE_GROUP:=vk-aci-test-$RANDOM_NUM}"
: "${LOCATION:=eastus2}"
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
: "${ACR_NAME=vkacr$RANDOM_NUM}"
: "${CSI_DRIVER_STORAGE_ACCOUNT_NAME=vkcsidrivers$RANDOM_NUM}"
: "${CSI_DRIVER_SHARE_NAME=vncsidriversharename}"
: "${ACR_NAME=vktestregistry$RANDOM_NUM}"

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

echo -e "\n......Creating Resource Group\n"
az group create --name "$RESOURCE_GROUP" --location "$LOCATION"


KUBE_DNS_IP=10.0.0.10

echo -e "\n......Creating vNet\n"
az network vnet create \
    --resource-group $RESOURCE_GROUP \
    --name $VNET_NAME \
    --address-prefixes $VNET_RANGE \
    --subnet-name $CLUSTER_SUBNET_NAME \
    --subnet-prefix $CLUSTER_SUBNET_RANGE
echo -e "\n......Creating vNet.........[DONE]\n"

echo -e "\n......Creating vNet subnet\n"
aci_subnet_id="$(az network vnet subnet create \
    --resource-group $RESOURCE_GROUP \
    --vnet-name $VNET_NAME \
    --name $ACI_SUBNET_NAME \
    --address-prefix $ACI_SUBNET_RANGE \
    --query id -o tsv)"
echo -e "\n......Creating vNet subnet.........[DONE]\n"


vnet_id="$(az network vnet show --resource-group $RESOURCE_GROUP --name $VNET_NAME --query id -o tsv)"
aks_subnet_id="$(az network vnet subnet show --resource-group $RESOURCE_GROUP --vnet-name $VNET_NAME --name $CLUSTER_SUBNET_NAME --query id -o tsv)"

TMPDIR="$(mktemp -d)"
echo -e "\n......Creating managed identities for AKS and Node\n"
cluster_identity="$(az identity create --name "${RESOURCE_GROUP}-aks-identity" --resource-group "${RESOURCE_GROUP}" --query principalId -o tsv)"
node_identity="$(az identity create --name "${RESOURCE_GROUP}-node-identity" --resource-group "${RESOURCE_GROUP}" --query principalId -o tsv)"

node_identity_id="$(az identity show --name ${RESOURCE_GROUP}-node-identity --resource-group ${RESOURCE_GROUP} --query id -o tsv)"
cluster_identity_id="$(az identity show --name ${RESOURCE_GROUP}-aks-identity --resource-group ${RESOURCE_GROUP} --query id -o tsv)"
echo -e "\n......Creating managed identities for AKS and Node.........[DONE]\n"

echo -e "\n......Creating ACR\n"
az acr create --resource-group ${RESOURCE_GROUP} --name ${ACR_NAME} --sku Standard
echo -e "\n......Creating ACR.........[DONE]\n"

az acr import --name ${ACR_NAME} --source docker.io/library/alpine:latest
export ACR_ID="$(az acr show --resource-group ${RESOURCE_GROUP} --name ${ACR_NAME} --query id -o tsv)"
export ACR_NAME=${ACR_NAME}


if [ "$E2E_TARGET" = "pr" ]; then
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
    --generate-ssh-keys \
    --attach-acr "$ACR_NAME"
fi

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
	--attach-acr $ACR_ID \
    --generate-ssh-keys
echo -e "\n......Creating AKS Cluster.........[DONE]\n"

echo -e "\n......Creating RBAC Role for Network Contributor on vNet\n"
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
echo -e "\n......Creating RBAC Role for Network Contributor on vNet.........[DONE]\n"


# Make sure ACI can create containers in the AKS RG.
# Note, this is not wonderful since it gives a lot of permissions to the identity which is also shared with the kubelet (which it doesn't need).
# Unfortunately there is no way to scope this down (AFIACT) currently.
echo -e "\n......Creating RBAC Role for Contributor on Resource Group\n"
az role assignment create \
    --role "Contributor" \
    --assignee-object-id "$node_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "/subscriptions/$(az account show --query id -o tsv)/resourceGroups/MC_${RESOURCE_GROUP}_${RESOURCE_GROUP}_${LOCATION}"

az role assignment create \
    --role "Contributor" \
    --assignee-object-id "$node_identity" \
    --assignee-principal-type "ServicePrincipal" \
    --scope "/subscriptions/$(az account show --query id -o tsv)/resourceGroups/${RESOURCE_GROUP}"
echo -e "\n......Creating RBAC Role for Contributor on Resource Group.........[DONE]\n"

echo -e "\n......Set AKS Cluster context\n"
az aks get-credentials -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" -f "${TMPDIR}/kubeconfig"
export KUBECONFIG="${TMPDIR}/kubeconfig"
echo -e "\n......Set AKS Cluster context.........[DONE]\n"

echo -e "\n......Get AKS Cluster Master URL\n"
MASTER_URI="$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")"
echo -e "\n......Get AKS Cluster Master URL.........[DONE]\n"

echo -e "\n......Install Virtual node on the AKS Cluster with ACI provider\n"
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
    --set "providers.azure.aciResourceGroup=$RESOURCE_GROUP" \
    --set "providers.azure.aciRegion=$LOCATION" \
    "$CHART_NAME" \
    ./charts/virtual-kubelet

kubectl wait --for=condition=available deploy "${TEST_NODE_NAME}-virtual-kubelet-azure-aci" --timeout=300s

while true; do
    kubectl get node "$TEST_NODE_NAME" &> /dev/null && break
    sleep 3
done

kubectl wait --for=condition=Ready --timeout=300s node "$TEST_NODE_NAME"

export TEST_NODE_NAME
echo -e "\n......Install Virtual node on the AKS Cluster with ACI provider.........[DONE]\n"

echo -e "\n......Initialize environment variabled needed for E2e tests\n"
## CSI Driver test
az storage account create -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g $RESOURCE_GROUP -l $LOCATION --sku Standard_LRS
export AZURE_STORAGE_CONNECTION_STRING=$(az storage account show-connection-string -n $CSI_DRIVER_STORAGE_ACCOUNT_NAME -g $RESOURCE_GROUP -o tsv)

az storage share create -n $CSI_DRIVER_SHARE_NAME --connection-string $AZURE_STORAGE_CONNECTION_STRING
CSI_DRIVER_STORAGE_ACCOUNT_KEY=$(az storage account keys list --resource-group $RESOURCE_GROUP --account-name $CSI_DRIVER_STORAGE_ACCOUNT_NAME --query "[0].value" -o tsv)

export CSI_DRIVER_STORAGE_ACCOUNT_NAME=$CSI_DRIVER_STORAGE_ACCOUNT_NAME
export CSI_DRIVER_STORAGE_ACCOUNT_KEY=$CSI_DRIVER_STORAGE_ACCOUNT_KEY

envsubst < e2e/fixtures/mi-pull-image.yaml > e2e/fixtures/mi-pull-image-exec.yaml

echo -e "\n......Initialize environment variabled needed for E2e tests.........[DONE]\n"

$@
