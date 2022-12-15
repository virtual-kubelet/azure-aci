# Kubernetes Virtual Kubelet with ACI

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/virtual-kubelet/azure-aci)](https://github.com/virtual-kubelet/azure-aci/releases)
[![aks-addon-e2e-tests](https://github.com/virtual-kubelet/azure-aci/actions/workflows/aks-addon-tests.yml/badge.svg?branch=master)](https://github.com/virtual-kubelet/azure-aci/actions/workflows/aks-addon-tests.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/virtual-kubelet/azure-aci)](https://goreportcard.com/report/github.com/virtual-kubelet/azure-aci)
[![codecov](https://codecov.io/gh/virtual-kubelet/azure-aci/branch/master/graph/badge.svg?token=XHb1xbrki0)](https://codecov.io/gh/virtual-kubelet/azure-aci)

Azure Container Instances (ACI) provide a hosted environment for running containers in Azure. When using ACI, there is no need to manage the underlying compute infrastructure, Azure handles this management for you. When running containers in ACI, you are charged by the second for each running container.

The Azure Container Instances provider for the Virtual Kubelet configures an ACI instance as a node in any Kubernetes cluster. When using the Virtual Kubelet ACI provider, pods can be scheduled on an ACI instance as if the ACI instance is a standard Kubernetes node. This configuration allows you to take advantage of both the capabilities of Kubernetes and the management value and cost benefit of ACI.

This document details configuring the Virtual Kubelet ACI provider.

## Table of Contents

* [Feature set](#current-feature-set)
* [Prerequisites](#prerequisites)
* [Set-up virtual node in AKS](#cluster-and-azure-account-setup)
* [Manual set-up](#manual-set-up)
* [Create a AKS cluster with a Virtual Network](#create-an-aks-cluster-with-vnet)
* [Validate the Virtual Kubelet ACI provider](#validate-the-virtual-kubelet-aci-provider)
* [Schedule a pod in ACI](#schedule-a-pod-in-aci)
* [Work around for the virtual kubelet pod](#Work-around-for-the-virtual-kubelet-pod)
* [Remove the Virtual Kubelet](#remove-the-virtual-kubelet)

## Current feature set

Virtual Kubelet's ACI provider relies heavily on the feature set that Azure Container Instances provide. Please check the Azure documentation accurate details on region availability, pricing and new features. The list here attempts to give an accurate reference for the features we support in ACI and the ACI provider within Virtual Kubelet.

### Features

* Volumes: empty dir, github repo, projection, Azure Files, Azure Files CSI drivers
* Secure env variables, config maps
* Bring your own virtual network (VNet)
* Network security group support
* Basic Azure Networking support within AKS virtual node
* [Exec support](https://docs.microsoft.com/azure/container-instances/container-instances-exec) for container instances
* Azure Monitor integration ( aka OMS)
* Support for init-containers ([use init containers](#Create-pod-with-init-containers))

### Limitations

* Using service principal credentials to pull ACR images ([see workaround](#Private-registry))
* Liveness and readiness probes
* [Limitations](https://docs.microsoft.com/azure/container-instances/container-instances-vnet) with VNet
* VNet peering
* Argument support for exec
* [Host aliases](https://kubernetes.io/docs/concepts/services-networking/add-entries-to-pod-etc-hosts-with-host-aliases/) support
* downward APIs (i.e podIP)

## Prerequisites

* Kubernetes cluster up and running (can be an AKS cluster or `minikube`) and that `kubectl` is already configured.
* A [Microsoft Azure account](https://azure.microsoft.com/free/).
* Install the [Azure CLI](#install-the-azure-cli).
* Install the [Kubernetes CLI](#install-the-kubernetes-cli).
* Install the [Helm CLI](#install-the-helm-cli).

You may also use [Azure cloud shell](https://docs.microsoft.com/azure/cloud-shell/overview) which has the above tools already installed.

### Install the Azure CLI

Install `az` by following the instructions for your operating system.
See the [full installation instructions](https://docs.microsoft.com/cli/azure/install-azure-cli?view=azure-cli-latest) if yours isn't listed below.

#### MacOS

```bash
brew install azure-cli
```

#### Windows

Download and run the [Azure CLI Installer (MSI)](https://aka.ms/InstallAzureCliWindows).

#### Ubuntu 64-bit

1. Add the azure-cli repo to your sources:

```bash
AZ_REPO=$(lsb_release -cs)
echo "deb [arch=amd64] https://packages.microsoft.com/repos/azure-cli/ $AZ_REPO main" |
    sudo tee /etc/apt/sources.list.d/azure-cli.list
```

2. Run the following commands to install the Azure CLI and its dependencies:

```bash
sudo apt-get install apt-transport-https
curl -sL https://packages.microsoft.com/keys/microsoft.asc |
    gpg --dearmor |
    sudo tee /etc/apt/trusted.gpg.d/microsoft.gpg > /dev/null
sudo apt-get update && sudo apt-get install azure-cli
```

### Install the Kubernetes CLI

Install `kubectl` by running the following command:

```bash
az aks install-cli
```

### Install the Helm 3.x CLI

[Helm](https://github.com/helm/helm) is a tool for installing pre-configured applications on Kubernetes. Install `helm` for macOS, Windows, or Linux [via binary releases or package managers](https://github.com/helm/helm#install) or check the detailed [Helm install guide](https://helm.sh/docs/intro/install/) for more options including building from source.

## Cluster and Azure Account Setup

Now that we have all the tools, we will set up your Azure account to work with ACI.

### Configure your Azure account

First let's identify your Azure subscription and save it for use later on in the quickstart.

1. Run `az login` and follow the instructions in the command output to authorize `az` to use your account
2. List your Azure subscriptions:

    ```bash
    az account list -o table
    ```

3. Copy your subscription ID and save it in an environment variable:

    **Bash**

    ```bash
    export AZURE_SUBSCRIPTION_ID="<SubscriptionId>"
    ```

    **PowerShell**

    ```powershell
    $env:AZURE_SUBSCRIPTION_ID = "<SubscriptionId>"
    ```

4. Enable ACI in your subscription:

   ```bash
   az provider register -n Microsoft.ContainerInstance
   ```

## Quick set up with AKS

### Linux containers with Virtual Nodes

Azure Kubernetes Service has an efficient way of setting up virtual kubelet with the ACI provider with a feature called virtual node. You can easily install a virtual node that will deploy Linux workloads to ACI. The pods that spin out will automatically get private IPs and will be within a subnet that is within the AKS cluster's Virtual Network. **Virtual Nodes is the recommended path for using the ACI provider on Linux AKS clusters.**

To install virtual node in the Azure portal go [here](https://docs.microsoft.com/azure/aks/virtual-nodes-portal). To install virtual node in the Azure CLI go [here](https://docs.microsoft.com/azure/aks/virtual-nodes-cli).

## Manual set-up

### Create a Resource Group for ACI

To use Azure Container Instances, you must provide a resource group. Create one with the az cli using the following command.

```bash
export ACI_REGION=eastus
az group create --name aci-group --location "$ACI_REGION"
export AZURE_RG=aci-group
```

### Create a service principal

This creates an identity for the Virtual Kubelet ACI provider to use when provisioning
resources on your account on behalf of Kubernetes. If you are provisioning Virtual Kubelet on AKS, please note: 1) This step is optional 2) The service principal will expire within a year unless [updated](https://docs.microsoft.com/azure/aks/update-credentials).

1. Create a service principal with RBAC enabled for the quickstart:

```bash
   az ad sp create-for-rbac --name virtual-kubelet-quickstart -o table
```

2. Save the values from the command output in environment variables:

    **Bash**

    ```bash
    export AZURE_TENANT_ID=<Tenant>
    export AZURE_CLIENT_ID=<AppId>
    export AZURE_CLIENT_SECRET=<Password>
    ```

    **PowerShell**

    ```powershell
    $env:AZURE_TENANT_ID = "<Tenant>"
    $env:AZURE_CLIENT_ID = "<AppId>"
    $env:AZURE_CLIENT_SECRET = "<Password>"
    ```

## Deployment of the ACI provider in your cluster

Run these commands to deploy the virtual kubelet which connects your Kubernetes cluster to Azure Container Instances.

```bash
export VK_RELEASE=virtual-kubelet-latest
```

Grab the public master URI for your Kubernetes cluster and save the value.

```bash
export MASTER_URI="$(kubectl cluster-info | awk '/Kubernetes control plane/{print $7}' | sed "s,\x1B\[[0-9;]*[a-zA-Z],,g")"
```

If your cluster is an AKS cluster:

```bash
export RELEASE_NAME=virtual-kubelet
export VK_RELEASE=virtual-kubelet-latest
export NODE_NAME=virtual-kubelet
export CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/master/charts/$VK_RELEASE.tgz

# Linux Virtual Node
helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set providers.azure.targetAKS=true \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName=$NODE_NAME

# Windows Virtual Node
helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set "nodeOsType=Windows" \
  --set providers.azure.targetAKS=true \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName="${NODE_NAME}-win"
```

For any other type of Kubernetes cluster:

```bash
RELEASE_NAME=virtual-kubelet
NODE_NAME=virtual-kubelet
CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/master/charts/$VK_RELEASE.tgz

# Linux Virtual Node
helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set rbac.install=true \
  --set providers.azure.targetAKS=false \
  --set providers.azure.aciResourceGroup=$AZURE_RG \
  --set providers.azure.aciRegion=$ACI_REGION \
  --set providers.azure.tenantId=$AZURE_TENANT_ID \
  --set providers.azure.subscriptionId=$AZURE_SUBSCRIPTION_ID \
  --set providers.azure.clientId=$AZURE_CLIENT_ID \
  --set providers.azure.clientKey=$AZURE_CLIENT_SECRET \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName=$NODE_NAME

# Windows Virtual Node
helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set rbac.install=true \
  --set "nodeOsType=Windows" \
  --set providers.azure.targetAKS=false \
  --set providers.azure.aciResourceGroup=$AZURE_RG \
  --set providers.azure.aciRegion=$ACI_REGION \
  --set providers.azure.tenantId=$AZURE_TENANT_ID \
  --set providers.azure.subscriptionId=$AZURE_SUBSCRIPTION_ID \
  --set providers.azure.clientId=$AZURE_CLIENT_ID \
  --set providers.azure.clientKey=$AZURE_CLIENT_SECRET \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName="${NODE_NAME}-win"
```

If your cluster has RBAC disabled set ```rbac.install=false```

Output:

```console
NAME:   virtual-kubelet
LAST DEPLOYED: Thu Feb 15 13:17:01 2018
NAMESPACE: default
STATUS: DEPLOYED

RESOURCES:
==> v1/Secret
NAME                             TYPE    DATA  AGE
virtual-kubelet-virtual-kubelet  Opaque  3     1s

==> v1beta1/Deployment
NAME                             DESIRED  CURRENT  UP-TO-DATE  AVAILABLE  AGE
virtual-kubelet-virtual-kubelet  1        1        1           0          1s

==> v1/Pod(related)
NAME                                              READY  STATUS             RESTARTS  AGE
virtual-kubelet-virtual-kubelet-7bcf5dc749-6mvgp  0/1    ContainerCreating  0         1s


NOTES:
The virtual kubelet is getting deployed on your cluster.

To verify that virtual kubelet has started, run:

```bash
  kubectl --namespace=default get pods -l "app=virtual-kubelet-virtual-kubelet"
```

## Create an AKS cluster with VNet

  Run the following commands to create an AKS cluster with a new Azure virtual network. Also, create two subnets. One will be delegated to the cluster and the other will be delegated to Azure Container Instances.

### Create an Azure virtual network and subnets

  First, set the following variables for your VNet range and two subnet ranges within that VNet.

```bash
  export VNET_RANGE=10.0.0.0/8  
  export CLUSTER_SUBNET_RANGE=10.240.0.0/16
  export ACI_SUBNET_RANGE=10.241.0.0/16
  export VNET_NAME=myAKSVNet
  export CLUSTER_SUBNET_NAME=myAKSSubnet
  export ACI_SUBNET_NAME=myACISubnet
  export AKS_CLUSTER_RG=myresourcegroup
  export KUBE_DNS_IP=10.0.0.10
```

Run the following command to create a virtual network within Azure, and a subnet within that VNet. The subnet will be dedicated to the nodes in the AKS cluster.

```bash
az network vnet create \
--resource-group $AKS_CLUSTER_RG \
--name $VNET_NAME \
--address-prefixes $VNET_RANGE \
--subnet-name $CLUSTER_SUBNET_NAME \
--subnet-prefix $CLUSTER_SUBNET_RANGE
```

Create a subnet that will be delegated to just resources within ACI, note that this needs to be an empty subnet, but within the same VNet that you already created.

```bash
az network vnet subnet create \
    --resource-group $AKS_CLUSTER_RG \
    --vnet-name $VNET_NAME \
    --name $ACI_SUBNET_NAME \
    --address-prefix $ACI_SUBNET_RANGE
```

### Create a service principal (OPTIONAL)

Create an Azure Active Directory service principal to allow AKS to interact with other Azure resources. You can use a pre-created service principal too.

```bash
az ad sp create-for-rbac -n "virtual-kubelet-sp" --skip-assignment
```

The output should look similar to the following.

```console
{
  "appId": "bef76eb3-d743-4a97-9534-03e9388811fc",
  "displayName": "azure-cli-2018-08-29-22-29-29",
  "name": "http://azure-cli-2018-08-29-22-29-29",
  "password": "1d257915-8714-4ce7-xxxxxxxxxxxxx",
  "tenant": "72f988bf-86f1-41af-91ab-2d7cd011db48"
}
```

Save the output values from the command output in environment variables.

```bash
export AZURE_TENANT_ID=<Tenant>
export AZURE_CLIENT_ID=<AppId>
export AZURE_CLIENT_SECRET=<Password>
```

These values can be integrated into the `az aks create` as a field `--service-principal $AZURE_CLIENT_ID`.

### Integrating Azure VNet Resource

If you want to integrate an already created Azure VNet resource with your AKS cluster than follow these steps.
Grab the virtual network resource id with the following command:

```bash
az network vnet show --resource-group $AKS_CLUSTER_RG --name $VNET_NAME --query id -o tsv
```

Grant access to the AKS cluster to use the virtual network by creating a role and assigning it.

```bash
az role assignment create --assignee $AZURE_CLIENT_ID --scope <vnetId> --role "Network Contributor"
```

### Create an AKS cluster with a virtual network

Grab the id of the cluster subnet you created earlier with the following command.

```bash
az network vnet subnet show --resource-group $AKS_CLUSTER_RG --vnet-name $VNET_NAME --name $CLUSTER_SUBNET_NAME --query id -o tsv
```

Save the entire output starting with "/subscriptions/..." in the following environment variable.

```bash
export VNET_SUBNET_ID=<subnet-resource>
```

Use the following command to create an AKS cluster with the virtual network you've already created.

```bash
az aks create \
    --resource-group myResourceGroup \
    --name myAKSCluster \
    --node-count 1 \
    --network-plugin azure \
    --service-cidr 10.0.0.0/16 \
    --dns-service-ip $KUBE_DNS_IP \
    --docker-bridge-address 172.17.0.1/16 \
    --vnet-subnet-id $VNET_SUBNET_ID \
    --client-secret $AZURE_CLIENT_SECRET
```

### Deploy Virtual Kubelet

Manually deploy the Virtual Kubelet, the following env. variables have already been set earlier. You do need to pass through the subnet you created for ACI earlier, otherwise the container instances will not be able to participate with the other pods within the cluster subnet.

Grab the public master URI for your Kubernetes cluster and save the value.

```bash
kubectl cluster-info
export MASTER_URI=<public uri>
```

Set the following values for the helm chart.

```bash
RELEASE_NAME=virtual-kubelet
NODE_NAME=virtual-kubelet
CHART_URL=https://github.com/virtual-kubelet/azure-aci/raw/master/charts/$VK_RELEASE.tgz
```

If your cluster is an AKS cluster:

```bash
helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set providers.azure.targetAKS=true \
  --set providers.azure.vnet.enabled=true \
  --set providers.azure.vnet.subnetName=$ACI_SUBNET_NAME \
  --set providers.azure.vent.subnetCidr=$ACI_SUBNET_RANGE \
  --set providers.azure.vnet.clusterCidr=$CLUSTER_SUBNET_RANGE \
  --set providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP \
  --set providers.azure.masterUri=$MASTER_URI \
  --set nodeName=$NODE_NAME
```

For any other type of cluster:

```bash
# the resource group where the virtual network is located in
export ACI_VNET_RESOURCE_GROUP=<resource group>

# the virtual network name where container will deploy to
export ACI_VNET_NAME=<virtual network name>
# subnet name where ACI will deploy to. Virtual Kubelet will automatically create subnet resource if it not exists
export ACI_SUBNET_NAME=<subnet name>
# subnet's IP range, for example 10.1.0.0/16. You don't need specific this system variable if subnet has been exists
export ACI_SUBNET_RANGE=<subnet name where ACI will run in>

helm install "$RELEASE_NAME" "$CHART_URL" \
  --set provider=azure \
  --set providers.azure.targetAKS=false \
  --set providers.azure.vnet.enabled=true \
  --set providers.azure.vnet.vnetResourceGroup=$ACI_VNET_RESOURCE_GROUP \
  --set providers.azure.vnet.vnetName=$ACI_VNET_NAME \
  --set providers.azure.vnet.subnetName=$ACI_SUBNET_NAME \
  --set providers.azure.vent.subnetCidr=$ACI_SUBNET_RANGE \
  --set providers.azure.vnet.kubeDnsIp=$KUBE_DNS_IP \
  --set providers.azure.tenantId=$AZURE_TENANT_ID \
  --set providers.azure.subscriptionId=$AZURE_SUBSCRIPTION_ID \
  --set providers.azure.aciResourceGroup=$AZURE_RG \
  --set providers.azure.aciRegion=$ACI_REGION \
  --set providers.azure.masterUri=$MASTER_URI \
  --set providers.azure.clientId=$AZURE_CLIENT_ID \
  --set providers.azure.clientKey=$AZURE_CLIENT_SECRET
  
  ```

## Validate the Virtual Kubelet ACI provider

To validate that the Virtual Kubelet has been installed, return a list of Kubernetes nodes using the [kubectl get nodes][kubectl-get] command.

```bash
kubectl get nodes
```

Output:

```console
NAME                                        STATUS    ROLES     AGE       VERSION
virtual-kubelet-aci-linux                   Ready     agent     2m        v1.13.1
aks-nodepool1-39289454-0                    Ready     agent     22h       v1.12.6
aks-nodepool1-39289454-1                    Ready     agent     22h       v1.12.6
aks-nodepool1-39289454-2                    Ready     agent     22h       v1.12.6
```

## Schedule a pod in ACI

Create a file named `virtual-kubelet-test.yaml` and copy in the following YAML.

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

Notice that Virtual-Kubelet nodes are tainted by default to avoid unexpected pods running on them, i.e. kube-proxy, other virtual-kubelet pods, etc. To schedule a pod to them, you need to add the toleration to the pod spec and a node selector:

```yaml
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

### Private registry

If your image is on a private registry, you need to [add a kubernetes secret to your cluster](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#create-a-secret-by-providing-credentials-on-the-command-line) and reference it in the pod spec.

```yaml
  spec:
    containers:
    - name: aci-helloworld
      image: <registry name>.azurecr.io/aci-helloworld:v1
      ports:
      - containerPort: 80
    imagePullSecrets:
    - name: <K8 secret name>
```

Run the application with the [kubectl create][kubectl-create] command.

```bash
kubectl create -f virtual-kubelet-test.yaml
```

Use the [kubectl get pods][kubectl-get] command with the `-o wide` argument to output a list of pods with the scheduled node.

```bash
kubectl get pods -o wide
```

Notice that the `helloworld` pod is running on the `virtual-kubelet` node.

```console
NAME                                            READY     STATUS    RESTARTS   AGE       IP             NODE
aci-helloworld-2559879000-8vmjw                 1/1       Running   0          39s       52.179.3.180   virtual-kubelet-aci-linux

```

If the AKS cluster was configured with a virtual network, then the output will look like the following. The container instance will get a private ip rather than a public one.

```console
NAME                            READY     STATUS    RESTARTS   AGE       IP           NODE
aci-helloworld-9b55975f-bnmfl   1/1       Running   0          4m        10.241.0.4   virtual-kubelet-aci-linux
```

To validate that the container is running in an Azure Container Instance, use the [az container list][az-container-list] Azure CLI command.

```bash
az container list -o table
```

Output:

```console
Name                             ResourceGroup    ProvisioningState    Image                     IP:ports         CPU/Memory       OsType    Location
-------------------------------  ---------------  -------------------  ------------------------  ---------------  ---------------  --------  ----------
helloworld-2559879000-8vmjw  myResourceGroup    Succeeded            microsoft/aci-helloworld  52.179.3.180:80  1.0 core/1.5 gb  Linux     eastus
```

<!--
### Schedule an ACI pod with a DNS Name label

Add an annotation to your Pod manifest, `virtualkubelet.io/dnsnamelabel` keyed to what you'd like the Azure Container Instance to receive as a DNS Name, and deploy it.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: helloworld
  annotations:
    virtualkubelet.io/dnsnamelabel: "helloworld-aci"
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
  tolerations:
  - key: virtual-kubelet.io/provider
    operator: Exists
  - key: azure.com/aci
    effect: NoSchedule
```

To confirm the Azure Container Instance received and bound the DNS Name specified, use the [az container show][az-container-show] Azure CLI command. Virtual Kubelet's naming
convention will affect how you use this query, with the argument to `-n` broken down as: nameSpace-podName. Unless specified, Kubernetes will assume
the namespace is `default`.

```azurecli-interactive
az container show -g myResourceGroup -n default-helloworld --query ipAddress.fqdn
```

Output:

```console
"helloworld-aci.westus.azurecontainer.io"
```
-->

### Create pod with init containers
Multiple init containers can be specified in the podspec similar to how containers are specified

```yaml
spec:
  initContainers:
  - image: <INIT CONTAINER IMAGE 1>
    name: init-container-01
    command: [ "/bin/sh" ]
    args: [ "-c", "echo \"Hi\"" ]
  - image: <INIT CONTAINER IMAGE 2>
    name: init-container-02
    command: [ "/bin/sh" ]
    args: [ "-c", "echo \"Hi\"" ]
  containers:
  - image: <CONTAINER IMAGE>
    imagePullPolicy: Always
    name: container
    command: [ "/bin/sh" ]
```
More information on init containers can be found in [Kubernetes](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/) and [ACI](https://docs.microsoft.com/en-us/azure/container-instances/container-instances-init-container) documentations

## Work around for the virtual kubelet pod

If your pod that's scheduled onto the Virtual Kubelet node is in a pending state please add this workaround to your Virtual Kubelet pod spec.

First, grab the logs from your virtual kubelet pod, with the following command. You can get the pod name from the `kubectl get pods` command  

```bash
kubectl logs virtual-kubelet-virtual-kubelet-7bcf5dc749-6mvgp
```

### Stream or pod watcher errors

If you see the following errors in the logs:

```console
ERROR: logging before flag.Parse: E0914 00:02:01.546132       1 streamwatcher.go:109] Unable to decode an event from the watch stream: stream error: stream ID 181; INTERNAL_ERROR
time="2018-09-14T00:02:01Z" level=error msg="Pod watcher connection is closed unexpectedly" namespace= node=virtual-kubelet-myconnector-linux operatingSystem=Linux provider=azure
```

Then copy the master URI with cluster-info.

```bash
kubectl cluster-info
```

Output:

```console
Kubernetes master is running at https://aksxxxx-xxxxx-xxxx-xxxxxxx.hcp.uksouth.azmk8s.io:443
```

Edit virtual kubelet deployment by first getting the deployment name.

```bash
kubectl get deploy
```

Output:

```console
NAME                           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
virtual-kubelet-virtual-kubelet 1         1         1            1           5d
aci-helloworld                  1         1         1            0           12m
```

Edit the deployment.

```bash
kubectl edit deploy virtual-kubelet-virtual-kubelet
```

Add the following name and value to the deployment in the environment section. Use your copied AKS master URI.

```yaml

  ...
- name: MASTER_URI
  value: https://aksxxxx-xxxxx-xxxx-xxxxxxx.hcp.uksouth.azmk8s.io:443

  ```

### Taint deprecated errors

If you see the following errors in the logs:

```console
Flag --taint has been deprecated, Taint key should now be configured using the VK_TAINT_KEY environment variable
```

Then edit your virtual kubelet deployment by first grabbing the deployment name.  

```bash
kubectl get deploy
```

Output:

```console
NAME                           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
virtual-kubelet-virtual-kubelet 1         1         1            1           5d
aci-helloworld                  1         1         1            0           12m
```

Edit the virtual kubelet deployment.

```bash
kubectl edit deploy virtual-kubelet-virtual-kubelet
```

Add the following as an environment variable within the deployment.

```yaml

...
- name: VK_TAINT_KEY
  value: azure.com/aci

```

Also, delete the following argument in your pod spec:

```yaml
- --taint
  - azure.com/aci
```

## Remove the Virtual Kubelet

You can remove your Virtual Kubelet node by deleting the Helm deployment. Run the following command:

```bash
helm uninstall virtual-kubelet
```

If you used Virtual Nodes, can follow the steps [here](https://docs.microsoft.com/azure/aks/virtual-nodes-cli#remove-virtual-nodes) to remove the add-on


<!-- LINKS -->
[kubectl-create]: https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#create
[kubectl-get]: https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#get
[az-container-list]: https://docs.microsoft.com/cli/azure/container?view=azure-cli-latest#az_container_list
[az-container-show]: https://docs.microsoft.com/cli/azure/container?view=azure-cli-latest#az_container_show
