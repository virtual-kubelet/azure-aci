package e2e

import (
	"strings"
	"testing"
)

func TestInitVaidationContainer(t *testing.T) {
	cmd := kubectl("get", "pod", "-l", "app=virtual-kubelet-azure-aci", "-n", "vk-azure-aci", "-o", "jsonpath={.items[0].status.phase}")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "Running" {
		t.Fatal("pod is not in running state")
	}

	cmd = kubectl("logs", "-l", "app=virtual-kubelet-azure-aci", "-c", "init-validation", "-n", "vk-azure-aci", "--tail=1")
	logOut, logErr := cmd.CombinedOutput()
	if logErr != nil {
		t.Fatal(string(out))
	}
	if !strings.Contains(string(logOut), "initial setup for virtual kubelet Azure ACI is successful") {
		t.Fatalf("init container setup is not successful. log: %s", string(logOut))
	}
}
