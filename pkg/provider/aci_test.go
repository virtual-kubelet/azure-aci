/**
* Copyright (c) Microsoft.  All rights reserved.
	*/

package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"

	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	fakeResourceGroup = "vk-rg"
	fakeNodeName      = "vk"
	fakeRegion        = "westus"
)

var (
	creationTime = "2006-01-02 15:04:05.999999999 -0700 MST"
	azConfig     auth.Config
)

// Test make registry credential
func TestMakeRegistryCredential(t *testing.T) {
	server := "server-" + uuid.New().String()
	username := "user-" + uuid.New().String()
	password := "pass-" + uuid.New().String()
	authConfig := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))

	tt := []struct {
		name        string
		authConfig  AuthConfig
		shouldFail  bool
		failMessage string
	}{
		{
			"Valid username and password",
			AuthConfig{Username: username, Password: password},
			false,
			"",
		},
		{
			"Username and password in auth",
			AuthConfig{Auth: authConfig},
			false,
			"",
		},
		{
			"No Username",
			AuthConfig{},
			true,
			"no username present in auth config for server",
		},
		{
			"Invalid Auth",
			AuthConfig{Auth: "123"},
			true,
			"error decoding the auth for server",
		},
		{
			"Malformed Auth",
			AuthConfig{Auth: base64.StdEncoding.EncodeToString([]byte("123"))},
			true,
			"malformed auth for server",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := makeRegistryCredential(server, tc.authConfig)

			if tc.shouldFail {
				assert.Check(t, err != nil, "conversion should fail")
				assert.Check(t, strings.Contains(err.Error(), tc.failMessage), "failed message is not expected")
				return
			}

			assert.Check(t, err, "conversion should not fail")
			assert.Check(t, cred != nil, "credential should not be nil")
			assert.Check(t, is.Equal(server, *cred.Server), "server doesn't match")
			assert.Check(t, is.Equal(username, *cred.Username), "username doesn't match")
			assert.Check(t, is.Equal(password, *cred.Password), "password doesn't match")
		})
	}
}

// Tests create pod without resource spec
func TestCreatePodWithoutResourceSpec(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers[0]).Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.0, *(containers[0]).Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(1.5, *(containers[0]).Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Nil((containers[0]).Resources.Limits), "Limits should be nil")

		return nil
	}
	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with resource request only
func TestCreatePodWithResourceRequestOnly(t *testing.T) {

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "container group is nil")
		assert.Check(t, containers != nil, "container should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "only container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, containers[0].Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, *(containers[0]).Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, *(containers[0]).Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Nil(containers[0].Resources.Limits), "Limits should be nil")

		return nil
	}

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	ctx := context.Background()

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(ctx, pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with default GPU SKU.
func TestCreatePodWithGPU(t *testing.T) {

	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers[0]).Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, *(containers[0]).Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, *(containers[0]).Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, (containers[0]).Resources.Requests.Gpu != nil, "Requests GPU is not expected")
		assert.Check(t, is.Equal(int32(10), *(containers[0]).Resources.Requests.Gpu.Count), "Requests GPU Count is not expected")
		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							gpuResourceName: resource.MustParse("10"),
						},
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with GPU SKU in annotation.
func TestCreatePodWithGPUSKU(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	gpuSKU := azaci.GpuSkuP100

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers[0]).Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, *(containers[0]).Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, *(containers[0]).Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, (containers[0]).Resources.Requests.Gpu != nil, "Requests GPU is not expected")
		assert.Check(t, is.Equal(int32(1), *(containers[0]).Resources.Requests.Gpu.Count), "Requests GPU Count is not expected")
		assert.Check(t, is.Equal(gpuSKU, (containers[0]).Resources.Requests.Gpu.Sku), "Requests GPU SKU is not expected")
		assert.Check(t, (containers[0]).Resources.Limits.Gpu != nil, "Limits GPU is not expected")

		return nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			Annotations: map[string]string{
				gpuTypeAnnotation: string(gpuSKU),
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							gpuResourceName: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests create pod with both resource request and limit.
func TestCreatePodWithResourceRequestAndLimit(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers[0]).Resources.Requests != nil, "Container resource requests should not be nil")
		assert.Check(t, is.Equal(1.98, *(containers[0]).Resources.Requests.CPU), "Request CPU is not expected")
		assert.Check(t, is.Equal(3.4, *(containers[0]).Resources.Requests.MemoryInGB), "Request Memory is not expected")
		assert.Check(t, is.Equal(3.999, *(containers[0]).Resources.Limits.CPU), "Limit CPU is not expected")
		assert.Check(t, is.Equal(8.0, *(containers[0]).Resources.Limits.MemoryInGB), "Limit Memory is not expected")

		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							"cpu":    resource.MustParse("3999m"),
							"memory": resource.MustParse("8010M"),
						},
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

// Tests get pods with empty list.
func TestGetPodsWithEmptyList(t *testing.T) {
	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupList = func(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {
		var result []azaci.ContainerGroup
		return &result, nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pods, err := provider.GetPods(context.Background())
	if err != nil {
		t.Fatal("Failed to get pods", err)
	}

	assert.Check(t, pods != nil, "Response pods should not be nil")
	assert.Check(t, is.Equal(0, len(pods)), "No pod should be returned")
}

// Tests get pods without requests limit.
func TestGetPodsWithoutResourceRequestsLimits(t *testing.T) {
	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupList = func(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {
		cgName := "default-nginx"
		node := fakeNodeName
		provisioning := "Creating"
		containerName := "nginx"
		port := int32(80)
		cpu := float64(0.99)
		memory := float64(1.5)
		var cg = azaci.ContainerGroup{
			Name: &cgName,
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &cgName,
				"Namespace":         &cgName,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &cgName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &provisioning,
				Containers: &[]azaci.Container{
					{
						Name: &containerName,
						ContainerProperties: &azaci.ContainerProperties{
							Image:   &containerName,
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
								},
							},
						},
					},
				},
			},
		}
		var result []azaci.ContainerGroup
		result = append(result, cg)
		return &result, nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pods, err := provider.GetPods(context.Background())
	if err != nil {
		t.Fatal("Failed to get pods", err)
	}

	assert.Check(t, pods != nil, "Response pods should not be nil")
	assert.Check(t, is.Equal(0, len(pods)), "No pod should be returned")

}

// Tests get pod without requests limit.
func TestGetPodWithoutResourceRequestsLimits(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	node := fakeNodeName
	provisioning := "Creating"
	containerName := "nginx"
	port := int32(80)
	cpu := float64(0.99)
	memory := float64(1.5)

	aciMocks := createNewACIMock()
	aciMocks.MockGetContainerGroupInfo =
		func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
			return &azaci.ContainerGroup{
				Tags: map[string]*string{
					"CreationTimestamp": &creationTime,
					"PodName":           &podName,
					"Namespace":         &podNamespace,
					"ClusterName":       &node,
					"NodeName":          &node,
					"UID":               &podName,
				},
				ContainerGroupProperties: &azaci.ContainerGroupProperties{
					ProvisioningState: &provisioning,
					Containers: &[]azaci.Container{
						{
							Name: &containerName,
							ContainerProperties: &azaci.ContainerProperties{
								Image:   &containerName,
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
									},
								},
							},
						},
					},
				},
			}, nil
		}

	aciMocks.MockGetContainerGroupList = func(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {
		cg := azaci.ContainerGroup{
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &provisioning,
				Containers: &[]azaci.Container{
					{
						Name: &containerName,
						ContainerProperties: &azaci.ContainerProperties{
							Image:   &containerName,
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
								},
							},
						},
					},
				},
			},
		}
		var result []azaci.ContainerGroup
		result = append(result, cg)
		return &result, nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, pod.Spec.Containers != nil, "Containers should not be nil")
	assert.Check(t, is.Nil(pod.Spec.Containers[0].Resources.Limits), "Containers[0].Resources.Limits should be nil")
	assert.Check(t, pod.Spec.Containers[0].Resources.Requests != nil, "Containers[0].Resources.Requests should be nil")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("0.99")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Cpu().Value()), "Containers[0].Resources.Requests.CPU doesn't match")
	assert.Check(t, is.Equal(ptrQuantity(resource.MustParse("1.5G")).Value(),
		pod.Spec.Containers[0].Resources.Requests.Memory().Value()), "Containers[0].Resources.Requests.Memory doesn't match")
}

// Tests get pod with GPU.
func TestGetPodWithGPU(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	node := fakeNodeName
	provisioning := "Creating"
	containerName := "nginx"
	port := int32(80)
	cpu := float64(0.99)
	memory := float64(1.5)
	count := int32(5)

	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupInfo = func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
		return &azaci.ContainerGroup{
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &provisioning,
				Containers: &[]azaci.Container{
					{
						Name: &containerName,
						ContainerProperties: &azaci.ContainerProperties{
							Image:   &containerName,
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
								Limits: &azaci.ResourceLimits{
									CPU:        &cpu,
									MemoryInGB: &memory,
									Gpu: &azaci.GpuResource{
										Count: &count,
										Sku:   azaci.GpuSkuP100,
									},
								},
							},
						},
					},
				},
			},
		}, nil
	}
	aciMocks.MockGetContainerGroupList = func(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {

		cg := azaci.ContainerGroup{
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &provisioning,
				Containers: &[]azaci.Container{
					{
						Name: &containerName,
						ContainerProperties: &azaci.ContainerProperties{
							Image:   &containerName,
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
								Limits: &azaci.ResourceLimits{
									Gpu: &azaci.GpuResource{
										Count: &count,
										Sku:   azaci.GpuSkuP100,
									},
								},
							},
						},
					},
				},
			},
		}
		var result []azaci.ContainerGroup
		result = append(result, cg)
		return &result, nil
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod, err := provider.GetPod(context.Background(), podNamespace, podName)
	if err != nil {
		t.Fatal("Failed to get pod", err)
	}

	assert.Check(t, pod != nil, "Response pod should not be nil")
	assert.Check(t, pod.Spec.Containers != nil, "Containers should not be nil")
	assert.Check(t, pod.Spec.Containers[0].Resources.Requests != nil, "Containers[0].Resources.Requests should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("0.99")).Value(), pod.Spec.Containers[0].Resources.Requests.Cpu().Value()),
		"Containers[0].Resources.Requests.CPU doesn't match")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("1.5G")).Value(), pod.Spec.Containers[0].Resources.Requests.Memory().Value()),
		"Containers[0].Resources.Requests.Memory doesn't match")
	gpuQuantity, ok := pod.Spec.Containers[0].Resources.Requests[gpuResourceName]
	assert.Check(t, is.Equal(ok, true), "Containers[0].Resources.Requests.GPU should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("5")).Value(), ptrQuantity(gpuQuantity).Value()),
		"Containers[0].Resources.Requests.GPU.Count doesn't match")
	assert.Check(t, pod.Spec.Containers[0].Resources.Limits != nil, "Containers[0].Resources.Limits should not be nil")
	gpuQuantity, ok = pod.Spec.Containers[0].Resources.Limits[gpuResourceName]
	assert.Check(t, is.Equal(ok, true), "Containers[0].Resources.Requests.GPU should not be nil")
	assert.Check(
		t,
		is.Equal(ptrQuantity(resource.MustParse("5")).Value(), ptrQuantity(gpuQuantity).Value()),
		"Containers[0].Resources.Limits.GPU.Count doesn't match")
}

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
	assert.Check(t, is.Equal(getContainerID(cgID, containerName), pod.Status.ContainerStatuses[0].ContainerID), "Container ID in the container status is not expected")
}

func TestPodToACISecretEnvVar(t *testing.T) {

	testKey := "testVar"
	testVal := "testVal"

	e := v1.EnvVar{
		Name:  testKey,
		Value: testVal,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{},
		},
	}
	aciEnvVar := getACIEnvVar(e)

	if aciEnvVar.Value != nil {
		t.Fatalf("ACI Env Variable Value should be empty for a secret")
	}

	if *aciEnvVar.Name != testKey {
		t.Fatalf("ACI Env Variable Name does not match expected Name")
	}

	if *aciEnvVar.SecureValue != testVal {
		t.Fatalf("ACI Env Variable Secure Value does not match expected value")
	}
}

func TestPodToACIEnvVar(t *testing.T) {

	testKey := "testVar"
	testVal := "testVal"

	e := v1.EnvVar{
		Name:      testKey,
		Value:     testVal,
		ValueFrom: &v1.EnvVarSource{},
	}
	aciEnvVar := getACIEnvVar(e)

	if aciEnvVar.SecureValue != nil {
		t.Fatalf("ACI Env Variable Secure Value should be empty for non-secret variables")
	}

	if *aciEnvVar.Name != testKey {
		t.Fatalf("ACI Env Variable Name does not match expected Name")
	}

	if *aciEnvVar.Value != testVal {
		t.Fatalf("ACI Env Variable Value does not match expected value")
	}
}

func setAuthConfig() error {
	err := azConfig.SetAuthConfig()
	if err != nil {
		return err
	}
	return nil
}

func createNewACIMock() *MockACIProvider {
	return NewMockACIProvider(func(ctx context.Context, region string) (*[]azaci.Capabilities, error) {
		gpu := "P100"
		capability := azaci.Capabilities{
			Location: &region,
			Gpu:      &gpu,
		}
		var result []azaci.Capabilities
		result = append(result, capability)
		return &result, nil
	})
}

func createTestProvider(aciMocks *MockACIProvider, resourceManager *manager.ResourceManager) (*ACIProvider, error) {
	ctx := context.TODO()

	err := setAuthConfig()
	if err != nil {
		return nil, err
	}

	if resourceManager == nil {
		resourceManager, err = manager.NewResourceManager(nil, nil, nil, nil, nil, nil)
		if err != nil {
			return nil, err
		}
	}

	err = os.Setenv("ACI_RESOURCE_GROUP", fakeResourceGroup)
	if err != nil {
		return nil, err
	}
	err = os.Setenv("ACI_REGION", fakeRegion)
	if err != nil {
		return nil, err
	}

	provider, err := NewACIProvider(ctx, "example.toml", azConfig, aciMocks, resourceManager, fakeNodeName, "Linux", "0.0.0.0", 10250, "cluster.local")
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func ptrQuantity(q resource.Quantity) *resource.Quantity {
	return &q
}

func TestConfigureNode(t *testing.T) {

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "virtual-kubelet",
			Labels: map[string]string{
				"type":                   "virtual-kubelet",
				"kubernetes.io/role":     "agent",
				"kubernetes.io/hostname": "virtual-kubelet",
			},
		},
		Spec: v1.NodeSpec{},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				Architecture:   "amd64",
				KubeletVersion: "1.18.4",
			},
		},
	}
	aciMocks := createNewACIMock()
	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	provider.ConfigureNode(context.TODO(), node)
	assert.Equal(t, "true", node.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"], "exclude-balancer label doesn't match")
	assert.Equal(t, "true", node.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"], "exclude-from-external-load-balancers label doesn't match")
	assert.Equal(t, "false", node.ObjectMeta.Labels["kubernetes.azure.com/managed"], "kubernetes.azure.com/managed label doesn't match")
}

func TestCreatePodWithNamedLivenessProbe(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		assert.Check(t, (*cg.ContainerGroupProperties.Containers)[0].LivenessProbe != nil, "Liveness probe expected")
		assert.Check(t, is.Equal(int32(10), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(int32(5), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.PeriodSeconds), "Probe Period doesn't match")
		assert.Check(t, is.Equal(int32(60), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(int32(3), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(int32(5), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, (*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")
		assert.Check(t, is.Equal(int32(8080), *(*cg.ContainerGroupProperties.Containers)[0].LivenessProbe.HTTPGet.Port), "Expected Port to be 8080")
		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					Ports: []v1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
						},
					},
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromString("http"),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithLivenessProbe(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers)[0].LivenessProbe != nil, "Liveness probe expected")
		assert.Check(t, is.Equal(int32(10), *(containers)[0].LivenessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(int32(5), *(containers)[0].LivenessProbe.PeriodSeconds), "Probe Period doesn't match")
		assert.Check(t, is.Equal(int32(60), *(containers)[0].LivenessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(int32(3), *(containers)[0].LivenessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(int32(5), *(containers)[0].LivenessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, (containers)[0].LivenessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")

		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithReadinessProbe(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	aciMocks := createNewACIMock()

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, (containers)[0].ReadinessProbe != nil, "Readiness probe expected")
		assert.Check(t, is.Equal(int32(10), *(containers)[0].ReadinessProbe.InitialDelaySeconds), "Initial Probe Delay doesn't match")
		assert.Check(t, is.Equal(int32(5), *(containers)[0].ReadinessProbe.PeriodSeconds), "Probe Period doesn't match")
		assert.Check(t, is.Equal(int32(60), *(containers)[0].ReadinessProbe.TimeoutSeconds), "Probe Timeout doesn't match")
		assert.Check(t, is.Equal(int32(3), *(containers)[0].ReadinessProbe.SuccessThreshold), "Probe Success Threshold doesn't match")
		assert.Check(t, is.Equal(int32(5), *(containers)[0].ReadinessProbe.FailureThreshold), "Probe Failure Threshold doesn't match")
		assert.Check(t, (containers)[0].ReadinessProbe.HTTPGet != nil, "Expected an HTTP Get Probe")

		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}

	provider, err := createTestProvider(aciMocks, nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}
