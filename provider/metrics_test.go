package provider

import (
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2019-06-01/insights"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	stats "k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1"
)

func TestCollectMetrics(t *testing.T) {
	cases := []metricTestCase{
		{
			desc: "no containers",
		}, // this is just for sort of fuzzing things, make sure there's no panics
		{
			desc:      "zeroed stats",
			stats:     [][2]float64{{0, 0}},
			rx:        0,
			tx:        0,
			collected: date.Time{Time: time.Now()},
		},
		{
			desc:      "normal",
			stats:     [][2]float64{{400.0, 1000.0}},
			rx:        100.0,
			tx:        5000.0,
			collected: date.Time{Time: time.Now()},
		},
		{
			desc:      "multiple containers",
			stats:     [][2]float64{{100.0, 250.0}, {400.0, 1000.0}, {103.0, 3992.0}},
			rx:        100.0,
			tx:        439833.0,
			collected: date.Time{Time: time.Now()},
		},
	}

	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			pod := fakePod(t, len(test.stats), time.Now())
			expected := podStatFromTestCase(t, pod, test)

			system, net := fakeACIMetrics(pod, test)
			actual := collectMetrics(pod, system, net)

			if len(actual.Containers) != len(expected.Containers) {
				t.Fatalf("got unexpected results\nexpected:\n%+v\nactual:\n%+v", expected, actual)
			}

			for _, actualContainer := range actual.Containers {
				found := false
				for _, expectedContainer := range expected.Containers {
					if expectedContainer.Name == actualContainer.Name {
						assert.YAMLEq(t, toYAML(t, expectedContainer), toYAML(t, actualContainer))
						found = true
						break
					}
				}

				if !found {
					t.Fatalf("Unexpected container:\n%+v", actualContainer)
				}
			}

			expected.Containers = nil
			actual.Containers = nil

			assert.YAMLEq(t, toYAML(t, expected), toYAML(t, actual))
		})
	}
}

type metricTestCase struct {
	desc      string
	stats     [][2]float64
	rx, tx    float64
	collected date.Time
}

func fakeACIMetrics(pod *v1.Pod, testCase metricTestCase) (insights.Response, insights.Response) {
	newMetricValue := func(mt aci.MetricType) insights.Metric {
		return insights.Metric{
			Name: &insights.LocalizableString{
				Value: to.StringPtr(string(mt)),
			},
		}
	}

	newNetMetric := func(collected date.Time, value float64) insights.TimeSeriesElement {
		return insights.TimeSeriesElement{
			Data: &[]insights.MetricValue{
				{TimeStamp: &collected, Average: &value},
			},
		}
	}

	newSystemMetric := func(c v1.ContainerStatus, collected date.Time, value float64) insights.TimeSeriesElement {
		return insights.TimeSeriesElement{
			Data: &[]insights.MetricValue{
				{TimeStamp: &collected, Average: &value},
			},
			Metadatavalues: &[]insights.MetadataValue{
				{Name: &insights.LocalizableString{Value: to.StringPtr("containerName")}, Value: &c.Name},
			},
		}
	}

	// create fake aci metrics for the container group and test data
	var cpuTimeseries, memTimeseries []insights.TimeSeriesElement
	cpuV := newMetricValue(aci.MetricTypeCPUUsage)
	memV := newMetricValue(aci.MetricTypeMemoryUsage)
	for i, c := range pod.Status.ContainerStatuses {
		cpuTimeseries = append(cpuTimeseries, newSystemMetric(c, testCase.collected, testCase.stats[i][0]))
		memTimeseries = append(memTimeseries, newSystemMetric(c, testCase.collected, testCase.stats[i][1]))
	}
	cpuV.Timeseries = &cpuTimeseries
	memV.Timeseries = &memTimeseries
	system := insights.Response{
		Value: &[]insights.Metric{cpuV, memV},
	}

	var rxVTimeseries, txVTimeseries []insights.TimeSeriesElement
	rxV := newMetricValue(aci.MetricTyperNetworkBytesRecievedPerSecond)
	txV := newMetricValue(aci.MetricTyperNetworkBytesTransmittedPerSecond)
	rxVTimeseries = append(rxVTimeseries, newNetMetric(testCase.collected, testCase.rx))
	txVTimeseries = append(txVTimeseries, newNetMetric(testCase.collected, testCase.tx))
	rxV.Timeseries = &rxVTimeseries
	txV.Timeseries = &txVTimeseries
	net := insights.Response{
		Value: &[]insights.Metric{rxV, txV},
	}
	return system, net
}

func fakePod(t *testing.T, size int, created time.Time) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              path.Base(t.Name()),
			Namespace:         path.Dir(t.Name()),
			UID:               types.UID(t.Name()),
			CreationTimestamp: metav1.NewTime(created),
		},
		Status: v1.PodStatus{
			Phase:             v1.PodRunning,
			ContainerStatuses: make([]v1.ContainerStatus, 0, size),
		},
	}

	for i := 0; i < size; i++ {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, v1.ContainerStatus{
			Name: "c" + strconv.Itoa(i),
		})
	}
	return pod
}

func podStatFromTestCase(t *testing.T, pod *v1.Pod, test metricTestCase) stats.PodStats {
	rx := uint64(test.rx)
	tx := uint64(test.tx)
	expected := stats.PodStats{
		StartTime: pod.CreationTimestamp,
		PodRef: stats.PodReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		},
		Network: &stats.NetworkStats{
			Time: metav1.NewTime(test.collected.ToTime()),
			InterfaceStats: stats.InterfaceStats{
				Name:    "eth0",
				RxBytes: &rx,
				TxBytes: &tx,
			},
		},
	}

	var (
		nodeCPU uint64
		nodeMem uint64
	)
	for i := range test.stats {
		cpu := uint64(test.stats[i][0] * 1000000)
		cpuNanoSeconds := cpu * 60
		mem := uint64(test.stats[i][1])

		expected.Containers = append(expected.Containers, stats.ContainerStats{
			StartTime: pod.CreationTimestamp,
			Name:      pod.Status.ContainerStatuses[i].Name,
			CPU:       &stats.CPUStats{Time: metav1.NewTime(test.collected.ToTime()), UsageNanoCores: &cpu, UsageCoreNanoSeconds: &cpuNanoSeconds},
			Memory:    &stats.MemoryStats{Time: metav1.NewTime(test.collected.ToTime()), UsageBytes: &mem, WorkingSetBytes: &mem},
		})
		nodeCPU += cpu
		nodeMem += mem
	}
	if len(expected.Containers) > 0 {
		nanoCPUSeconds := nodeCPU * 60
		expected.CPU = &stats.CPUStats{UsageNanoCores: &nodeCPU, UsageCoreNanoSeconds: &nanoCPUSeconds, Time: metav1.NewTime(test.collected.ToTime())}
		expected.Memory = &stats.MemoryStats{UsageBytes: &nodeMem, WorkingSetBytes: &nodeMem, Time: metav1.NewTime(test.collected.ToTime())}
	}
	return expected
}

func toYAML(t testing.TB, obj interface{}) string {
	raw, err := yaml.Marshal(obj)
	require.NoError(t, err, "converting to yaml")
	return string(raw)
}
