package e2e

import (
	"strings"
	"testing"
)

func TestInitVaidationContainer(t *testing.T) {
	cmd := kubectl("get", "pod", "-l", "app=aci-connector-linux", "-n", "kube-system", "-o", "jsonpath={.items[1].status.phase}")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "Running" {
		t.Fatal("pod is not in running state")
	}

	cmd = kubectl("logs", "-l", "app=aci-connector-linux", "-c", "init-validation", "-n", "kube-system", "--tail=1")
	logOut, logErr := cmd.CombinedOutput()
	if logErr != nil {
		t.Fatal(string(out))
	}
	if !strings.Contains(string(logOut), "initial setup for virtual kubelet Azure ACI is successful") {
		t.Fatalf("init container setup is not successful. log: %s", string(logOut))
	}
}
