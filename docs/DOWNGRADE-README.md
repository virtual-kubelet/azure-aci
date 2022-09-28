# Azure ACI plugin for Virtual Kubelet Downgrade setup

## Installation

Instructions for setup a downgrade for the previous official release using Helm.

### Prerequisites

- [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Installing the chart

1. Clone project

```shell
$ git clone https://github.com/virtual-kubelet/azure-aci.git
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

# The following variables needs to be set according to the customer cluster.
#(The information can be found in the Azure portal: AKS overview->Properties->Networking panel)
export VIRTUAL_NODE_SUBNET_NAME=
export VIRTUAL_NODE_SUBNET_RANGE=
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
    ./helm

```

3. Verify that azure-aci pod is running properly

```shell
$ kubectl get nodes
```
<details>
<summary>Result</summary>

```shell
NAME                                   STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci-downgrade                    Ready     agent     2m         v1.19.10-vk-azure-aci-v1.4.1-dev
```
</details><br/>

### Configuration

For more advanced configurations, please refer to the helm parameters [README](../charts/virtual-kubelet/README.md)