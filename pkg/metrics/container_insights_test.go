package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/virtual-kubelet/azure-aci/pkg/client"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type MyMockContainerGroupMetricsGetter struct {
	containersCPU    map[string][]client.TimeSeriesEntry
	containersMemory map[string][]client.TimeSeriesEntry
	podRx            []client.TimeSeriesEntry
	podTx            []client.TimeSeriesEntry
}

func (mockMetricsGetter *MyMockContainerGroupMetricsGetter) GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options client.MetricsRequestOptions) (*client.ContainerGroupMetricsResult, error) {
	newMetricTimeseriesForMultipleContainers := func(containerTimeSeries map[string][]client.TimeSeriesEntry) []client.MetricTimeSeries {
		var result []client.MetricTimeSeries = make([]client.MetricTimeSeries, len(containerTimeSeries))
		for containerName, timeseriesEntry := range containerTimeSeries {
			series := client.MetricTimeSeries{
				MetadataValues: []client.MetricMetadataValue{{Name: client.ValueDescriptor{Value: "containername"}, Value: containerName}},
				Data:           timeseriesEntry,
			}
			result = append(result, series)
		}
		return result
	}

	newMetricsTimeSeriesForSinglePod := func(podTimeSeries []client.TimeSeriesEntry) []client.MetricTimeSeries {
		return []client.MetricTimeSeries{
			{
				MetadataValues: []client.MetricMetadataValue{},
				Data:           podTimeSeries,
			},
		}
	}

	result := &client.ContainerGroupMetricsResult{Value: make([]client.MetricValue, 0)}
	for _, metricsType := range options.Types {
		switch metricsType {
		case client.MetricTypeCPUUsage:
			result.Value = append(result.Value, client.MetricValue{
				Desc:       client.MetricDescriptor{Value: client.MetricTypeCPUUsage},
				Timeseries: newMetricTimeseriesForMultipleContainers(mockMetricsGetter.containersCPU),
			})
		case client.MetricTypeMemoryUsage:
			result.Value = append(result.Value, client.MetricValue{
				Desc:       client.MetricDescriptor{Value: client.MetricTypeMemoryUsage},
				Timeseries: newMetricTimeseriesForMultipleContainers(mockMetricsGetter.containersMemory),
			})
		case client.MetricTyperNetworkBytesRecievedPerSecond:
			result.Value = append(result.Value, client.MetricValue{
				Desc:       client.MetricDescriptor{Value: client.MetricTyperNetworkBytesRecievedPerSecond},
				Timeseries: newMetricsTimeSeriesForSinglePod(mockMetricsGetter.podRx),
			})
		case client.MetricTyperNetworkBytesTransmittedPerSecond:
			result.Value = append(result.Value, client.MetricValue{
				Desc:       client.MetricDescriptor{Value: client.MetricTyperNetworkBytesTransmittedPerSecond},
				Timeseries: newMetricsTimeSeriesForSinglePod(mockMetricsGetter.podTx),
			})
		}
	}
	return result, nil
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

func TestContainerInsightsMetrics_NonCumulativeValues(t *testing.T) {
	newUInt64Pointer := func(value int) *uint64 {
		var u = uint64(value)
		return &u
	}
	findContainerStats := func(containerName string, statsSlices []stats.ContainerStats) *stats.ContainerStats {
		for _, s := range statsSlices {
			if s.Name == containerName {
				return &s
			}
		}
		return nil
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
							UsageBytes:      newUInt64Pointer(1000),
							WorkingSetBytes: newUInt64Pointer(1000),
						},
					},
				},
				CPU: &stats.CPUStats{
					UsageNanoCores:       newUInt64Pointer(10 * 1000000),
					UsageCoreNanoSeconds: newUInt64Pointer(10 * 1000000 * 60),
				},
				Memory: &stats.MemoryStats{
					UsageBytes:      newUInt64Pointer(1000),
					WorkingSetBytes: newUInt64Pointer(1000),
				},
				Network: &stats.NetworkStats{
					InterfaceStats: stats.InterfaceStats{
						Name:    "eth0",
						RxBytes: newUInt64Pointer(2000 * 60),
						TxBytes: newUInt64Pointer(3000 * 60),
					},
				},
			},
		},
		{
			name: "multiple container",
			pod: PodInfo{
				name:       "pod1",
				namespace:  "ns",
				containers: []string{"container1", "container2"},
			},
			containerInsightMetrics: ContainerInsightMetrics{
				containersCPU: map[string]TimeSeries{
					"container1": {time.Now(), []float64{12, 11, 10}},
					"container2": {time.Now(), []float64{22, 21, 20}},
				},
				containersMemory: map[string]TimeSeries{
					"container1": {time.Now(), []float64{1002, 1001, 1000}},
					"container2": {time.Now(), []float64{2002, 2001, 2000}},
				},
				podRx: TimeSeries{time.Now(), []float64{3000}},
				podTx: TimeSeries{time.Now(), []float64{4000}},
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
							UsageBytes:      newUInt64Pointer(1000),
							WorkingSetBytes: newUInt64Pointer(1000),
						},
					},
					{
						Name: "container2",
						CPU: &stats.CPUStats{
							UsageNanoCores:       newUInt64Pointer(20 * 1000000),
							UsageCoreNanoSeconds: newUInt64Pointer(20 * 1000000 * 60),
						},
						Memory: &stats.MemoryStats{
							UsageBytes:      newUInt64Pointer(2000),
							WorkingSetBytes: newUInt64Pointer(2000),
						},
					},
				},
				CPU: &stats.CPUStats{
					UsageNanoCores:       newUInt64Pointer(10*1000000 + 20*1000000),
					UsageCoreNanoSeconds: newUInt64Pointer(10*1000000*60 + 10*1000000*60),
				},
				Memory: &stats.MemoryStats{
					UsageBytes:      newUInt64Pointer(3000),
					WorkingSetBytes: newUInt64Pointer(3000),
				},
				Network: &stats.NetworkStats{
					InterfaceStats: stats.InterfaceStats{
						Name:    "eth0",
						RxBytes: newUInt64Pointer(3000 * 60),
						TxBytes: newUInt64Pointer(4000 * 60),
					},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			mockMetricsGetter := &MyMockContainerGroupMetricsGetter{
				containersCPU:    toTimeSeriesEntryMap(test.containerInsightMetrics.containersCPU),
				containersMemory: toTimeSeriesEntryMap(test.containerInsightMetrics.containersMemory),
				podRx:            toTimeSeriesEntry(test.containerInsightMetrics.podRx),
				podTx:            toTimeSeriesEntry(test.containerInsightMetrics.podTx),
			}
			metricsProvider := NewContainerInsightsMetricsProvider(mockMetricsGetter, "rg")
			pod := fakePods(test.pod)
			actualyPodStatus, err := metricsProvider.GetPodStats(context.Background(), pod)
			assert.NilError(t, err)
			assert.Equal(t, *actualyPodStatus.CPU.UsageNanoCores, *test.expectedPodStats.CPU.UsageNanoCores)
			assert.Equal(t, *actualyPodStatus.Memory.UsageBytes, *test.expectedPodStats.Memory.UsageBytes)
			assert.Equal(t, *actualyPodStatus.Memory.WorkingSetBytes, *test.expectedPodStats.Memory.WorkingSetBytes)
			assert.Equal(t, *actualyPodStatus.Network.TxBytes, *test.expectedPodStats.Network.TxBytes)
			assert.Equal(t, *actualyPodStatus.Network.RxBytes, *test.expectedPodStats.Network.RxBytes)
			assert.Equal(t, len(actualyPodStatus.Containers), len(test.expectedPodStats.Containers))
			for _, expectedContainerStat := range test.expectedPodStats.Containers {
				actualyContainerStat := findContainerStats(expectedContainerStat.Name, actualyPodStatus.Containers)
				assert.Assert(t, actualyContainerStat != nil)
				assert.Equal(t, *actualyContainerStat.CPU.UsageNanoCores, *expectedContainerStat.CPU.UsageNanoCores)
				assert.Equal(t, *actualyContainerStat.Memory.UsageBytes, *expectedContainerStat.Memory.UsageBytes)
				assert.Equal(t, *actualyContainerStat.Memory.WorkingSetBytes, *expectedContainerStat.Memory.WorkingSetBytes)
			}
		})
	}
}

func TestContainerInsightsMetrics_CumulativeValues(t *testing.T) {
	t.Run("accumulate value if time series conjunctive", func(t *testing.T) {
		pod := fakePods(PodInfo{
			name:              "pod1",
			namespace:         "ns",
			containers:        []string{"container1"},
			creationTimestamp: time.Now(),
		})

		// first metrics
		time1 := time.Now().Truncate(time.Minute).Add(-1 * time.Minute)
		metrics1 := ContainerInsightMetrics{
			containersCPU: map[string]TimeSeries{
				"container1": {time1, []float64{10}},
			},
			containersMemory: map[string]TimeSeries{
				"container1": {time1, []float64{1000}},
			},
			podRx: TimeSeries{time1, []float64{2000}},
			podTx: TimeSeries{time1, []float64{3000}},
		}
		mockMetricsGetter1 := &MyMockContainerGroupMetricsGetter{
			containersCPU:    toTimeSeriesEntryMap(metrics1.containersCPU),
			containersMemory: toTimeSeriesEntryMap(metrics1.containersMemory),
			podRx:            toTimeSeriesEntry(metrics1.podRx),
			podTx:            toTimeSeriesEntry(metrics1.podTx),
		}

		// second metrics
		time2 := time.Now().Truncate(time.Minute)
		metrics2 := ContainerInsightMetrics{
			containersCPU: map[string]TimeSeries{
				"container1": {time2, []float64{10}},
			},
			containersMemory: map[string]TimeSeries{
				"container1": {time2, []float64{1000}},
			},
			podRx: TimeSeries{time2, []float64{2000}},
			podTx: TimeSeries{time2, []float64{3000}},
		}
		mockMetricsGetter2 := &MyMockContainerGroupMetricsGetter{
			containersCPU:    toTimeSeriesEntryMap(metrics2.containersCPU),
			containersMemory: toTimeSeriesEntryMap(metrics2.containersMemory),
			podRx:            toTimeSeriesEntry(metrics2.podRx),
			podTx:            toTimeSeriesEntry(metrics2.podTx),
		}

		var actualyPodStatus *stats.PodStats
		var err error
		metricsProvider := NewContainerInsightsMetricsProvider(mockMetricsGetter1, "rg")
		actualyPodStatus, err = metricsProvider.GetPodStats(context.Background(), pod)
		assert.Equal(t, *actualyPodStatus.Network.RxBytes, uint64(2000*60))
		assert.Equal(t, *actualyPodStatus.Network.TxBytes, uint64(3000*60))
		assert.Equal(t, *actualyPodStatus.CPU.UsageCoreNanoSeconds, uint64(10*1000000*60))
		assert.NilError(t, err)
		metricsProvider.metricsGetter = mockMetricsGetter2
		actualyPodStatus, err = metricsProvider.GetPodStats(context.Background(), pod)
		assert.NilError(t, err)
		assert.Equal(t, *actualyPodStatus.Network.RxBytes, uint64(2000*60*2))
		assert.Equal(t, *actualyPodStatus.Network.TxBytes, uint64(3000*60*2))
		assert.Equal(t, *actualyPodStatus.CPU.UsageCoreNanoSeconds, uint64(10*1000000*60)*2)
	})
	t.Run("accumulate value if time series not conjunctive", func(t *testing.T) {
		pod := fakePods(PodInfo{
			name:              "pod1",
			namespace:         "ns",
			containers:        []string{"container1"},
			creationTimestamp: time.Now(),
		})

		// first metrics
		// time is two minutes ago
		time1 := time.Now().Truncate(time.Minute).Add(-2 * time.Minute)
		metrics1 := ContainerInsightMetrics{
			containersCPU: map[string]TimeSeries{
				"container1": {time1, []float64{10}},
			},
			containersMemory: map[string]TimeSeries{
				"container1": {time1, []float64{1000}},
			},
			podRx: TimeSeries{time1, []float64{2000}},
			podTx: TimeSeries{time1, []float64{3000}},
		}
		mockMetricsGetter1 := &MyMockContainerGroupMetricsGetter{
			containersCPU:    toTimeSeriesEntryMap(metrics1.containersCPU),
			containersMemory: toTimeSeriesEntryMap(metrics1.containersMemory),
			podRx:            toTimeSeriesEntry(metrics1.podRx),
			podTx:            toTimeSeriesEntry(metrics1.podTx),
		}

		// second metrics
		// time is now, NOT conjective with time 1
		time2 := time.Now().Truncate(time.Minute)
		metrics2 := ContainerInsightMetrics{
			containersCPU: map[string]TimeSeries{
				"container1": {time2, []float64{10}},
			},
			containersMemory: map[string]TimeSeries{
				"container1": {time2, []float64{1000}},
			},
			podRx: TimeSeries{time2, []float64{2000}},
			podTx: TimeSeries{time2, []float64{3000}},
		}
		mockMetricsGetter2 := &MyMockContainerGroupMetricsGetter{
			containersCPU:    toTimeSeriesEntryMap(metrics2.containersCPU),
			containersMemory: toTimeSeriesEntryMap(metrics2.containersMemory),
			podRx:            toTimeSeriesEntry(metrics2.podRx),
			podTx:            toTimeSeriesEntry(metrics2.podTx),
		}

		var actualyPodStatus *stats.PodStats
		var err error
		metricsProvider := NewContainerInsightsMetricsProvider(mockMetricsGetter1, "rg")
		actualyPodStatus, err = metricsProvider.GetPodStats(context.Background(), pod)
		assert.Equal(t, *actualyPodStatus.Network.RxBytes, uint64(2000*60))
		assert.Equal(t, *actualyPodStatus.Network.TxBytes, uint64(3000*60))
		assert.Equal(t, *actualyPodStatus.CPU.UsageCoreNanoSeconds, uint64(10*1000000*60))
		assert.NilError(t, err)
		metricsProvider.metricsGetter = mockMetricsGetter2
		actualyPodStatus, err = metricsProvider.GetPodStats(context.Background(), pod)
		assert.NilError(t, err)
		assert.Equal(t, *actualyPodStatus.Network.RxBytes, uint64(2000*60))
		assert.Equal(t, *actualyPodStatus.Network.TxBytes, uint64(3000*60))
		assert.Equal(t, *actualyPodStatus.CPU.UsageCoreNanoSeconds, uint64(10*1000000*60))
	})
}

func toTimeSeriesEntryMap(testTimeSeriesMap map[string]TimeSeries) map[string][]client.TimeSeriesEntry {
	result := make(map[string][]client.TimeSeriesEntry, len(testTimeSeriesMap))
	for k, v := range testTimeSeriesMap {
		result[k] = toTimeSeriesEntry(v)
	}
	return result
}

func toTimeSeriesEntry(testTimeSeries TimeSeries) []client.TimeSeriesEntry {
	size := len(testTimeSeries.dataPoints)
	result := make([]client.TimeSeriesEntry, size)
	for i, dataPoint := range testTimeSeries.dataPoints {
		result[i].Average = dataPoint
		result[i].Timestamp = testTimeSeries.lastPointTimestamp.Truncate(time.Minute).Add(time.Minute * time.Duration(size-i-1))
	}
	return result
}

func fakePods(podinfo PodInfo) *v1.Pod {
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
