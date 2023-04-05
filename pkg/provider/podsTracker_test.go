package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
)

func TestUpdatePodStatus(t *testing.T) {
	podNames := []string{"p1", "p2"}
	podNamespace := "ns-" + uuid.New().String()

	cases := []struct {
		description string
		podName     string
		shouldFail  bool
		failMessage string
	}{
		{
			description: "pod is found in the list and successfully updated",
			podName:     "p2",
			shouldFail:  false,
			failMessage: "",
		},
		{
			description: "pod is not found in list",
			podName:     "fakePod",
			shouldFail:  true,
			failMessage: "pod not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			podLister := NewMockPodLister(mockCtrl)
			podLister.EXPECT().List(gomock.Any()).Return(testsutil.CreatePodsList(podNames, podNamespace), nil)

			podsTracker := &PodsTracker{
				pods:     podLister,
				updateCb: func(p *v1.Pod) {},
			}

			err := podsTracker.UpdatePodStatus(context.Background(), podNamespace, tc.podName, func(podStatus *v1.PodStatus) {}, true)
			if !tc.shouldFail {
				assert.Check(t, err == nil, "Not expected to return error")
			} else {
				assert.Check(t, err != nil, "expecting an error")
				assert.Check(t, strings.Contains(err.Error(), tc.failMessage), "failed message is expected")
			}
		})
	}
}

func TestProcessPodUpdates(t *testing.T) {
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	aciMocks := createNewACIMock()

	aciProvider, err := createTestProvider(aciMocks, NewMockConfigMapLister(mockCtrl),
		NewMockSecretLister(mockCtrl), NewMockPodLister(mockCtrl))
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	pod := testsutil.CreatePodObj(podName, podNamespace)
	containersList := testsutil.CreateACIContainersListObj(runningState, "Initializing",
		testsutil.CgCreationTime.Add(time.Second*2), testsutil.CgCreationTime.Add(time.Second*3),
		true, true, true)

	cases := []struct {
		description             string
		podPhase                v1.PodPhase
		getContainerGroupMock   func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error)
		shouldProcessPodUpdates bool
	}{
		{
			description: "Pod is updated after retrieving the pod status from the provider",
			podPhase:    v1.PodPending,
			getContainerGroupMock: func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error) {
				return testsutil.CreateContainerGroupObj(podName, podNamespace, "Succeeded", containersList, "Succeeded"), nil
			},
			shouldProcessPodUpdates: true,
		},
		{
			description: "Pod is updated after provider cannot retrieve the pod status but the pod is in a running state",
			podPhase:    v1.PodRunning,
			getContainerGroupMock: func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error) {
				return nil, errdefs.NotFound("cg is not found")
			},
			shouldProcessPodUpdates: true,
		},
		{
			description:             "Pod status update is skipped because pod has reached a failed phase",
			podPhase:                v1.PodFailed,
			shouldProcessPodUpdates: false,
		},
		{
			description: "Pod is not updated because pod status could not be retrieved from the provider and pod is not in running phase",
			podPhase:    v1.PodPending,
			getContainerGroupMock: func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error) {
				return nil, errdefs.NotFound("cg is not found")
			},
			shouldProcessPodUpdates: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			pod.Status.Phase = tc.podPhase
			aciMocks.MockGetContainerGroupInfo = tc.getContainerGroupMock

			podLister := NewMockPodLister(mockCtrl)

			podsTracker := &PodsTracker{
				pods:     podLister,
				updateCb: func(p *v1.Pod) {},
				handler:  aciProvider,
			}

			podUpdated := podsTracker.processPodUpdates(context.Background(), pod)

			if !tc.shouldProcessPodUpdates {
				assert.Equal(t, podUpdated, false, "pod should not be updated because it was not processed")
			} else {
				assert.Equal(t, podUpdated, true, "pod should be updated")

				if tc.podPhase == v1.PodPending {
					assert.Equal(t, pod.Status.Phase, v1.PodSucceeded, "Pod phase should be set to succeeded")
					assert.Check(t, pod.Status.Conditions != nil, "podStatus conditions should be set")
					assert.Check(t, pod.Status.StartTime != nil, "podStatus start time should be set")
					assert.Check(t, pod.Status.ContainerStatuses != nil, "podStatus container statuses should be set")
					assert.Check(t, is.Equal(len(pod.Status.Conditions), 3), "3 pod conditions should be present")
				}

				if tc.podPhase == v1.PodRunning {
					assert.Equal(t, pod.Status.Phase, v1.PodFailed, "Pod status was not found so the pod phase should be set to failed")
					assert.Equal(t, pod.Status.Reason, statusReasonNotFound, "Pod status was not found so the pod reason should be set to not found")
					assert.Equal(t, pod.Status.Message, statusMessageNotFound, "Pod status was not found so the pod message should be set to not found")
				}
			}
		})
	}
}

func TestCleanupDanglingPods(t *testing.T) {
	podName1 := "pod-" + uuid.New().String()
	podName2 := "pod-" + uuid.New().String()
	danglingPodName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	podsNames := []string{podName1, podName2}
	k8sPods := testsutil.CreatePodsList(podsNames, podNamespace)

	activePods := testsutil.CreatePodsList([]string{danglingPodName}, podNamespace)
	activePods = append(activePods, k8sPods[0], k8sPods[1])

	cg1 := testsutil.CreateContainerGroupObj(podName1, podNamespace, "Succeeded",
		testsutil.CreateACIContainersListObj(runningState, "Initializing",
			testsutil.CgCreationTime.Add(time.Second*2),
			testsutil.CgCreationTime.Add(time.Second*3),
			false, false, false), "Succeeded")

	cg2 := testsutil.CreateContainerGroupObj(podName2, podNamespace, "Succeeded",
		testsutil.CreateACIContainersListObj(runningState, "Initializing",
			testsutil.CgCreationTime.Add(time.Second*2),
			testsutil.CgCreationTime.Add(time.Second*3),
			false, false, false), "Succeeded")

	cg3 := testsutil.CreateContainerGroupObj(danglingPodName, podNamespace, "Succeeded",
		testsutil.CreateACIContainersListObj(runningState, "Initializing",
			testsutil.CgCreationTime.Add(time.Second*2),
			testsutil.CgCreationTime.Add(time.Second*3),
			false, false, false), "Succeeded")

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	aciMocks := createNewACIMock()

	aciMocks.MockGetContainerGroupList = func(ctx context.Context, resourceGroup string) ([]*azaciv2.ContainerGroup, error) {
		var result []*azaciv2.ContainerGroup
		result = append(result, cg1, cg2, cg3)
		return result, nil
	}

	aciMocks.MockGetContainerGroup = func(ctx context.Context, resourceGroup, containerGroupName string) (*azaciv2.ContainerGroup, error) {
		switch containerGroupName {
		case podName1:
			return cg1, nil
		case podName2:
			return cg2, nil
		case danglingPodName:
			return cg3, nil
		default:
			return nil, nil
		}
	}

	aciMocks.MockDeleteContainerGroup = func(ctx context.Context, resourceGroup, cgName string) error {
		updatedActivePods := make([]*v1.Pod, 0)

		for _, pod := range activePods {
			podCgName := fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
			if podCgName != cgName {
				updatedActivePods = append(updatedActivePods, pod)
			}
		}

		activePods = updatedActivePods
		return nil
	}

	activePodsLister := NewMockPodLister(mockCtrl)
	k8sPodsLister := NewMockPodLister(mockCtrl)
	mockPodsNamespaceLister := NewMockPodNamespaceLister(mockCtrl)

	aciProvider, err := createTestProvider(aciMocks, NewMockConfigMapLister(mockCtrl),
		NewMockSecretLister(mockCtrl), activePodsLister)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}

	podsTracker := &PodsTracker{
		pods: k8sPodsLister,
		updateCb: func(updatedPod *v1.Pod) {
		},
		handler: aciProvider,
	}

	k8sPodsLister.EXPECT().List(gomock.Any()).Return(k8sPods, nil).AnyTimes()

	activePodsLister.EXPECT().Pods(podNamespace).Return(mockPodsNamespaceLister).AnyTimes()
	mockPodsNamespaceLister.EXPECT().Get(danglingPodName).Return(activePods[0], nil)
	mockPodsNamespaceLister.EXPECT().Get(podName1).Return(activePods[1], nil)
	mockPodsNamespaceLister.EXPECT().Get(podName2).Return(activePods[2], nil)

	aciProvider.tracker = podsTracker
	podsTracker.cleanupDanglingPods(context.Background())

	assert.Equal(t, len(activePods), 2, "The dangling pod should be deleted from activePods")
	assert.DeepEqual(t, activePods, k8sPods)
}
