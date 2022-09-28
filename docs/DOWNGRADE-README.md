# Azure ACI plugin for Virtual Kubelet Downgrade setup

## Installation

Instructions for install a previous official release using Helm (1.4.1 as an example in this document).

### Prerequisites

- Install [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Installing the chart

1. Clone project

```shell
$ git clone https://github.com/virtual-kubelet/azure-aci.git
$ cd azure-aci
$ git checkout v1.4.1
```

2. Install chart using Helm v3.0+
```shell
export CHART_NAME=virtual-kubelet-azure-aci-downgrade
export VK_RELEASE=virtual-kubelet-azure-aci-1.4.1
export NODE_NAME=virtual-kubelet-aci-1.4.1
export CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/gh-pages/charts/$VK_RELEASE.tgz
export MASTER_URI=$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")
export IMG_URL=mcr.microsoft.com
export IMG_REPO=oss/virtual-kubelet/virtual-kubelet
export IMG_TAG=1.4.1
export ENABLE_VNET=true
export VIRTUAL_NODE_SUBNET_NAME=virtual-node-aci

# This is the client id of the aciconnectorlinux-#CLUSTERNAME managed identity in the MC_XXXX resource group of the AKS cluster.
# Tip: you can find the id from the same env variable from the Pod spec of the aci-connector-linux-XXXX pod running in the kube-system namespace if any.
export VIRTUALNODE_USER_IDENTITY_CLIENTID=
# You can find the subnet range from the vnet view in the portal, check for the IP range of subnet "virtual-node-aci", by default, it is "10.240.0.0/16".
export VIRTUAL_NODE_SUBNET_RANGE=
# You can find the following from the portal: your aks service Overview->Properties->Networking
# By default, the CLUSTER_SUBNET_RANGE is "10.0.0.0/16" and the KUBE_DNS_IP is "10.0.0.10".
export CLUSTER_SUBNET_RANGE=
export KUBE_DNS_IP=

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

Note that the above helm command will install the 1.4.1 aci virtual kubelet in the `default` namespace. 
The original aci virtual kubelet is deployed in the `kube-system` namespace.


3. Verify that manually deployed virtual kubelet pod is running properly

```shell
$ kubectl get nodes
```
<details>
<summary>Result</summary>

```shell
NAME                                   STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci-1.4.1              Ready     agent     2m        v1.19.10-vk-azure-aci-v1.4.1
virtual-node-aci-linux                 Ready     agent   150m        v1.19.10-vk-azure-aci-v1.4.4-dev
```
</details><br/>

The `virtual-kubelet-aci-1.4.1` virtual node is managed by the downgraded version of aci virtual kubelet.
Users can add labels/taints to the `virtual-kubelet-aci-1.4.1` node and change the deployment Pod
template accordingly so that new ACI Pods can be scheduled to the `virtual-kubelet-aci-1.4.1` virtual node.  

### Configuration

For more advanced configurations, please refer to the helm parameters [README](../charts/virtual-kubelet/README.md)
