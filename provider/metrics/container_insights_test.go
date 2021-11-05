package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/virtual-kubelet/azure-aci/client/aci"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type MockContainerGroupMetricsGetter struct {
	containersCPU    map[string][]aci.TimeSeriesEntry
	containersMemory map[string][]aci.TimeSeriesEntry
	podRx            []aci.TimeSeriesEntry
	podTx            []aci.TimeSeriesEntry
}

func (mockMetricsGetter *MockContainerGroupMetricsGetter) GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options aci.MetricsRequest) (*aci.ContainerGroupMetricsResult, error) {
	newMetricTimeseriesForMultipleContainers := func(containerTimeSeries map[string][]aci.TimeSeriesEntry) []aci.MetricTimeSeries {
		var result []aci.MetricTimeSeries = make([]aci.MetricTimeSeries, len(containerTimeSeries))
		for containerName, timeseriesEntry := range containerTimeSeries {
			series := aci.MetricTimeSeries{
				MetadataValues: []aci.MetricMetadataValue{{Name: aci.ValueDescriptor{Value: "containername"}, Value: containerName}},
				Data:           timeseriesEntry,
			}
			result = append(result, series)
		}
		return result
	}

	newMetricsTimeSeriesForSinglePod := func(podTimeSeries []aci.TimeSeriesEntry) []aci.MetricTimeSeries {
		return []aci.MetricTimeSeries{
			{
				MetadataValues: []aci.MetricMetadataValue{},
				Data:           podTimeSeries,
			},
		}
	}

	result := &aci.ContainerGroupMetricsResult{Value: make([]aci.MetricValue, 0)}
	for _, metricsType := range options.Types {
		switch metricsType {
		case aci.MetricTypeCPUUsage:
			result.Value = append(result.Value, aci.MetricValue{
				Desc:       aci.MetricDescriptor{Value: aci.MetricTypeCPUUsage},
				Timeseries: newMetricTimeseriesForMultipleContainers(mockMetricsGetter.containersCPU),
			})
		case aci.MetricTypeMemoryUsage:
			result.Value = append(result.Value, aci.MetricValue{
				Desc:       aci.MetricDescriptor{Value: aci.MetricTypeMemoryUsage},
				Timeseries: newMetricTimeseriesForMultipleContainers(mockMetricsGetter.containersMemory),
			})
		case aci.MetricTyperNetworkBytesRecievedPerSecond:
			result.Value = append(result.Value, aci.MetricValue{
				Desc:       aci.MetricDescriptor{Value: aci.MetricTyperNetworkBytesRecievedPerSecond},
				Timeseries: newMetricsTimeSeriesForSinglePod(mockMetricsGetter.podRx),
			})
		case aci.MetricTyperNetworkBytesTransmittedPerSecond:
			result.Value = append(result.Value, aci.MetricValue{
				Desc:       aci.MetricDescriptor{Value: aci.MetricTyperNetworkBytesTransmittedPerSecond},
				Timeseries: newMetricsTimeSeriesForSinglePod(mockMetricsGetter.podTx),
			})
		}
	}
	return result, nil
	/* exemple of result
	result := &aci.ContainerGroupMetricsResult{
		Value: []aci.MetricValue{
			{
				Desc: aci.MetricDescriptor{Value: aci.MetricTypeCPUUsage},
				Timeseries: []aci.MetricTimeSeries{
					{
						MetadataValues: []aci.MetricMetadataValue{{Name: aci.ValueDescriptor{Value: "containername"}, Value: "containe-1"}},
						Data: []aci.TimeSeriesEntry{
							{
								Timestamp: time.Now(),
								Average:   100,
							},
						},
					},
				},
			},
		},
	}
	return result, nil
	*/
}

type MetricsTestCase struct {
	name                    string
	pod                     PodInfo
	containerInsightMetrics ContainerInsightMetrics
	expectedPodStats        stats.PodStats
}

type PodInfo struct {
	name              string
	namespace         string
	containers        []string
	creationTimestamp time.Time
}

type ContainerInsightMetrics struct {
	containersCPU    map[string]TimeSeries
	containersMemory map[string]TimeSeries
	podRx            TimeSeries
	podTx            TimeSeries
}

type TimeSeries struct {
	lastPointTimestamp time.Time
	dataPoints         []float64
}

func TestGetPodMetrics(t *testing.T) {
	newUInt64Pointer := func(value int) *uint64 {
		var u = uint64(value)
		return &u
	}
	testCases := []MetricsTestCase{
		{
			name: "single container",
			pod: PodInfo{
				name:       "pod1",
				namespace:  "ns",
				containers: []string{"container1"},
			},
			containerInsightMetrics: ContainerInsightMetrics{
				containersCPU: map[string]TimeSeries{
					"container1": {time.Now(), []float64{10}},
				},
				containersMemory: map[string]TimeSeries{
					"container1": {time.Now(), []float64{1000}},
				},
				podRx: TimeSeries{time.Now(), []float64{2000}},
				podTx: TimeSeries{time.Now(), []float64{3000}},
			},
			expectedPodStats: stats.PodStats{
				Containers: []stats.ContainerStats{
					{
						Name: "container1",
						CPU: &stats.CPUStats{
							UsageNanoCores:       newUInt64Pointer(10 * 1000000),
							UsageCoreNanoSeconds: newUInt64Pointer(10 * 1000000 * 60),
						},
						Memory: &stats.MemoryStats{
							UsageBytes:      newUInt64Pointer(2000),
							WorkingSetBytes: newUInt64Pointer(3000),
						},
					},
				},
				CPU: &stats.CPUStats{
					UsageNanoCores:       newUInt64Pointer(10 * 1000000),
					UsageCoreNanoSeconds: newUInt64Pointer(10 * 1000000 * 60),
				},
				Memory: &stats.MemoryStats{
					UsageBytes:      newUInt64Pointer(2000),
					WorkingSetBytes: newUInt64Pointer(3000),
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			mockMetricsGetter := &MockContainerGroupMetricsGetter{
				containersCPU:    toTimeSeriesEntryMap(test.containerInsightMetrics.containersCPU),
				containersMemory: toTimeSeriesEntryMap(test.containerInsightMetrics.containersMemory),
				podRx:            toTimeSeriesEntry(test.containerInsightMetrics.podRx),
				podTx:            toTimeSeriesEntry(test.containerInsightMetrics.podTx),
			}
			metricsProvider := NewContainerInsightsMetricsProvider(mockMetricsGetter, "rg")
			pod := fakePod(test.pod)
			actualyPodStatus, err := metricsProvider.GetPodMetrics(context.Background(), pod)
			assert.NilError(t, err)
			fmt.Printf("a: %+v\n", *actualyPodStatus.CPU.UsageNanoCores)
			fmt.Printf("e: %+v\n", *test.expectedPodStats.CPU.UsageNanoCores)
			assert.Equal(t, *actualyPodStatus.CPU.UsageNanoCores, *test.expectedPodStats.CPU.UsageNanoCores)
		})
	}
}

func toTimeSeriesEntryMap(testTimeSeriesMap map[string]TimeSeries) map[string][]aci.TimeSeriesEntry {
	result := make(map[string][]aci.TimeSeriesEntry, len(testTimeSeriesMap))
	for k, v := range testTimeSeriesMap {
		result[k] = toTimeSeriesEntry(v)
	}
	return result
}

func toTimeSeriesEntry(testTimeSeries TimeSeries) []aci.TimeSeriesEntry {
	size := len(testTimeSeries.dataPoints)
	result := make([]aci.TimeSeriesEntry, size)
	for i, dataPoint := range testTimeSeries.dataPoints {
		result[i].Average = dataPoint
		result[i].Timestamp = testTimeSeries.lastPointTimestamp.Add(time.Minute * time.Duration(size-i-1))
	}
	return result
}

func fakePod(podinfo PodInfo) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              podinfo.name,
			Namespace:         podinfo.namespace,
			UID:               types.UID(podinfo.name),
			CreationTimestamp: metav1.NewTime(podinfo.creationTimestamp),
		},
		Status: v1.PodStatus{
			Phase:             v1.PodRunning,
			ContainerStatuses: make([]v1.ContainerStatus, 0, len(podinfo.containers)),
		},
	}

	for _, containerName := range podinfo.containers {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, v1.ContainerStatus{
			Name: containerName,
		})
	}
	return pod
}
