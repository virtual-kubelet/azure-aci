/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/google/uuid"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestGetPodWithContainerID(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	err := azConfig.SetAuthConfig()
	if err != nil {
		t.Fatal("failed to get auth configuration", err)
	}

	cgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerInstance/containerGroups/%s-%s", azConfig.AuthConfig.SubscriptionID, fakeResourceGroup, podNamespace, podName)
	successState := "Succeeded"
	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupInfo = func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
		node := fakeNodeName
		return &azaci.ContainerGroup{
			Name: &name,
			ID:   &cgID,
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				IPAddress:         &azaci.IPAddress{IP: &testsutil.FakeIP},
				ProvisioningState: &successState,
				InstanceView: &azaci.ContainerGroupPropertiesInstanceView{
					State: &successState,
				},
				Containers: testsutil.CreateACIContainersListObj("Running", "Initializing", testsutil.CgCreationTime.Add(time.Second*2), testsutil.CgCreationTime.Add(time.Second*3), true, false, false),
			},
		}, nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, &pod != nil, "Response pod should not be nil")
	assert.Check(t, is.Equal(1, len(pod.Status.ContainerStatuses)), "1 container status is expected")
	assert.Check(t, is.Equal(testsutil.TestContainerName, pod.Status.ContainerStatuses[0].Name), "Container name in the container status doesn't match")
	assert.Check(t, is.Equal(testsutil.TestImageNginx, pod.Status.ContainerStatuses[0].Image), "Container image in the container status doesn't match")
	assert.Check(t, is.Equal(getContainerID(&cgID, &testsutil.TestContainerName), pod.Status.ContainerStatuses[0].ContainerID), "Container ID in the container status is not expected")
}
