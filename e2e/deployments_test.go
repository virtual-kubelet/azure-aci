package e2e

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

type KeyVault struct {
	Value string `json:"value"`
}

func TestImagePullUsingKubeletIdentityAndSecrets(t *testing.T) {
	cmd := kubectl("config", "current-context")
	previousCluster, _ := cmd.CombinedOutput()

	subscriptionID := "076cd026-379c-4383-8bec-8835382efe90"
	tenantID := "72f988bf-86f1-41af-91ab-2d7cd011db47"
	clientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"
	azureRG := "aci-virtual-node-test-rg"

	imageRepository := "docker.io"
	imageName := "ysalazar/virtual-kubelet"
	imageTag := "test"

	region := "westus"

	aksClusterName := "aksClusterE2E10"
	nodeName := "virtual-kubelet"
	virtualNodeReleaseName := "virtual-kubelet-e2etest-aks"

	vkRelease := "virtual-kubelet-latest"
	chartURL := "https://github.com/virtual-kubelet/azure-aci/raw/master/charts/" + vkRelease + ".tgz"

	managedIdentity := "e2eDeployTestMI10"
	managedIdentityURI := "/subscriptions/" + subscriptionID + "/resourcegroups/" + azureRG + "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/" + managedIdentity
	containerRegistry := "acivirtualnodetestregistry"

	//create MI with role assignment
	cmd = az("identity", "create", "--resource-group", azureRG, "--name", managedIdentity)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	spID, _ := az("identity", "show", "--resource-group", azureRG, "--name", managedIdentity,
		"--query", "principalId", "--output", "tsv").CombinedOutput()

	registryID, _ := az("acr", "show", "--resource-group", azureRG, "--name", containerRegistry, "--query",
		"id", "--output", "tsv").CombinedOutput()

	cmd = az("role", "assignment", "create", "--assignee-object-id", string(spID),
		"--scope", string(registryID), "--role", "acrpull", "--assignee-principal-type", "ServicePrincipal")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	//get client secret
	vaultName := "aci-virtual-node-test-kv"
	secretName := "aci-virtualnode-sp-dev-credential"
	cmd = az("keyvault", "secret", "show", "--name", secretName, "--vault-name", vaultName, "-o", "json")

	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	var keyvault KeyVault
	json.Unmarshal(out, &keyvault)
	azureClientSecret := keyvault.Value

	//create cluster
	cmd = az("aks", "create",
		"--resource-group", azureRG,
		"--name", aksClusterName,
		"--node-count", "1",
		"--network-plugin", "azure",
		"--service-cidr", "10.0.0.0/16",
		"--dns-service-ip", "10.0.0.10",
		"--docker-bridge-address", "172.17.0.1/16",
		"--enable-managed-identity",
		"--assign-identity", managedIdentityURI,
		"--assign-kubelet-identity", managedIdentityURI,
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Log("aks cluster created")

	//connect cluster
	cmd = az("aks", "get-credentials",
		"--resource-group", azureRG,
		"--name", aksClusterName,
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Log("connected to cluster")

	//get master URI
	cmd = kubectl("cluster-info")

	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	clusterInfo := strings.Fields(string(out))[6] //this return the link with some invisible characters
	//delete invisible characters and save masterURI
	re := regexp.MustCompile("\\x1B\\[[0-9;]*[a-zA-Z]")
	masterURI := re.ReplaceAllString(clusterInfo, "")

	//create virtual node
	cmd = helm("install", virtualNodeReleaseName, chartURL,
		"--set", "provider=azure",
		"--set", "rbac.install=true",
		"--set", "enableAuthenticationTokenWebhook=false",
		"--set", "providers.azure.targetAKS=true",
		"--set", "providers.azure.clientId="+clientID,
		"--set", "providers.azure.clientKey="+azureClientSecret,
		"--set", "providers.azure.masterUri="+masterURI,
		"--set", "providers.azure.aciResourceGroup="+azureRG,
		"--set", "providers.azure.aciRegion="+region,
		"--set", "providers.azure.tenantId="+tenantID,
		"--set", "providers.azure.subscriptionId="+subscriptionID,
		"--set", "nodeName="+nodeName,
		"--set", "image.repository="+imageRepository,
		"--set", "image.name="+imageName,
		"--set", "image.tag="+imageTag,
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Log(string(out))

	//test pod lifecycle
	CreatePodFromKubectl(t, "mi-pull-image", "fixtures/mi-pull-image.yaml")
	DeletePodFromKubectl(t, "mi-pull-image")

	t.Log("deleting")

	kubectl("config", "use-context", string(previousCluster))

	kubectl("delete", "deployments", "--all")
	kubectl("delete", "pods", "--all")
	kubectl("delete", "node", nodeName)
	helm("uninstall", virtualNodeReleaseName)

	az("aks", "delete", "--name", aksClusterName, "--resource-group", azureRG, "--yes")
	kubectl("config", "delete-context", aksClusterName)
}
