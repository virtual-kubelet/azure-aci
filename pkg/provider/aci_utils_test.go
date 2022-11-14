/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"testing"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

func TestGetPodWithContainerID(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	podLister := NewMockPodLister(mockCtrl)

	mockPodsNamespaceLister := NewMockPodNamespaceLister(mockCtrl)
	podLister.EXPECT().Pods(podNamespace).Return(mockPodsNamespaceLister)
	mockPodsNamespaceLister.EXPECT().Get(podName).
		Return(testsutil.CreatePodObj(podName, podNamespace), nil)

	err := azConfig.SetAuthConfig()
	if err != nil {
		t.Fatal("failed to get auth configuration", err)
	}

	aciMocks := createNewACIMock()
	cgID := ""
	aciMocks.MockGetContainerGroupInfo = func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {

		cg := testsutil.CreateContainerGroupObj(podName, podNamespace, "Succeeded",
			testsutil.CreateACIContainersListObj("Running", "Initializing", testsutil.CgCreationTime.Add(time.Second*2), testsutil.CgCreationTime.Add(time.Second*3), false, false, false), "Succeeded")
		cgID = *cg.ID
		return cg, nil
	}

	resourceManager, err := manager.NewResourceManager(
		podLister,
		NewMockSecretLister(mockCtrl),
		NewMockConfigMapLister(mockCtrl),
		NewMockServiceLister(mockCtrl),
		NewMockPersistentVolumeClaimLister(mockCtrl),
		NewMockPersistentVolumeLister(mockCtrl))
	if err != nil {
		t.Fatal("Unable to prepare the mocks for resourceManager", err)
	}

	provider, err := createTestProvider(aciMocks, resourceManager)
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
