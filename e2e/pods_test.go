package e2e

import (
	"testing"
)

func TestPodLifecycle(t *testing.T) {
	// delete the pod first
	/*kubectl("delete", "pod/vk-e2e-hpa")

	spec, err := fixtures.ReadFile("fixtures/hpa.yml")
	if err != nil {
		t.Fatal(err)
	}

	cmd := kubectl("apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(spec)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/vk-e2e-hpa", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod")

	// query metrics
	deadline = time.Now().Add(5 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/vk-e2e-hpa")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Fatal("failed to query pod's stats from metris server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
		time.Sleep(10 * time.Second)
	}

	t.Log("clean up pod")
	cmd = kubectl("delete", "pod/vk-e2e-hpa", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}*/
}

func TestDeploymentUsingSecretsAndKubeletIdentity(t *testing.T) {
	//cmd := kubectl("config", "current-context")
	//previousCluster, _ := cmd.CombinedOutput()

	azureRG := "aci-virtual-node-test-rg"
	aksClusterName := "virtualKubeletE2ETestCluster"
	azureClientID := "d1464cac-2a02-4e77-a1e3-c6a9220e99b9"

	azureClientSecret := ""

	//managedIdentity := ""

	//create cluster
	cmd := az("aks", "create",
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
	  		"--set provider=azure",
	  		"--set rbac.install=true",
	  		"--set enableAuthenticationTokenWebhook=false",
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

	//erase cluster

	//kubectl("config", "use-context", string(previousCluster))
}
