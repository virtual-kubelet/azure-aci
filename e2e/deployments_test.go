package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

type KeyVault struct {
	Value string `json:"value"`
}

func TestImagePullUsingKubeletIdentityAndSecrets(t *testing.T) {
	/*cmd := kubectl("config", "current-context")
	previousCluster, _ := cmd.CombinedOutput()*/

	subscriptionID := "076cd026-379c-4383-8bec-8835382efe90"
	tenantID := "72f988bf-86f1-41af-91ab-2d7cd011db47"
	clientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"

	region := "westus"
	azureRG := "aci-virtual-node-test-rg"

	//aksClusterName := "aksClusterE2E05"
	nodeName := "virtual-kubelet"
	virtualNodeReleaseName := "virtual-kubelet-e2etest-aks"

	vkRelease := "virtual-kubelet-latest"
	chartURL := "https://github.com/virtual-kubelet/azure-aci/raw/master/charts/" + vkRelease + ".tgz"

	//get client secret
	vaultName := "aci-virtual-node-test-kv"
	secretName := "aci-virtualnode-sp-dev-credential"
	cmd := az("keyvault", "secret", "show", "--name", secretName, "--vault-name", vaultName, "-o", "json")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	var keyvault KeyVault
	json.Unmarshal(out, &keyvault)
	azureClientSecret := keyvault.Value

	t.Log(azureClientSecret)

	//create MI with role assignment
	/*managedIdentity := "e2eDeployTestMI"

	cmd = az("identity", "create", "--resource-group", azureRG, "--name", managedIdentity)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	spID, _ := az("identity", "show", "--resource-group", azureRG, "--name", managedIdentity,
		"--query", "principalID", "--output", "tsv").CombinedOutput()

	az("identity", "role", "assignment", "create", "--assignee", string(spID),
		"--scope", "<registry-id>", "--role", "acrpull")

	managedIdentityURI := "/subscriptions/" + subscriptionID + "/resourcegroups/" + azureRG + "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/" + managedIdentity

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
	t.Log("connected to cluster")*/

	//get master URI
	cmd = kubectl("cluster-info")

	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	clusterInfo := strings.Fields(string(out))
	masterURI := clusterInfo[6] //the 7th string has the masterURI on the response of doing 'kubectl cluster-info'

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
		"--set", "image.repository=docker.io",
		"--set", "image.name=ysalazar/virtual-kubelet",
		"--set", "image.tag=test",
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	t.Log(string(out))

	//test pod lifecycle

	/*t.Log("deleting")

	kubectl("config", "use-context", string(previousCluster))*/

	/*az("identity", "delete", "--resource-group", azureRG, "--name", managedIdentity)
	az("aks", "delete", "--name", aksClusterName, "--resource-group", azureRG, "--yes")
	helm("uninstall", virtualNodeReleaseName)*/
}
