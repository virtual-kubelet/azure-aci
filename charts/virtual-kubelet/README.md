# Azure ACI plugin for Virtual Kubelet

## Installation

Quick start instructions for the setup  using Helm.

### Prerequisites

- [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)
- [AKS](https://docs.microsoft.com/en-us/azure/aks/learn/quick-kubernetes-deploy-cli)

### Installing the chart

1. Clone project

```shell

$ git clone https://github.com/virtual-kubelet/azure-aci.git
$ cd helm

```

2. Install chart using Helm v3.0+

```shell
$ export RELEASE_TAG=1.6.1
$ export CHART_NAME=virtual-kubelet-azure-aci
$ export VK_RELEASE=$CHART_NAME-$RELEASE_TAG
$ export NODE_NAME=virtual-kubelet-aci
$ export CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/gh-pages/charts/$VK_RELEASE.tgz

$ helm install "$CHART_NAME" "$CHART_URL" \
  --set provider=azure \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName=$NODE_NAME
```

3. Verify that azure-aci pod is running properly

```shell
$ kubectl get nodes
```
<details>
<summary>Result</summary>

```shell
NAME                                   STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci                    Ready     agent     2m         v1.19.10-vk-azure-aci-vx.x.x-dev
```
</details><br/>

### Configuration

The following table lists the configurable parameters of the azure-aci chart and the default values.

| Parameter                                      | Description                                                                                                           | Default                               |
|------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|---------------------------------------|
 | namespace                                      | The name of the namespace that azure-aci will be deployed in.                                                         | `vk-azure-aci`                        | 
| image.secretName                               | The name of image secret.                                                                                             | `virtual-kubelet-aci-acr`             |
| image.repository                               | Image repository.                                                                                                     | `mcr.microsoft.com`                   |
| image.name                                     | Image name.                                                                                                           | `oss/virtual-kubelet/virtual-kubelet` |
| image.tag                                      | Image release version/tag.                                                                                            | `latest`                              |
| image.pullPolicy                               | Image pull policy.                                                                                                    | `Always`                              | 
| initImage.name                                 | Init container image name.                                                                                            | `oss/virtual-kubelet/init-validation` |
| initImage.initTag                              | Init container image release version/tag.                                                                             | `0.2.0`                               |
| initImage.pullPolicy                           | Init container image pull policy.                                                                                     | `Always`                              |
| nodeName                                       | The node name that will be assigned to be the VK one.                                                                 | `virtual-node-aci-linux-helm`         |
| nodeOsType                                     | The node/VM type. Values should be `Windows` or `Linux`.                                                              | `Linux`                               |
| monitoredNamespace                             | Kubernetes namespace. default values means monitor `all`                                                              | `""`                                  |
| apiserverCert                                  | API server certificate. By default, the provider will generate a certificate.                                         | ` `                                   |
| apiserverKey                                   | API Server Key. Must be provided only if `apiserverCert` has been set.                                                | ` `                                   |
| logLevel                                       | Log verbosity level.                                                                                                  | ` `                                   |
| disableVerifyClients                           | False means "Do not require client certificate validation".                                                           | `false`                               |
| enableAuthenticationTokenWebhook               | True means to pass `--authentication-token-webhook=true` ,`--client-verify-ca` args.                                  | `true`                                |
| taint.enabled                                  | Taint enabled flag.                                                                                                   | `true`                                |
| taint.key                                      | Taint Key.                                                                                                            | `virtual-kubelet.io/provider`         |
| taint.value                                    | Taint value.                                                                                                          | Same as `provider` parameter          |
| taint.effect                                   | The value must be `NoSchedule`, `PreferNoSchedule` or `NoExecute`.                                                    | `NoSchedule`                          |
| trace.exporter                                 | The default exporter is `opencensus`.                                                                                 | `""`                                  |
| trace.serviceName                              | The service name that exporter get info for. Default is the node name.                                                | Same as `nodeName` parameter          |
| trace.sampleRate                               | Trace sample rate.                                                                                                    | `0`                                   |
| providers.azure.targetAKS                      | Set to true if deploying to Azure Kubernetes Service (AKS), otherwise false.                                          | `true`                                |
| providers.azure.clientId                       | Only required if `targetAKS` is false.                                                                                | ` `                                   |
| providers.azure.clientKey                      | Only required if `targetAKS` is false.                                                                                | ` `                                   |
| providers.azure.tenantId                       | Only required if `targetAKS` is false.                                                                                | ` `                                   |
| providers.azure.subscriptionId                 | Only required if `targetAKS` is false.                                                                                | ` `                                   |
| providers.azure.managedIdentityID              | Only required if `targetAKS` is false.                                                                                | ` `                                   |
| providers.azure.aciResourceGroup               | `aciResourceGroup` and `aciRegion` are required only for non-AKS deployments.                                         | ` `                                   |
| providers.azure.aciRegion                      | `aciResourceGroup` and `aciRegion` are required only for non-AKS deployments.                                         | ` `                                   |
| providers.azure.enableRealTimeMetrics          | Enable Real-Time metrics.                                                                                             | `true`                                |
| providers.azure.masterUri                      | API server URL for the AKS cluster.                                                                                   | ` `                                   |
| providers.azure.loganalytics.enabled           | Log Analytics enabled flag.                                                                                           | `false`                               |
| providers.azure.loganalytics.workspaceId       | Log Analytics workspace ID.                                                                                           | ` `                                   |
| providers.azure.loganalytics.workspaceKey      | Log Analytics workspace Key.                                                                                          | ` `                                   |
| providers.azure.loganalytics.clusterResourceId | Log Analytics cluster resource ID.                                                                                    | ` `                                   |
| providers.azure.vnet.enabled                   | VNet enabled flag.                                                                                                    | `false`                               |
| providers.azure.vnet.vnetResourceGroup         | VNet resource group name.                                                                                             | ` `                                   |
| providers.azure.vnet.subnetName                | If subnet already created on VNet, don't pass subnetCidr if it does not match the existing one.                       | `virtual-node-aci`                    |
| providers.azure.vnet.subnetCidr                | Subnet Cidr. Only required if a subnet has been created outside of VNet.                                              | `10.241.0.0/16`                       |
| providers.azure.vnet.clusterCidr               | If cluster subnet has a different range, please specify its value here. defaults is `10.240.0.0/16` if not specified. | ` `                                   |
| providers.azure.vnet.kubeDnsIp                 | Defaults is `10.0.0.10` if not specified.                                                                             | ` `                                   |
| provider                                       | Virtual Kubelet provider name. Only valid value is `azure`.                                                           | `azure`                               |
| rbac.install                                   | Install Default RBAC roles and bindings.                                                                              | `true`                                |
| rbac.serviceAccountName                        | RBAC service account name.                                                                                            | `virtual-kubelet-helm`                |
| rbac.apiVersion                                | RBAC api version.                                                                                                     | `v1`                                  |
| rbac.roleRef                                   | Cluster role reference.                                                                                               | `cluster-admin`                       |
