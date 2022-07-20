package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	subscriptionID = "076cd026-379c-4383-8bec-8835382efe90"
	tenantID       = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	clientID       = "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"
	azureRG        = "aci-virtual-node-test-rg"

	imageRepository = "docker.io"
	imageName       = "ysalazar/virtual-kubelet"
	imageTag        = "test"

	region = "westus"

	containerRegistry = "acivirtualnodetestregistry"

	nodeName               = "virtual-kubelet"
	virtualNodeReleaseName = "virtual-kubelet-e2etest-aks"

	vkRelease = "virtual-kubelet-latest"
	chartURL  = "https://github.com/virtual-kubelet/azure-aci/raw/master/charts/" + vkRelease + ".tgz"
)

type KeyVault struct {
	Value string `json:"value"`
}

func ConnectToAKSCluster(t *testing.T, clusterName string) {
	cmd := az("aks", "get-credentials",
		"--resource-group", azureRG,
		"--name", clusterName,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
}

func GetCurrentClusterMasterURI(t *testing.T) string {
	cmd := kubectl("cluster-info")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	clusterInfo := strings.Fields(string(out))[6]
	masterURI := cleanString(clusterInfo)

	return masterURI
}

func TestImagePull_KubeletIdentityInAKSCLuster(t *testing.T) {
	cmd := kubectl("config", "current-context")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	clusterInfo := strings.Fields(string(out))[0]
	previousCluster := cleanString(clusterInfo)

	testName := "ImagePull-KI26"
	aksClusterName := "aksClusterE2E-" + testName
	managedIdentity := "e2eDeployTestMI-" + testName

	//create MI with role assignment
	cmd = az("identity", "create", "--resource-group", azureRG, "--name", managedIdentity)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	spID, _ := az("identity", "show", "--resource-group", azureRG, "--name", managedIdentity,
		"--query", "principalId", "--output", "tsv").CombinedOutput()

	userID, _ := az("identity", "show", "--resource-group", azureRG, "--name", managedIdentity,
		"--query", "id", "--output", "tsv").CombinedOutput()
	managedIdentityURI := cleanString(strings.Fields(string(userID))[0])

	registryID, _ := az("acr", "show", "--resource-group", azureRG, "--name", containerRegistry, "--query",
		"id", "--output", "tsv").CombinedOutput()

	cmd = az("role", "assignment", "create", "--assignee-object-id", string(spID),
		"--scope", string(registryID), "--role", "acrpull", "--assignee-principal-type", "ServicePrincipal")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

	azureRGURI := "/subscriptions/" + subscriptionID + "/resourceGroups/" + azureRG
	cmd = az("role", "assignment", "create", "--assignee-object-id", string(spID),
		"--scope", azureRGURI, "--role", "Contributor", "--assignee-principal-type", "ServicePrincipal")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}

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

	ConnectToAKSCluster(t, aksClusterName)
	masterURI := GetCurrentClusterMasterURI(t)

	t.Run("virtual_node_with_secrets", func(t *testing.T) {
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

		//test pod lifecycle
		CreatePodFromKubectl(t, "mi-pull-image", "fixtures/mi-pull-image.yaml")
		DeletePodFromKubectl(t, "mi-pull-image")

		//delete virtual node
		kubectl("delete", "deployments", "--all")
		kubectl("delete", "pods", "--all")
		kubectl("delete", "node", nodeName)
		helm("uninstall", virtualNodeReleaseName)
	})

	t.Run("virtual_node_with_no_secrets", func(t *testing.T) {
		releaseName := virtualNodeReleaseName + "02"
		//create virtual node
		cmd = helm("install", releaseName, chartURL,
			"--set", "provider=azure",
			"--set", "rbac.install=true",
			"--set", "enableAuthenticationTokenWebhook=false",
			"--set", "providers.azure.targetAKS=true",
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

		//test pod lifecycle
		CreatePodFromKubectl(t, "mi-pull-image", "fixtures/mi-pull-image.yaml")
		DeletePodFromKubectl(t, "mi-pull-image")

		//delete virtual node
		kubectl("delete", "deployments", "--all")
		kubectl("delete", "pods", "--all")
		kubectl("delete", "node", nodeName)
		helm("uninstall", releaseName)
	})

	cmd = kubectl("config", "use-context", string(previousCluster))
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	cmd = kubectl("config", "delete-context", aksClusterName)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	cmd = az("aks", "delete", "--name", aksClusterName, "--resource-group", azureRG, "--yes", "--no-wait")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	cmd = az("identity", "delete", "--resource-group", azureRG, "--name", managedIdentity)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
}

func TestAKSDeployment_attachACR(t *testing.T) {
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

	aksClusterName := "aksClusterE2E-attachACR"
	//create cluster
	cmd = az("aks", "create",
		"--resource-group", azureRG,
		"--name", aksClusterName,
		"--node-count", "1",
		"--network-plugin", "azure",
		"--service-cidr", "10.0.0.0/16",
		"--dns-service-ip", "10.0.0.10",
		"--docker-bridge-address", "172.17.0.1/16",
		"--service-principal", clientID,
		"--client-secret", azureClientSecret,
		"--enable-managed-identity",
		"--attach-acr", containerRegistry,
	)
	_, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("error expected")
	}
}
