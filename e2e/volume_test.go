package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPodWithCSIDriver(t *testing.T) {
	// delete the pod first
	cmd := kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	testStorageAccount := os.Getenv("CSI_DRIVER_STORAGE_ACCOUNT_NAME")
	testStorageKey := os.Getenv("CSI_DRIVER_STORAGE_ACCOUNT_KEY")

	// create namespace
	cmd = kubectl("apply", "-f", "fixtures/namespace.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("create", "secret", "generic", "csidriversecret", "--from-literal", "azurestorageaccountname="+testStorageAccount, "--from-literal", "azurestorageaccountkey="+testStorageKey, "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("apply", "-f", "fixtures/csi-driver.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/vk-e2e-csi-driver", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod with CSI driver")

	// query metrics
	deadline = time.Now().Add(5 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/vk-e2e-csi-driver")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Fatal("failed to query pod's stats from metrics server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
	}

	t.Log("clean up pod")
	cmd = kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}

func TestPodWithMultiVolume(t *testing.T) {
	// delete the pod first
	cmd := kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	// create namespace
	cmd = kubectl("apply", "-f", "fixtures/namespace.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("apply", "-f", "fixtures/multi-volume.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/vk-e2e-volume", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod with multi-volume")

	cmd = kubectl("exec", "pod/vk-e2e-volume", "-c", "hpa-example", "--namespace=vk-test", "--", "ls /var/run/secrets/kubernetes.io/serviceaccount")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if strings.Contains(string(out), "No such file or directory") {
		t.Fatal("failed to list mounted volumes")
	}
	t.Log("success exec mounted files for pod with multi-volume")

	// query metrics
	deadline = time.Now().Add(5 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/vk-e2e-volume")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Fatal("failed to query pod's stats from metrics server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
	}

	t.Log("clean up pod")
	cmd = kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}
