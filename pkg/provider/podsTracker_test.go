package provider

import (
	"context"
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
