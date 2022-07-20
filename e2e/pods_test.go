package e2e

import (
	"os"
	"testing"
	"time"
)

type E2EPodTestCase struct {
	name string
	pod  E2EPodInfo
}

type E2EPodInfo struct {
	name string
	dir  string
}

func TestPodLifecycle(t *testing.T) {
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

	testCases := []E2EPodTestCase{
		{
			name: "basic pod",
			pod: E2EPodInfo{
				name: "vk-e2e-hpa",
				dir:  "fixtures/hpa.yml",
			},
		},
		{
			name: "pod with init container",
			pod: E2EPodInfo{
				name: "vk-e2e-initcontainers",
				dir:  "fixtures/initcontainers_pod.yml",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			cmd = kubectl("apply", "-f", test.pod.dir, "--namespace=vk-test")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatal(string(out))
			}

			deadline, ok := t.Deadline()
			timeout := time.Until(deadline)
			if !ok {
				timeout = 300 * time.Second
			}
			cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/"+test.pod.name, "--namespace=vk-test")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatal(string(out))
			}
			t.Log("success create pod")

			// query metrics
			deadline = time.Now().Add(5 * time.Minute)
			for {
				t.Log("query metrics ....")
				cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/"+test.pod.name)
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
		})
	}

	t.Log("clean up")
	cmd = kubectl("delete", "namespace", "vk-test", "--ignore-not-found")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
}

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
