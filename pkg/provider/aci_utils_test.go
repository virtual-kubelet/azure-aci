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
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/google/uuid"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestGetPodWithContainerID(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	containerName := "c-" + uuid.New().String()
	containerImage := "ci-" + uuid.New().String()

	err := azConfig.SetAuthConfig()
	if err != nil {
		t.Fatal("failed to get auth configuration", err)
	}

	cgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerInstance/containerGroups/%s-%s", azConfig.AuthConfig.SubscriptionID, fakeResourceGroup, podNamespace, podName)

	provisioning := "Creating"
	port := int32(80)
	cpu := float64(0.99)
	memory := float64(1.5)
	count := int32(5)
	successState := "Succeeded"
	currentTime := date.Time{
		Time: time.Now(),
	}

	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupInfo = func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
		node := fakeNodeName
		return &azaci.ContainerGroup{
			ID: &cgID,
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &successState,
				InstanceView: &azaci.ContainerGroupPropertiesInstanceView{
					State: &successState,
				},
				Containers: &[]azaci.Container{
					{
						Name: &containerName,
						ContainerProperties: &azaci.ContainerProperties{
							InstanceView: &azaci.ContainerPropertiesInstanceView{
								CurrentState: &azaci.ContainerState{
									State:        &successState,
									StartTime:    &currentTime,
									FinishTime:   &currentTime,
									DetailStatus: &podName,
								},
								PreviousState: &azaci.ContainerState{
									State:        &provisioning,
									StartTime:    &currentTime,
									DetailStatus: &podName,
								},
								RestartCount: &count,
								Events:       &[]azaci.Event{},
							},
							Image:   &containerImage,
							Command: &[]string{"nginx", "-g", "daemon off;"},
							Ports: &[]azaci.ContainerPort{
								{
									Protocol: azaci.ContainerNetworkProtocolTCP,
									Port:     &port,
								},
							},
							Resources: &azaci.ResourceRequirements{
								Requests: &azaci.ResourceRequests{
									CPU:        &cpu,
									MemoryInGB: &memory,
									Gpu: &azaci.GpuResource{
										Count: &count,
										Sku:   azaci.GpuSkuP100,
									},
								},
							},
							LivenessProbe:  &azaci.ContainerProbe{},
							ReadinessProbe: &azaci.ContainerProbe{},
						},
					},
				},
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
	assert.Check(t, is.Equal(containerName, pod.Status.ContainerStatuses[0].Name), "Container name in the container status doesn't match")
	assert.Check(t, is.Equal(containerImage, pod.Status.ContainerStatuses[0].Image), "Container image in the container status doesn't match")
	assert.Check(t, is.Equal(getContainerID(&cgID, &containerName), pod.Status.ContainerStatuses[0].ContainerID), "Container ID in the container status is not expected")
}
