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
	kubectl("delete", "pod/vk-e2e-hpa", "--namespace=vk-test")

	CreatePodFromKubectl(t, "vk-e2e-hpa", "fixtures/hpa.yml")
	//QueryKubectlMetrics(t, "vk-e2e-hpa")
	CleanPodFromKubectl(t, "vk-e2e-hpa")
}

func TestInitContainerPod(t *testing.T) {
	kubectl("delete", "pod/vk-e2e-initcontainers", "--namespace=vk-test")

	CreatePodFromKubectl(t, "vk-e2e-initcontainers", "fixtures/initcontainers_pod.yml")
	//QueryKubectlMetrics(t, "vk-e2e-initcontainers")
	CleanPodFromKubectl(t, "vk-e2e-initcontainers")
}
