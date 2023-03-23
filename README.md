# Kubernetes Virtual Kubelet with ACI

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/virtual-kubelet/azure-aci)](https://github.com/virtual-kubelet/azure-aci/releases)
[![aks-addon-e2e-tests](https://github.com/virtual-kubelet/azure-aci/actions/workflows/aks-addon-tests.yml/badge.svg?branch=master)](https://github.com/virtual-kubelet/azure-aci/actions/workflows/aks-addon-tests.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/virtual-kubelet/azure-aci)](https://goreportcard.com/report/github.com/virtual-kubelet/azure-aci)
[![codecov](https://codecov.io/gh/virtual-kubelet/azure-aci/branch/master/graph/badge.svg?token=XHb1xbrki0)](https://codecov.io/gh/virtual-kubelet/azure-aci)

Azure Container Instances (ACI) provides a hosted environment for running containers in Azure. When using ACI, there is no need to manage the underlying compute infrastructure since Azure handles this management overhead. When running containers in ACI, users are charged based on the container lifecycle in seconds.

The ACI provider for the Virtual Kubelet configures ACI service as a virtual node in a Kubernetes cluster. Hence, pods scheduled on the virtual node can be run on ACI instances. This configuration allows users to take advantage of both the capabilities of Kubernetes and the management value and cost benefit of ACI.

## Features 

Virtual Kubelet's ACI provider relies heavily on the feature set that ACI service provides. Please check the Azure documentations for region availability, pricing and new features. The list here presents a sample reference for the features that ACI provider supports. **Note**: Users should **NOT** expect feature parities between Virutal Kubelet and real Kubernetes Kubelet.

### Supported

* Volumes: empty dir, github repo, projection, Azure Files, Azure Files CSI drivers
* Secure env variables, config maps
* Virtual network integration (VNet)
* Network security group support
* [Exec support](https://docs.microsoft.com/azure/container-instances/container-instances-exec) for container instances
* Azure Monitor integration ( aka OMS)
* Support for init-containers ([use init containers](#Create-pod-with-init-containers))

### Limitations (Not supported)

* Using service principal credentials to pull ACR images ([see workaround](#Private-registry))
* Liveness and readiness probes
* [Limitations](https://docs.microsoft.com/azure/container-instances/container-instances-vnet) with VNet
* VNet peering
* Argument support for exec
* [Host aliases](https://kubernetes.io/docs/concepts/services-networking/add-entries-to-pod-etc-hosts-with-host-aliases/) support
* Downward APIs (i.e podIP)
* Projected volumes
* Potentially any new features introduced in real Kubelet since 1.24.

## Installation

### Using Azure Portal for AKS clusters

Please follow this offical [document ](https://learn.microsoft.com/en-us/azure/aks/virtual-nodes-portal) to install virtual node for an AKS cluster using Azure Portal.

### Using Azure CLI for AKS clusters

Please follow this offical [document ](https://learn.microsoft.com/en-us/azure/aks/virtual-nodes-cli) to install virtual node for an AKS cluster using Azure CLI.

### Deploy Virtual Kubelet manually using Helm 

In the following cases, users may need to install Virtual Kubelet manually.
- The ACI provider releases are rolled out to all AKS clusters that are configured with virtual node addon progressively. If the AKS managed version has problem, you can follow the [downgrade](./docs/DOWNGRADE-README.md) document to use a previous version or the [upgrade](./docs/UPGRADE-README.md) document to use the latest released version. In either case, a new virtual node will be created in the cluster.
- Install a virtual node to support running windows ACI instances ([document](./docs/windows-virtual-node.md)).

Note: Mannually installed virtual nodes are **NOT** managed by AKS anymore, they will not be upgraded automatically.

### Non-AKS Kubernetes clusters
We do not support them. If you have specific requests for those clusters, please file an issue to describe the needs.


## Demo

To validate that the Virtual Kubelet has been installed, check and find the virtual node in the Kubernetes cluster.

```bash
kubectl get nodes
NAME                                        STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci-linux                   Ready     agent     2m        v1.13.1
aks-nodepool1-XXXXXXXX-0                    Ready     agent     22h       v1.12.6
aks-nodepool1-XXXXXXXX-1                    Ready     agent     22h       v1.12.6
aks-nodepool1-XXXXXXXX-2                    Ready     agent     22h       v1.12.6
```

Create a test Pod `virtual-kubelet-test.yaml`.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: helloworld
spec:
  containers:
  - image: microsoft/aci-helloworld
    imagePullPolicy: Always
    name: helloworld
    resources:
      requests:
        memory: 1G
        cpu: 1
    ports:
    - containerPort: 80
      name: http
      protocol: TCP
    - containerPort: 443
      name: https
  dnsPolicy: ClusterFirst
  nodeSelector:
    kubernetes.io/role: agent
    beta.kubernetes.io/os: linux
    type: virtual-kubelet
  tolerations:
  - key: virtual-kubelet.io/provider
    operator: Exists
  - key: azure.com/aci
    effect: NoSchedule
```

Note that virtual nodes are tainted by default to avoid unexpected pods running on them, i.e., `kube-proxy`. To schedule a pod to them, you need to add the toleration to the pod spec and a node selector:

```yaml
...
  nodeSelector:
    kubernetes.io/role: agent
    beta.kubernetes.io/os: linux
    type: virtual-kubelet
  tolerations:
  - key: virtual-kubelet.io/provider
    operator: Exists
  - key: azure.com/aci
    effect: NoSchedule
```

If your image is in a private registry, you need to [add a kubernetes secret to your cluster](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#create-a-secret-by-providing-credentials-on-the-command-line) and reference it in the pod spec.

```yaml
...
  spec:
    containers:
    - name: aci-helloworld
      image: <registry name>.azurecr.io/aci-helloworld:v1
      ports:
      - containerPort: 80
    imagePullSecrets:
    - name: <K8 secret name>
```

Run the application.

```bash
kubectl apply -f virtual-kubelet-test.yaml
```

Note that the `helloworld` pod is running on the `virtual-kubelet` node.

```console
NAME                                            READY     STATUS    RESTARTS   AGE       IP             NODE
aci-helloworld-2559879000-XXXXXX                 1/1       Running   0          39s       52.179.XXX.XXX   virtual-kubelet-aci-linux

```

If the AKS cluster was configured with a virtual network, then the output will look like the following. The container instance will get a private IP rather than a public one.

```console
NAME                            READY     STATUS    RESTARTS   AGE       IP           NODE
aci-helloworld-9b55975f-XXXXX   1/1       Running   0          4m        10.241.XXX.XXX   virtual-kubelet-aci-linux
```

To validate that the container is running in an Azure Container Instance, use the [az container list][az-container-list] Azure CLI command.

```bash
az container list -o table
```

<details>
<summary>Result</summary>

```console
Name                             ResourceGroup    ProvisioningState    Image                     IP:ports         CPU/Memory       OsType    Location
-------------------------------  ---------------  -------------------  ------------------------  ---------------  ---------------  --------  ----------
helloworld-2559879000-XXXXXX  myResourceGroup    Succeeded            microsoft/aci-helloworld  52.179.XXX.XXX:80  1.0 core/1.5 gb  Linux     eastus
```
</details><br/>


## Uninstallation

For manual installation, you can remove the virtual node by deleting the Helm deployment. Run the following command:

```bash
helm uninstall $ChartName
```

If it is an AKS managed virtual node,  please follow the steps [here](https://docs.microsoft.com/azure/aks/virtual-nodes-cli#remove-virtual-nodes) to remove the add-on.


<!-- LINKS -->
[az-container-list]: https://docs.microsoft.com/cli/azure/container?view=azure-cli-latest#az_container_list
