package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestUpdatePodStatus(t *testing.T) {
	podNames:= []string{"p1", "p2"}

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
			podLister.EXPECT().List(gomock.Any()).Return(fakePods(podNames), nil)

			podsTracker := &PodsTracker {
				pods: podLister,
				updateCb: func(p *v1.Pod) {},
			}

			err := podsTracker.UpdatePodStatus(context.Background(), "ns", tc.podName, func(podStatus *v1.PodStatus){}, true)
			if !tc.shouldFail {
				assert.Check(t, err == nil, "Not expected to return error")
			} else {
				assert.Check(t, err != nil, "expecting an error")
				assert.Check(t, strings.Contains(err.Error(), tc.failMessage), "failed message is expected")
			}
		})
	}
}

func fakePods(podNames []string) []*v1.Pod {
	result := make([]*v1.Pod, 0, len(podNames))
	for _, podName := range podNames {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              podName,
				Namespace:         "ns",
				CreationTimestamp: metav1.NewTime(time.Now()),
				UID:               types.UID(uuid.New().String()),
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
			},
		}
		result = append(result, pod)
	}
	return result
}