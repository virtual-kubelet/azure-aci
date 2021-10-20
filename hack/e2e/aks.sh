#!/usr/bin/env bash

# To test a latest version of the code, you need to build+push the image somewhere.
# Then you can set `IMG_URL`, `IMG_REPO`, and `IMG_TAG` accordingly

set -e -u

if ! type helm > /dev/null; then
  exit 1
fi

if ! type go > /dev/null; then
  exit 1
fi

: "${RESOURCE_GROUP:=vk-aci-test-$(date +%s)}"
: "${LOCATION:=westus2}"
: "${CLUSTER_NAME:=${RESOURCE_GROUP}}"
: "${NODE_COUNT:=1}"
: "${CHART_NAME:=vk-aci-test}"
: "${TEST_NODE_NAME:=vk-aci-test}"
: "${IMG_REPO:=oss/virtual-kubelet/virtual-kubelet}"
: "${IMG_URL:=mcr.microsoft.com}"

: "${VNET_RANGE=10.0.0.0/8}"
: "${CLUSTER_SUBNET_RANGE=10.240.0.0/16}"
: "${ACI_SUBNET_RANGE=10.241.0.0/16}"
: "${VNET_NAME=myAKSVNet}"
: "${CLUSTER_SUBNET_NAME=myAKSSubnet}"
: "${ACI_SUBNET_NAME=myACISubnet}"

: "${AZURE_CLIENT_ID:=}"
: "${AZURE_CLIENT_SECRET:=}"
: "${AZURE_TENANT_ID:=}"

error() {
    echo "$@" >&2
    exit 1
}

if [ ! -v IMG_TAG ] || [ -z "$IMG_TAG" ]; then
    IMG_TAG="$(git describe --abbrev=0 --tags)"
    IMG_TAG="${IMG_TAG#v}"
fi

if [ -n "$AZURE_CLIENT_ID" ]; then
    [ -n "$AZURE_CLIENT_SECRET" ] || error "AZURE_CLIENT_SECRET is required when AZURE_CLIENT_ID is set"
    [ -n "$AZURE_TENANT_ID" ] || error "AZURE_TENANT_ID is required when AZURE_CLIENT_ID is set"
fi

TMPDIR=""

cleanup() {
    az group delete --name "$RESOURCE_GROUP" --yes --no-wait
    az ad sp delete --id "$AZURE_CLIENT_ID" || true
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
sp="$(az ad sp create-for-rbac --name $RESOURCE_GROUP --skip-assignment -o json)"

AZURE_CLIENT_ID="$(jq -sr '.[0].appId' <<<$sp)"
AZURE_CLIENT_SECRET="$(jq -sr '.[0].password' <<<$sp)"
AZURE_TENANT_ID="$(jq -rs '.[0].tenant' <<<$sp)"

az aks create \
    -g "$RESOURCE_GROUP" \
    -l "$LOCATION" \
    -c "$NODE_COUNT" \
    -n "$CLUSTER_NAME" \
    --network-plugin azure \
    --vnet-subnet-id "$aks_subnet_id" \
    --dns-service-ip "$KUBE_DNS_IP" \
    --service-principal $AZURE_CLIENT_ID \
    --client-secret $AZURE_CLIENT_SECRET

az role assignment create \
    --role "Network Contributor" \
    --assignee "$AZURE_CLIENT_ID" \
    --scope "$vnet_id"
az role assignment create \
    --role "Network Contributor" \
    --assignee "$AZURE_CLIENT_ID" \
    --scope "$aci_subnet_id"

az role assignment create \
    --role "Contributor" \
    --assignee "$AZURE_CLIENT_ID" \
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
    --set "providers.azure.clientId=${AZURE_CLIENT_ID}" \
    --set "providers.azure.clientKey=${AZURE_CLIENT_SECRET}" \
    --set "providers.azure.tenantId=${AZURE_TENANT_ID}" \
    --set "providers.azure.vnet.subnetName=$ACI_SUBNET_NAME" \
    --set "providers.azure.vent.subnetCidr=$ACI_SUBNET_RANGE" \
    --set "providers.azure.vnet.clusterCidr=$CLUSTER_SUBNET_RANGE" \
    --set "providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP" \
    --set "providers.azure.masterUri=$MASTER_URI" \
    "$CHART_NAME" \
    ./helm

kubectl wait --for=condition=available deploy "${TEST_NODE_NAME}-virtual-kubelet-aci-for-aks" --timeout=300s

while true; do
    kubectl get node "$TEST_NODE_NAME" &> /dev/null && break
    sleep 3
done

kubectl wait --for=condition=Ready --timeout=300s node "$TEST_NODE_NAME"

export TEST_NODE_NAME
export _RUN_TESTS=1

$@
