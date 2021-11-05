package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type podNameMatcher struct {
	podName string
}

func (m podNameMatcher) Matches(arg interface{}) bool {
	var pod *v1.Pod = arg.(*v1.Pod)
	return pod.Name == m.podName
}

func (m podNameMatcher) String() string {
	return fmt.Sprintf("pod name is equal to %v (%T)", m.podName, m.podName)
}
func podNameEq(podName string) gomock.Matcher {

	return podNameMatcher{
		podName: podName,
	}
}

func TestGetStatsSummary(t *testing.T) {
	testCases := map[string]map[string]uint64{
		"two pods cases": {
			"pod1": uint64(1000),
			"pod2": uint64(2000),
		},
	}

	for testName, test := range testCases {
		t.Run(testName, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockedPodGetter := NewMockPodGetter(ctrl)
			mockedPodStatsGetter := NewMockpodStatsGetter(ctrl)
			podMetricsProvider := NewACIPodMetricsProvider("node-1", "rg", mockedPodGetter, nil)
			podMetricsProvider.containerInsightsPodStatsGetter = mockedPodStatsGetter
			mockedPodGetter.EXPECT().GetPods().Return(fakePod(test)).AnyTimes()
			for podName, cpu := range test {
				mockedPodStatsGetter.EXPECT().GetPodStats(gomock.Any(), podNameEq(podName)).Return(fakePodStatus(podName, cpu), nil)
			}
			ctx := context.Background()
			actuallyStatsSummary, err := podMetricsProvider.GetStatsSummary(ctx)
			assert.NilError(t, err)
			for _, actualPod := range actuallyStatsSummary.Pods {
				assert.Equal(t, *actualPod.CPU.UsageNanoCores, test[actualPod.PodRef.Name])
			}
		})
	}

}

func fakePod(podNames map[string]uint64) []*v1.Pod {
	result := make([]*v1.Pod, 0, len(podNames))
	for podName, _ := range podNames {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              podName,
				Namespace:         "ns",
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
			},
		}
		result = append(result, pod)
	}
	return result
}

func fakePodStatus(podName string, cpu uint64) *stats.PodStats {
	return &stats.PodStats{
		PodRef: stats.PodReference{
			Name: podName,
		},
		CPU: &stats.CPUStats{
			UsageNanoCores: &cpu,
		},
	}
}

func newUInt64Pointer(value int) *uint64 {
	var u = uint64(value)
	return &u
}
