package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWindowsPodLifecycle(t *testing.T) {
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

	winNodeName := os.Getenv("TEST_WINDOWS_NODE_NAME")
	cmd = exec.Command("sed", "-i", "-e", fmt.Sprintf("s|TEST_WINDOWS_NODE_NAME|%s|g", winNodeName), "fixtures/win-pod.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	cmd = kubectl("apply", "-f", "fixtures/win-pod.yml", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/vk-e2e-windows", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod")

	// check container logs
	t.Log("get container logs ....")
	cmd = kubectl("logs", "pod/vk-e2e-windows", "--namespace=vk-test", "--tail=5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) == "" {
		t.Fatal("failed to get container's logs")
	}
	t.Logf("success query container logs %s", string(out))

	// check pod status
	t.Log("get pod status ....")
	cmd = kubectl("get", "pod", "--field-selector=status.phase=Running", "--namespace=vk-test", "--output=jsonpath={.items..metadata.name}")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "vk-e2e-windows" {
		t.Fatal("failed to get pod's status")
	}
	t.Logf("success query pod status %s", string(out))

	// check container status
	t.Log("get container status ....")
	cmd = kubectl("get", "pod", "vk-e2e-windows", "--namespace=vk-test", "--output=jsonpath={.status.containerStatuses[0].ready}")
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
