### Pulling Images Using AKS Managed identity
If your image is on a private reigstry, the AKS agent pool identity can be used to pull the images

Attach the private acr registry to the cluster. This will give AcrPull access to the AKS agent pool managed identity
```bash
az aks update -g <resource group> -n <cluster name> --attach-acr <acr name>
```

Create a new pod that pulls an image from the private registry, for example
```yaml
spec:
  containers:
  - image: <ACR NAME>.azurecr.io/<IMAGE NAME>:<IMAGE TAG>
    name: test-container
```

#### Optional: Use a Custom Managed Identity
To use a custom manged identity instead of the AKS agent pool identity, it must be added as a kubelet identity on the aks cluster.
```bash
az identity create -g <RESOURCE GROUP> -n <USER ASSIGNED IDENTITY NAME>
az aks update -g <RESOURCE GROUP> -n <CLUSER NAME> --assign-kubelet-identity <USER ASSIGNED IDENTITY URI>
```

Then Attach the container registry 
```bash
az aks update -g <resource group> -n <cluster name> --attach-acr <acr name>
```

