/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/virtual-kubelet/azure-aci/pkg/featureflag"
)

func TestPodWithInitConfidentialContainer(t *testing.T) {
	ctx := context.TODO()
	enabledFeatures := featureflag.InitFeatureFlag(ctx)
	if !enabledFeatures.IsEnabled(ctx, featureflag.ConfidentialComputeFeature) {
		t.Skipf("%s feature is not enabled", featureflag.ConfidentialComputeFeature)
	}

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

	// create confidential container pod
	cmd = kubectl("apply", "-f", "fixtures/confidential_container_pod.yml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	deadline, ok := t.Deadline()
	timeout := time.Until(deadline)
	if !ok {
		timeout = 300 * time.Second
	}
	cmd = kubectl("wait", "--for=condition=ready", "--timeout="+timeout.String(), "pod/confidential-container-sevsnp", "--namespace=vk-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	t.Log("success create pod")

	// query metrics
	deadline = time.Now().Add(10 * time.Minute)
	for {
		t.Log("query metrics ....")
		cmd = kubectl("get", "--raw", "/apis/metrics.k8s.io/v1beta1/namespaces/vk-test/pods/confidential-container-sevsnp")
		out, err := cmd.CombinedOutput()
		if time.Now().After(deadline) {
			t.Log("ACI Pod logs:")
			c := kubectl("logs", "-l", "app=aci-connector-linux", "--namespace=kube-system", "--tail=20")
			l, _ := c.CombinedOutput()
			t.Log(string(l))

			t.Log("Confidential Container Pod logs:")
			c = kubectl("logs", "confidential-container-sevsnp", "--namespace=vk-test", "--tail=20")
			l, _ = c.CombinedOutput()
			t.Log(string(l))

			t.Fatal("failed to query pod's stats from metrics server API")
		}
		if err == nil {
			t.Logf("success query metrics %s", string(out))
			break
		}
		time.Sleep(10 * time.Second)
	}

	// check pod status
	t.Log("get pod status ....")
	cmd = kubectl("get", "pod", "--field-selector=status.phase=Running", "--namespace=vk-test", "--output=jsonpath={.items..metadata.name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	if string(out) != "confidential-container-sevsnp" {
		t.Fatal("failed to get pod's status")
	}
	t.Logf("success query pod status %s", string(out))

	// check container status
	t.Log("get container status ....")
	cmd = kubectl("get", "pod", "confidential-container-sevsnp", "--namespace=vk-test", "--output=jsonpath={.status.containerStatuses[0].ready}")
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
