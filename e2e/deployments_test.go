package e2e

import (
	"encoding/json"
	"testing"
)

type KeyVault struct {
	Value string `json:"value"`
}

func TestImagePullUsingSecretsAndKubeletIdentity(t *testing.T) {
	cmd := kubectl("config", "current-context")
	previousCluster, _ := cmd.CombinedOutput()

	subscriptionID := "076cd026-379c-4383-8bec-8835382efe90"
	//tenantID := "72f988bf-86f1-41af-91ab-2d7cd011db47"
	//clientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"

	aksClusterName := "aksClusterE2E01"

	azureRG := "aci-virtual-node-test-rg"
	//azureClientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"

	//get secret
	vaultName := "aci-virtual-node-test-kv"
	secretName := "aci-virtualnode-sp-dev-credential"
	cmd = az("keyvault", "secret", "show", "--name", secretName, "--vault-name", vaultName, "-o", "json")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	var keyvault KeyVault
	json.Unmarshal(out, &keyvault)
	//azureClientSecret := keyvault.Value

	//create MI with role assignment
	managedIdentity := "e2eDeployTestMI"

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
	t.Log("connected to cluster")

	//create virtual node
	/*helm("install \"$RELEASE_NAME\" \"$CHART_URL\"",
	  		"--set", "provider=azure","aksClusterNamek=false",
	  		"--set", "providers.azure.targetAKS=true",
	  		"--set", "providers.azure.clientId=", clientID,
	  		"--set", "providers.azure.clientKey=", azureClientSecret,
	  		"--set providers.azure.masterUri=$MASTER_URI",
	  		"--set providers.azure.aciResourceGroup=$AZURE_RG".
			"--set providers.azure.aciRegion=$ACI_REGION",
	  		"--set providers.azure.tenantId=$AZURE_TENANT_ID",
	  		"--set providers.azure.subscriptionId=$AZURE_SUBSCRIPTION_ID",
	  		"--set nodeName=$NODE_NAME",
	  		"--set image.repository=docker.io",
	  		"--set image.name=suselva/virtual-kubelet",
	 		"--set image.tag=latest"
		)*/

	//test pod lifecycle

	t.Log("deleting")

	kubectl("config", "use-context", string(previousCluster))

	/*az("identity", "delete", "--resource-group", azureRG, "--name", managedIdentity)
	az("aks", "delete", "--name", aksClusterName, "--resource-group", azureRG, "--yes")*/
}
