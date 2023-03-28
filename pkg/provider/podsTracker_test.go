package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
)

func TestUpdatePodStatus(t *testing.T) {
	podNames:= []string{"p1", "p2"}
	podNamespace := "ns-" + uuid.New().String()

	cases := []struct {
		description		string
		podName			string
		shouldFail		bool
		failMessage		string
	}{
		{
			description: "pod is found in the list and successfully updated",
			podName: "p2",
			shouldFail: false,
			failMessage: "",
		},
		{
			description: "pod is not found in list",
			podName: "fakePod",
			shouldFail: true,
			failMessage: "pod not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			podLister := NewMockPodLister(mockCtrl)
			podLister.EXPECT().List(gomock.Any()).Return(testsutil.CreatePodsList(podNames, podNamespace), nil)

			podsTracker := &PodsTracker {
				pods: podLister,
				updateCb: func(p *v1.Pod) {},
			}

			err := podsTracker.UpdatePodStatus(context.Background(), podNamespace, tc.podName, func(podStatus *v1.PodStatus){}, true)
			if !tc.shouldFail {
				assert.Check(t, err == nil, "Not expected to return error")
			} else {
				assert.Check(t, err != nil, "expecting an error")
				assert.Check(t, strings.Contains(err.Error(), tc.failMessage), "failed message is expected")
			}
		})
	}
}
