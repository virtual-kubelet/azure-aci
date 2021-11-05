package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ContainerInsightsMetricsProvider struct {
	metricsGetter ContainerGroupMetricsGetter
	resourceGroup string
}

func NewContainerInsightsMetricsProvider(metricsGetter ContainerGroupMetricsGetter, resourceGroup string) *ContainerInsightsMetricsProvider {
	return &ContainerInsightsMetricsProvider{
		metricsGetter: metricsGetter,
		resourceGroup: resourceGroup,
	}
}

func (metricsProvider *ContainerInsightsMetricsProvider) GetPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	end := time.Now()
	start := end.Add(-1 * time.Minute)
	logger := log.G(ctx).WithFields(log.Fields{
		"UID":       string(pod.UID),
		"Name":      pod.Name,
		"Namespace": pod.Namespace,
	})
	logger.Debug("Acquired semaphore")

	cgName := containerGroupName(pod.Namespace, pod.Name)
	// cpu/mem and net stats are split because net stats do not support container level detail
	systemStats, err := metricsProvider.metricsGetter.GetContainerGroupMetrics(ctx, metricsProvider.resourceGroup, cgName, aci.MetricsRequest{
		Dimension:    "containerName eq '*'",
		Start:        start,
		End:          end,
		Aggregations: []aci.AggregationType{aci.AggregationTypeAverage},
		Types:        []aci.MetricType{aci.MetricTypeCPUUsage, aci.MetricTypeMemoryUsage},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching cpu/mem stats for container group %s", cgName)
	}
	logger.Debug("Got system stats")

	netStats, err := metricsProvider.metricsGetter.GetContainerGroupMetrics(ctx, metricsProvider.resourceGroup, cgName, aci.MetricsRequest{
		Start:        start,
		End:          end,
		Aggregations: []aci.AggregationType{aci.AggregationTypeAverage},
		Types:        []aci.MetricType{aci.MetricTyperNetworkBytesRecievedPerSecond, aci.MetricTyperNetworkBytesTransmittedPerSecond},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching network stats for container group %s", cgName)
	}
	logger.Debug("Got network stats")

	var podStats stats.PodStats = collectMetrics(pod, systemStats, netStats)
	return &podStats, nil
}

func collectMetrics(pod *v1.Pod, system, net *aci.ContainerGroupMetricsResult) stats.PodStats {
	var stat stats.PodStats
	containerStats := make(map[string]*stats.ContainerStats, len(pod.Status.ContainerStatuses))
	stat.StartTime = pod.CreationTimestamp

	for _, m := range system.Value {
		// cpu/mem stats are per container, so each entry in the time series is for a container, not the container group.
		for _, entry := range m.Timeseries {
			if len(entry.Data) == 0 {
				continue
			}

			var cs *stats.ContainerStats
			for _, v := range entry.MetadataValues {
				if strings.ToLower(v.Name.Value) != "containername" {
					continue
				}
				if cs = containerStats[v.Value]; cs == nil {
					cs = &stats.ContainerStats{Name: v.Value, StartTime: stat.StartTime}
					containerStats[v.Value] = cs
				}
			}
			if cs == nil {
				continue
			}

			if stat.Containers == nil {
				stat.Containers = make([]stats.ContainerStats, 0, len(containerStats))
			}

			data := entry.Data[len(entry.Data)-1] // get only the last entry
			switch m.Desc.Value {
			case aci.MetricTypeCPUUsage:
				if cs.CPU == nil {
					cs.CPU = &stats.CPUStats{}
				}

				// average is the average number of millicores over a 1 minute interval (which is the interval we are pulling the stats for)
				nanoCores := uint64(data.Average * 1000000)
				usageNanoSeconds := nanoCores * 60
				cs.CPU.Time = metav1.NewTime(data.Timestamp)
				cs.CPU.UsageCoreNanoSeconds = &usageNanoSeconds
				cs.CPU.UsageNanoCores = &nanoCores

				if stat.CPU == nil {
					var zero uint64
					stat.CPU = &stats.CPUStats{UsageNanoCores: &zero, UsageCoreNanoSeconds: &zero, Time: metav1.NewTime(data.Timestamp)}
				}
				podCPUSec := *stat.CPU.UsageCoreNanoSeconds
				podCPUSec += usageNanoSeconds
				stat.CPU.UsageCoreNanoSeconds = &podCPUSec

				podCPUCore := *stat.CPU.UsageNanoCores
				podCPUCore += nanoCores
				stat.CPU.UsageNanoCores = &podCPUCore
			case aci.MetricTypeMemoryUsage:
				if cs.Memory == nil {
					cs.Memory = &stats.MemoryStats{}
				}
				cs.Memory.Time = metav1.NewTime(data.Timestamp)
				bytes := uint64(data.Average)
				cs.Memory.UsageBytes = &bytes
				cs.Memory.WorkingSetBytes = &bytes

				if stat.Memory == nil {
					var zero uint64
					stat.Memory = &stats.MemoryStats{UsageBytes: &zero, WorkingSetBytes: &zero, Time: metav1.NewTime(data.Timestamp)}
				}
				podMem := *stat.Memory.UsageBytes
				podMem += bytes
				stat.Memory.UsageBytes = &podMem
				stat.Memory.WorkingSetBytes = &podMem
			}
		}
	}

	for _, m := range net.Value {
		if stat.Network == nil {
			stat.Network = &stats.NetworkStats{}
		}
		// network stats are for the whole container group, so there should only be one entry here.
		if len(m.Timeseries) == 0 {
			continue
		}
		entry := m.Timeseries[0]
		if len(entry.Data) == 0 {
			continue
		}
		data := entry.Data[len(entry.Data)-1] // get only the last entry

		bytes := uint64(data.Average)
		switch m.Desc.Value {
		case aci.MetricTyperNetworkBytesRecievedPerSecond:
			stat.Network.RxBytes = &bytes
		case aci.MetricTyperNetworkBytesTransmittedPerSecond:
			stat.Network.TxBytes = &bytes
		}
		stat.Network.Time = metav1.NewTime(data.Timestamp)
		stat.Network.InterfaceStats.Name = "eth0"
	}

	for _, cs := range containerStats {
		stat.Containers = append(stat.Containers, *cs)
	}

	stat.PodRef = stats.PodReference{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		UID:       string(pod.UID),
	}

	return stat
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}
