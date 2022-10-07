# Install a downgraded Azure ACI plugin for Virtual Kubelet

This document presents the instructions to install a previous official Azure ACI plugin using Helm (1.4.1 as an example).

## Prerequisites

Install [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

## Install the helm chart of previous release

### Clone the project

```shell
$ git clone https://github.com/virtual-kubelet/azure-aci.git
$ cd azure-aci
$ git checkout v1.4.1
```

### Prepare `env` variables

```shell
# Fixed variables
export CHART_NAME=virtual-kubelet-azure-aci-downgrade
export VK_RELEASE=virtual-kubelet-azure-aci-1.4.1
export NODE_NAME=virtual-kubelet-aci-1.4.1
export CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/gh-pages/charts/$VK_RELEASE.tgz
export MASTER_URI=$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")
export IMG_URL=mcr.microsoft.com
export IMG_REPO=oss/virtual-kubelet/virtual-kubelet
export IMG_TAG=1.4.1
export ENABLE_VNET=true

# ASK cluster dependent variables
# You can find the subnet name and IP range from the vnet view in the azure portal.
# The subnet name should contain the "virtual-node-aci" substr.
export VIRTUAL_NODE_SUBNET_NAME=
# By default, the subnet ip range is "10.240.0.0/16".
export VIRTUAL_NODE_SUBNET_RANGE=
# You can find the following from the portal: AKS service Overview->Properties->Networking
# By default, the CLUSTER_SUBNET_RANGE is "10.0.0.0/16" and the KUBE_DNS_IP is "10.0.0.10".
export CLUSTER_SUBNET_RANGE=
export KUBE_DNS_IP=
```

### Install the chart

The chart will be installed in the `default` namespace. The new resources include a virtual kubelet deployment and all other
related objects. To compare, the built-in ACI virtual kubelet is deployed in the `kube-system` namespace.

#### Option 1: Install using azure managed identity (recommend)

```shell
# The client id of the aciconnectorlinux-XXXX managed identity in the MC_XXXX resource group.
# Tip: you may be able to find the same env variable from Pod spec of the aci-connector-linux-XXXX 
# pod running in kube-system namespace and reuse the id.
export VIRTUALNODE_USER_IDENTITY_CLIENTID=

helm install "$CHART_NAME" \
    --set provider=azure \
    --set providers.azure.masterUri=$MASTER_URI \
    --set nodeName=$NODE_NAME \
    --set image.repository=$IMG_URL  \
    --set image.name=$IMG_REPO \
    --set image.tag=$IMG_TAG \
    --set providers.azure.masterUri=$MASTER_URI \
    --set providers.azure.vnet.enabled=$ENABLE_VNET \
    --set providers.azure.vnet.subnetName=$VIRTUAL_NODE_SUBNET_NAME \
    --set providers.azure.vnet.subnetCidr=$VIRTUAL_NODE_SUBNET_RANGE \
    --set providers.azure.vnet.clusterCidr=$CLUSTER_SUBNET_RANGE \
    --set providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP \
    --set providers.azure.managedIdentityID=$VIRTUALNODE_USER_IDENTITY_CLIENTID \
    ./helm

```

#### Option 2: Install using service principal 

If you decide to use service principal for the virtual kubelet to access Azure services,
make sure to add "Contributor" role and MC_XXXX resource group scope for the service principal.

```shell
export VK_SP=
export RESOURCE_GROUP=
export LOCATION=
export SUBSCRIPTION_ID=$(az account show --query id -o tsv)
# Create a service principal for a resource group using a preferred name and role
export AZURE_CLIENT_SECRET=$(az ad sp create-for-rbac --name $VK_SP \
                         --role "Contributor" \
                         --scopes /subscriptions/${SUBSCRIPTION_ID}/resourceGroups/MC_${RESOURCE_GROUP}_${RESOURCE_GROUP}_${LOCATION}\
                         --query password -o tsv)

export AZURE_CLIENT_ID=$(az ad sp list --display-name $VK_SP --query [$VK_SP].appId -o tsv)
```

In case the AKS cluster is using a custom VNet, you will need to create the following role assignments for the VNet ID and ACI subnet ID:

```shell
export VNET_ID=$(az network vnet show --resource-group $VNET_RESOURCE_GROUP --name $VNET_NAME --query id -o tsv)
export ACI_SUBNET_ID=$(az network vnet subnet show --resource-group $VNET_RESOURCE_GROUP --vnet-name $VNET_NAME --name $ACI_SUBNET_NAME --query id -o tsv)

az role assignment create \
    --assignee-object-id AZURE_CLIENT_ID \
    --role "Network Contributor" \
    --assignee-principal-type "ServicePrincipal" \
    --scope $VNET_ID

az role assignment create \
    --assignee-object-id AZURE_CLIENT_ID \
    --role "Network Contributor" \
    --assignee-principal-type "ServicePrincipal" \
    --scope ACI_SUBNET_ID
```

Now, we can use the service principal we created to install the helm chart:

```shell
# Note: in case you want to reset the Service Principal password, you can run "az ad sp credential reset --id $AZURE_CLIENT_ID --query password -o tsv"

helm install "$CHART_NAME" \
    --set provider=azure \
    --set providers.azure.masterUri=$MASTER_URI \
    --set nodeName=$NODE_NAME \
    --set image.repository=$IMG_URL  \
    --set image.name=$IMG_REPO \
    --set image.tag=$IMG_TAG \
    --set  providers.azure.targetKey=true \
    --set  providers.azure.clientId=$AZURE_CLIENT_ID \
    --set  providers.azure.targetAKS=$AZURE_CLIENT_SECRET \
    --set providers.azure.masterUri=$MASTER_URI \
    --set providers.azure.vnet.enabled=$ENABLE_VNET \
    --set providers.azure.vnet.subnetName=$VIRTUAL_NODE_SUBNET_NAME \
    --set providers.azure.vnet.subnetCidr=$VIRTUAL_NODE_SUBNET_RANGE \
    --set providers.azure.vnet.clusterCidr=$CLUSTER_SUBNET_RANGE \
    --set providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP \
    ./helm
```

### Verification

```shell
$ kubectl get nodes

```shell
NAME                                   STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci-1.4.1              Ready     agent     2m        v1.19.10-vk-azure-aci-v1.4.1
virtual-node-aci-linux                 Ready     agent   150m        v1.19.10-vk-azure-aci-v1.4.4-dev
```

The `virtual-kubelet-aci-1.4.1` virtual node is managed by the downgraded version of ACI virtual kubelet.
Users can add labels/taints to the `virtual-kubelet-aci-1.4.1` node and change the deployment Pod
template accordingly so that new Pods can be scheduled to the `virtual-kubelet-aci-1.4.1` virtual node.  

### Uninstallation

Once the downgraded virtual kubelet is not needed anymore, run the following commands to undo the changes.

```shell
helm uninstall virtual-kubelet-azure-aci-downgrade
kubectl delete node virtual-kubelet-aci-1.4.1
```
