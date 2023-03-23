# Install an ACI plugin for running windows ACI instances

ACI provides limited supports for running windows containers. The windows ACI instances do not support VNET, hence the configuration is simpler.


## Prepare the env variables
```bash
export RELEASE_TAG=1.5.1  # or the latest released version
export RELEASE_NAME=virtual-kubelet-azure-aci
export VK_RELEASE=$RELEASE_NAME-$RELEASE_TAG
export NODE_NAME=virtual-kubelet
export CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/gh-pages/charts/$VK_RELEASE.tgz
export MASTER_URI="$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")"
```

## Install the helm chart

If you are using an AKS cluster, run the following command:
```bash
helm install "$RELEASE_NAME" "$CHART_URL" \
        --set nodeOsType=Windows \
        --set providers.azure.targetAKS=true \
        --set providers.azure.masterUri=$MASTER_URI \
        --set nodeName="${NODE_NAME}-win"
```
