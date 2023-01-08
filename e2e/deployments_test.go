package e2e

import (
	"testing"
	"time"

	"github.com/virtual-kubelet/azure-aci/pkg/featureflag"
)

func TestImagePullUsingKubeletIdentityMI(t *testing.T) {
	ctx := context.TODO()
	enabledFeatures := featureflag.InitFeatureFlag(ctx)
	if !enabledFeatures.IsEnabled(ctx, featureflag.ManagedIdentityPullFeature) {
		t.Skipf("%s feature is not enabled", featureflag.ManagedIdentityPullFeature)
	}
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

	// run container group pulling image from acr using MI
	cmd = kubectl("apply", "-f", "fixtures/mi-pull-image-exec.yaml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}

	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/e2etest-acr-test-mi-container", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success pulling image from ACR using managed identity")

	// query metrics
	deadline = time.Now().Add(5 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/e2etest-acr-test-mi-container")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Fatal("failed to query pod's stats from metrics server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
	}

	// check pod status
	t.Log("get pod status ....")
	cmd = kubectl("get", "pod", "--field-selector=status.phase=Running", "--namespace=vk-test", "--output=jsonpath={.items..metadata.name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "e2etest-acr-test-mi-container" {
		t.Fatal("failed to get pod's status")
	}
	t.Logf("success query pod status %s", string(out))

	// check container status
	t.Log("get container status ....")
	cmd = kubectl("get", "pod", "e2etest-acr-test-mi-container", "--namespace=vk-test", "--output=jsonpath={.status.containerStatuses[0].ready}")
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
