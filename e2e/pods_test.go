package e2e

import (
	"testing"
	"time"
)

func CreatePodFromKubectl(t *testing.T, podName string, podDir string) {
	cmd := kubectl("apply", "-f", podDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/"+podName, "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	t.Log("success create pod")
}

func QueryKubectlMetrics(t *testing.T, podName string) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd := kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/"+podName)
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
}

func CleanPodFromKubectl(t *testing.T, podName string) {
	t.Log("clean up pod")
	cmd := kubectl("delete", "pod/"+podName, "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}

func TestPodLifecycle(t *testing.T) {
	podName := "vk-e2e-hpa"
	podDir := "fixtures/hpa.yml"

	kubectl("delete", "pod/"+podName, "--namespace=vk-test")

	CreatePodFromKubectl(t, podName, podDir)
	QueryKubectlMetrics(t, podName)
	CleanPodFromKubectl(t, podName)
}

func TestInitContainerPod(t *testing.T) {
	podName := "vk-e2e-initcontainers"
	podDir := "fixtures/initcontainers_pod.yml"

	kubectl("delete", "pod/"+podName, "--namespace=vk-test")

	CreatePodFromKubectl(t, podName, podDir)
	QueryKubectlMetrics(t, podName)
	CleanPodFromKubectl(t, podName)
}
