
package e2e

import (
	"testing"
	"time"
	"io/ioutil"
	"os/exec"
	"os"

	"gotest.tools/assert"
)

func TestPodWithInitContainersOrder(t *testing.T) {
	// delete the namespace first
	cmd := kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	// create namespace
	cmd = kubectl("apply", "-f", "fixtures/namespace.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	testStorageAccount := os.Getenv("CSI_DRIVER_STORAGE_ACCOUNT_NAME")
	testStorageKey := os.Getenv("CSI_DRIVER_STORAGE_ACCOUNT_KEY")

	cmd = kubectl("create", "secret", "generic", "csidriversecret", "--from-literal", "azurestorageaccountname="+testStorageAccount, "--from-literal", "azurestorageaccountkey="+testStorageKey, "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("apply", "-f", "fixtures/initcontainers_ordertest_pod.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/vk-e2e-initcontainers-order", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod")

	// query metrics
	deadline = time.Now().Add(10 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/vk-e2e-initcontainers-order")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Fatal("failed to query pod's stats from metrics server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
		time.Sleep(10 * time.Second)
	}

	// download file created by pod
	cmd = exec.Command("az", "storage", "file", "download", "--account-name", testStorageAccount, "--account-key", testStorageKey, "-s", "vncsidriversharename", "-p", "newfile.txt")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("file newfile.txt downloaded from storage account")

	file, err := ioutil.ReadFile("newfile.txt")
	if err != nil {
		t.Fatal("could not read downloaded file")
	}
	t.Log("read file content successfully")

	fileContent := string(file)
	expectedString := "Hi from init-container-01\nHi from init-container-02\nHi from container\n"
	assert.Equal(t, fileContent, expectedString, "file content doesn't match expected value")

	// check pod status
	t.Log("get pod status ....")
	cmd = kubectl("get", "pod", "--field-selector=status.phase=Running", "--namespace=vk-test", "--output=jsonpath={.items..metadata.name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "vk-e2e-initcontainers-order" {
		t.Fatal("failed to get pod's status")
	}
	t.Logf("success query pod status %s", string(out))

	// check container status
	t.Log("get container status ....")
	cmd = kubectl("get", "pod", "vk-e2e-initcontainers-order", "--namespace=vk-test", "--output=jsonpath={.status.containerStatuses[0].ready}")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "true" {
		t.Fatal("failed to get pod's status")
	}
	t.Logf("success query container status %s", string(out))

	t.Log("clean up pod")
	cmd = kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}
