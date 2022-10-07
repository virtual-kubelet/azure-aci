package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
			podMetricsProvider := NewACIPodMetricsProvider("node-1", "rg", mockedPodGetter, nil, nil)
			podMetricsProvider.podStatsGetter = mockedPodStatsGetter
			mockedPodGetter.EXPECT().GetPods().Return(fakePod(getMapKeys(test))).AnyTimes()
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

func TestPodStatsGetterDecider(t *testing.T) {
	t.Run("useContainerInsightAndContainerGroupCacheNotTakeEffective", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		ctx := context.Background()
		pod1 := fakePod([]string{"pod-1"})[0]
		pod2 := fakePod([]string{"pod-1"})[0]

		mockedAciCgGetter := NewMockContainerGroupGetter(ctrl)

		// Times(2) because we expect the Container Group not be cached
		mockedAciCgGetter.EXPECT().GetContainerGroup(
			gomock.Any(), gomock.Any(), gomock.Any()).Return(
			&client.ContainerGroupWrapper{
				ContainerGroupPropertiesWrapper: &client.ContainerGroupPropertiesWrapper{
					Extensions: []*client.Extension{
						{
							Properties: &client.ExtensionProperties{
								Type:    client.ExtensionTypeKubeProxy,
								Version: client.ExtensionVersion_1,
								Settings: map[string]string{
									client.KubeProxyExtensionSettingClusterCIDR: "10.240.0.0/16",
									client.KubeProxyExtensionSettingKubeVersion: client.KubeProxyExtensionKubeVersion,
								},
								ProtectedSettings: map[string]string{},
							},
						},
					},
				},
			}, nil).Times(2)
		mockedContainerInsights := NewMockpodStatsGetter(ctrl)
		mockedContainerInsights.EXPECT().GetPodStats(gomock.Any(), gomock.Any()).Return(fakePodStatus("pod-1", 0), nil).Times(2)

		decider := NewPodStatsGetterDecider(mockedContainerInsights, nil, "rg", mockedAciCgGetter)

		decider.GetPodStats(ctx, pod1)

		/* this time use a pod with new UID but same name.
		we expect the Container Group will not be cached
		*/
		decider.GetPodStats(ctx, pod2)
	})

	t.Run("useRealtimeMetricsAndContainerGroupCacheTakeEffective", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockedAciCgGetter := NewMockContainerGroupGetter(ctrl)

		// Times(1) here because we expect the Container Group be cached
		mockedAciCgGetter.EXPECT().GetContainerGroup(
			gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeContainerGroupWrapper(), nil).Times(1)

		mockedRealtime := NewMockpodStatsGetter(ctrl)
		mockedRealtime.EXPECT().GetPodStats(gomock.Any(), gomock.Any()).Return(fakePodStatus("pod-1", 0), nil).Times(1)
		mockedRealtime.EXPECT().GetPodStats(gomock.Any(), gomock.Any()).Return(fakePodStatus("pod-1", 0), nil).Times(1)

		decider := NewPodStatsGetterDecider(nil, mockedRealtime, "rg", mockedAciCgGetter)
		ctx := context.Background()
		pod := fakePod([]string{"pod-1"})[0]
		decider.GetPodStats(ctx, pod)
		decider.GetPodStats(ctx, pod)
	})
}

func fakePod(podNames []string) []*v1.Pod {
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

func fakeContainerGroupWrapper() *client.ContainerGroupWrapper {
	return &client.ContainerGroupWrapper{
		ContainerGroupPropertiesWrapper: &client.ContainerGroupPropertiesWrapper{
			Extensions: []*client.Extension{
				{
					Properties: &client.ExtensionProperties{
						Type:    client.ExtensionTypeKubeProxy,
						Version: client.ExtensionVersion_1,
						Settings: map[string]string{
							client.KubeProxyExtensionSettingClusterCIDR: "10.240.0.0/16",
							client.KubeProxyExtensionSettingKubeVersion: client.KubeProxyExtensionKubeVersion,
						},
						ProtectedSettings: map[string]string{},
					},
				},
				{
					Properties: &client.ExtensionProperties{
						Type:              client.ExtensionTypeRealtimeMetrics,
						Version:           client.ExtensionVersion_1,
						Settings:          map[string]string{},
						ProtectedSettings: map[string]string{},
					},
				},
			},
		},
	}
}

func getMapKeys(m map[string]uint64) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}
