package e2e

import (
	"testing"
)

func TestMIDeploymentUsingSecretsAndKubeletIdentity(t *testing.T) {
	cmd := kubectl("config", "current-context")
	previousCluster, _ := cmd.CombinedOutput()

	aksClusterName := "aksClusterE2E01"

	azureRG := "aci-virtual-node-test-rg"
	azureClientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"

	azureClientSecret := ""

	//managedIdentity := ""

	//create cluster
	cmd = az("aks", "create",
		"--resource-group", azureRG,
		"--name", aksClusterName,
		"--node-count", "1",
		"--network-plugin", "azure",
		"--service-cidr", "10.0.0.0/16",
		"--dns-service-ip", "10.0.0.10",
		"--docker-bridge-address", "172.17.0.1/16",
		"--service-principal", azureClientID,
		"--client-secret", azureClientSecret,

		/*"--enable-managed-identity",
		"–assign-identity "+managedIdentity,
		"–assign-kubelet-identity "+managedIdentity,*/
	)
	out, err := cmd.CombinedOutput()
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
	  		"--set provider=azure",aksClusterNamek=false",
	  		"--set providers.azure.targetAKS=true",
	  		"--set providers.azure.clientId=$AZURE_CLIENT_ID",
	  		"--set providers.azure.clientKey=$AZURE_CLIENT_SECRET",
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

	t.Log("deleting cluster")

	kubectl("config", "use-context", string(previousCluster))
	az("aks", "delete", "--name", aksClusterName, "--resource-group", azureRG, "--yes", "--no-wait")
}
