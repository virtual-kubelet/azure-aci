package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
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

			podLister := NewMockPodGetter(ctrl)
			mockedPodStatsGetter := NewMockpodStatsGetter(ctrl)
			podMetricsProvider := NewACIPodMetricsProvider("node-1", "rg", podLister, nil)
			podMetricsProvider.podStatsGetter = mockedPodStatsGetter
			podLister.EXPECT().List(gomock.Any()).Return(fakePod(getMapKeys(test)), nil)
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

func TestGetMetricsResource(t *testing.T) {
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
			podMetricsProvider.podStatsGetter = mockedPodStatsGetter
			mockedPodGetter.EXPECT().GetPods().Return(fakePod(getMapKeys(test))).AnyTimes()
			for podName, cpu := range test {
				mockedPodStatsGetter.EXPECT().GetPodStats(gomock.Any(), podNameEq(podName)).Return(fakePodStatus(podName, cpu), nil)
			}
			ctx := context.Background()
			actuallyMetricsResource, err := podMetricsProvider.GetMetricsResource(ctx)
			assert.NilError(t, err)
			for _, metricFamily := range actuallyMetricsResource {
				if *metricFamily.Name == "pod_cpu_usage_seconds_total" {
					assert.Equal(t, uint64(*metricFamily.Metric[0].Counter.Value), test[*metricFamily.Metric[0].Label[1].Value])
					assert.Equal(t, uint64(*metricFamily.Metric[1].Counter.Value), test[*metricFamily.Metric[1].Label[1].Value])
				}
				if *metricFamily.Name == "pod_memory_working_set_types" {
					assert.Equal(t, uint64(*metricFamily.Metric[0].Gauge.Value), test[*metricFamily.Metric[0].Label[1].Value])
					assert.Equal(t, uint64(*metricFamily.Metric[1].Gauge.Value), test[*metricFamily.Metric[1].Label[1].Value])
				}
				if *metricFamily.Name == "container_cpu_usage_seconds_total" {
					assert.Equal(t, uint64(*metricFamily.Metric[0].Counter.Value), test[*metricFamily.Metric[0].Label[2].Value])
					assert.Equal(t, uint64(*metricFamily.Metric[1].Counter.Value), test[*metricFamily.Metric[1].Label[2].Value])
				}
				if *metricFamily.Name == "container_memory_working_set_types" {
					assert.Equal(t, uint64(*metricFamily.Metric[0].Gauge.Value), test[*metricFamily.Metric[0].Label[2].Value])
					assert.Equal(t, uint64(*metricFamily.Metric[1].Gauge.Value), test[*metricFamily.Metric[1].Label[2].Value])
				}
				if *metricFamily.Name == "container_start_time_seconds" {
					assert.Check(t, metricFamily.Metric[0].Gauge.Value != nil)
					assert.Check(t, metricFamily.Metric[1].Gauge.Value != nil)
				}
			}
		})
	}
}
func TestPodStatsGetterDecider(t *testing.T) {
	t.Run("useRealtimeMetricsAndContainerGroupCacheTakeEffective", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockedAciCgGetter := NewMockContainerGroupGetter(ctrl)

		// Times(1) here because we expect the Container Group be cached
		mockedAciCgGetter.EXPECT().GetContainerGroup(
			gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeContainerGroup(), nil).Times(1)

		mockedRealtime := NewMockpodStatsGetter(ctrl)
		mockedRealtime.EXPECT().GetPodStats(gomock.Any(), gomock.Any()).Return(fakePodStatus("pod-1", 0), nil).Times(1)
		mockedRealtime.EXPECT().GetPodStats(gomock.Any(), gomock.Any()).Return(fakePodStatus("pod-1", 0), nil).Times(1)

		decider := NewPodStatsGetterDecider(mockedRealtime, "rg", mockedAciCgGetter)
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
	nanosec := cpu * 1e9
	return &stats.PodStats{
		PodRef: stats.PodReference{
			Name: podName,
		},
		CPU: &stats.CPUStats{
			UsageNanoCores: &cpu,
			UsageCoreNanoSeconds: &nanosec,
		},
		Memory: &stats.MemoryStats{
			WorkingSetBytes: &cpu,
		},
		Containers: []stats.ContainerStats{
			stats.ContainerStats{
				Name: "testcontainer",
				StartTime: metav1.NewTime(time.Now()),
				CPU: &stats.CPUStats{
					UsageNanoCores: &cpu,
					UsageCoreNanoSeconds: &nanosec,
				},
				Memory: &stats.MemoryStats{
					WorkingSetBytes: &cpu,
				},
			},
		},
	}
}

func fakeContainerGroup() *azaciv2.ContainerGroup {
	return &azaciv2.ContainerGroup{
		Properties: &azaciv2.ContainerGroupPropertiesProperties{
			Extensions: []*azaciv2.DeploymentExtensionSpec{
				{
					Properties: &azaciv2.DeploymentExtensionSpecProperties{
						ExtensionType: &client.ExtensionTypeKubeProxy,
						Version:       &client.ExtensionVersion_1,
						Settings: map[string]string{
							client.KubeProxyExtensionSettingClusterCIDR: "10.240.0.0/16",
							client.KubeProxyExtensionSettingKubeVersion: client.KubeProxyExtensionKubeVersion,
						},
						ProtectedSettings: map[string]string{},
					},
				},
				{
					Properties: &azaciv2.DeploymentExtensionSpecProperties{
						ExtensionType:     &client.ExtensionTypeRealtimeMetrics,
						Version:           &client.ExtensionVersion_1,
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
