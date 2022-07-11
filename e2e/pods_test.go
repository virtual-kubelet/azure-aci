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

func TestDeployUsingSecrets(t *testing.T) {
	cmd := kubectl("config", "current-context")
	out, _ := cmd.CombinedOutput()
	t.Log(string(out))
}
