package metrics

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/thoas/go-funk"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// container insights implementation of podStatsGetter interface
type containerInsightsPodStatsGetter struct {
	metricsGetter        ContainerGroupMetricsGetter
	resourceGroup        string
	cumulativeUsageCache *cache.Cache
}

// some of values in stats.PodStats are require cumulative data. But Container Insights be only able to
// provide the average value in 1 minute period. So we have to calculate the cumulative value from the average.
// we need to remember the last cumulative value and add new value of next time windows.

type podCumulativeUsage struct {
	podUID                         string
	containersUsageCoreNanoSeconds map[string]cumulativeUsage
	networkRx                      cumulativeUsage
	networkTx                      cumulativeUsage
}

type cumulativeUsage struct {
	lastUpdateTime time.Time
	value          uint64
}

func NewContainerInsightsMetricsProvider(metricsGetter ContainerGroupMetricsGetter, resourceGroup string) *containerInsightsPodStatsGetter {
	return &containerInsightsPodStatsGetter{
		metricsGetter:        metricsGetter,
		resourceGroup:        resourceGroup,
		cumulativeUsageCache: cache.New(time.Minute*30, time.Minute*30),
	}
}

func (containerInsights *containerInsightsPodStatsGetter) getPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	logger := log.G(ctx).WithFields(log.Fields{
		"UID":       string(pod.UID),
		"Name":      pod.Name,
		"Namespace": pod.Namespace,
	})
	logger.Debug("Acquired semaphore")
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	logger.Debugf("getPodStats, start=%s, end=%s", start, end)
	cgName := containerGroupName(pod.Namespace, pod.Name)
	// cpu/mem and net stats are split because net stats do not support container level detail
	systemStats, err := containerInsights.metricsGetter.GetContainerGroupMetrics(ctx, containerInsights.resourceGroup, cgName, aci.MetricsRequest{
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

	netStats, err := containerInsights.metricsGetter.GetContainerGroupMetrics(ctx, containerInsights.resourceGroup, cgName, aci.MetricsRequest{
		Start:        start,
		End:          end,
		Aggregations: []aci.AggregationType{aci.AggregationTypeAverage},
		Types:        []aci.MetricType{aci.MetricTyperNetworkBytesRecievedPerSecond, aci.MetricTyperNetworkBytesTransmittedPerSecond},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching network stats for container group %s", cgName)
	}
	logger.Debug("getPodStats, system: %+v", systemStats)
	logger.Debug("getPodStats, net: %+v", netStats)
	logger.Debug("Got network stats")

	var podStats stats.PodStats = collectMetrics(logger, pod, systemStats, netStats)
	containerInsights.updateCumulativeValues(logger, pod, systemStats, netStats, &podStats)
	return &podStats, nil
}

func collectMetrics(logger log.Logger, pod *v1.Pod, system, net *aci.ContainerGroupMetricsResult) stats.PodStats {
	stat := stats.PodStats{
		StartTime: pod.CreationTimestamp,
		PodRef: stats.PodReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		},
		CPU: &stats.CPUStats{
			UsageNanoCores:       newUInt64Pointer(0),
			UsageCoreNanoSeconds: newUInt64Pointer(0),
			Time:                 metav1.NewTime(time.Now()),
		},
		Memory: &stats.MemoryStats{
			UsageBytes:      newUInt64Pointer(0),
			WorkingSetBytes: newUInt64Pointer(0),
			Time:            metav1.NewTime(time.Now()),
		},
		Network: &stats.NetworkStats{
			Time: metav1.NewTime(time.Now()),
			InterfaceStats: stats.InterfaceStats{
				Name:     "eth0",
				RxBytes:  newUInt64Pointer(0),
				RxErrors: newUInt64Pointer(0),
				TxBytes:  newUInt64Pointer(0),
				TxErrors: newUInt64Pointer(0),
			},
		},
		Containers: make([]stats.ContainerStats, 0),
	}

	for _, c := range pod.Status.ContainerStatuses {
		containerName := c.Name
		logger.Debugf("star to populate stats for container '%s'", containerName)
		cs := stats.ContainerStats{
			Name:      containerName,
			StartTime: stat.StartTime,
			CPU: &stats.CPUStats{
				Time:                 metav1.NewTime(time.Now()),
				UsageNanoCores:       newUInt64Pointer(0),
				UsageCoreNanoSeconds: newUInt64Pointer(0),
			},
			Memory: &stats.MemoryStats{
				Time:            metav1.NewTime(time.Now()),
				RSSBytes:        newUInt64Pointer(0),
				WorkingSetBytes: newUInt64Pointer(0),
			},
		}
		if cpuSeries, found := findTimeSeries(system, aci.MetricTypeCPUUsage, &containerName); found && len(cpuSeries) > 0 {
			data := cpuSeries[len(cpuSeries)-1]
			nanoCores := uint64(data.Average * 1000000)
			cs.CPU.UsageNanoCores = &nanoCores
			cs.CPU.Time = metav1.NewTime(data.Timestamp)

			podCPUCore := *stat.CPU.UsageNanoCores
			podCPUCore += nanoCores
			stat.CPU.UsageNanoCores = &podCPUCore
			stat.CPU.Time = metav1.NewTime(data.Timestamp)
		}
		if memorySeries, found := findTimeSeries(system, aci.MetricTypeMemoryUsage, &containerName); found && len(memorySeries) > 0 {
			data := memorySeries[len(memorySeries)-1]
			bytes := uint64(data.Average)
			cs.Memory.UsageBytes = &bytes
			cs.Memory.WorkingSetBytes = &bytes
			cs.Memory.Time = metav1.NewTime(data.Timestamp)

			podMem := *stat.Memory.UsageBytes
			podMem += bytes
			stat.Memory.UsageBytes = &podMem
			stat.Memory.WorkingSetBytes = &podMem
			stat.Memory.Time = metav1.NewTime(data.Timestamp)
		}
		stat.Containers = append(stat.Containers, cs)
	}

	return stat
}

func findTimeSeries(metricsResult *aci.ContainerGroupMetricsResult, metricsType aci.MetricType, containerName *string) ([]aci.TimeSeriesEntry, bool) {
	for _, metrics := range metricsResult.Value {
		if metrics.Desc.Value == metricsType {
			if containerName == nil {
				if len(metrics.Timeseries) > 0 {
					return metrics.Timeseries[0].Data, true
				}
			} else {
				for _, timeSeries := range metrics.Timeseries {
					foundContainer := funk.Contains(timeSeries.MetadataValues, func(meta aci.MetricMetadataValue) bool {
						return meta.Name.Value == "containername" && meta.Value == *containerName
					})
					if foundContainer {
						return timeSeries.Data, true
					}
				}
			}
		}
	}
	return nil, false
}

func (containerInsights *containerInsightsPodStatsGetter) updateCumulativeValues(logger log.Logger, pod *v1.Pod, system, net *aci.ContainerGroupMetricsResult, stat *stats.PodStats) {
	cacheKey := string(pod.UID)
	c, found := containerInsights.cumulativeUsageCache.Get(cacheKey)
	var cachedCumulativeValue *podCumulativeUsage
	if !found {
		logger.Infof("Didn't find cumulative values in cache for pod: pod name '%s', UID '%s'", pod.Name, cacheKey)
		cachedCumulativeValue = &podCumulativeUsage{
			podUID: cacheKey,
			networkRx: cumulativeUsage{
				lastUpdateTime: time.Time{},
				value:          0,
			},
			networkTx: cumulativeUsage{
				lastUpdateTime: time.Time{},
				value:          0,
			},
			containersUsageCoreNanoSeconds: make(map[string]cumulativeUsage),
		}
	} else {
		cachedCumulativeValue = c.(*podCumulativeUsage)
	}
	defer containerInsights.cumulativeUsageCache.Set(cacheKey, cachedCumulativeValue, cache.DefaultExpiration)

	if rxSeries, found := findTimeSeries(net, aci.MetricTyperNetworkBytesRecievedPerSecond, nil); found && len(rxSeries) > 0 {
		data := rxSeries[len(rxSeries)-1]
		stat.Network.Time = metav1.NewTime(data.Timestamp)
		updateCumulativeUsage(logger, rxSeries, &cachedCumulativeValue.networkRx, stat.Network.RxBytes, 1)
	}

	if txSeries, found := findTimeSeries(net, aci.MetricTyperNetworkBytesTransmittedPerSecond, nil); found && len(txSeries) > 0 {
		data := txSeries[len(txSeries)-1]
		stat.Network.Time = metav1.NewTime(data.Timestamp)
		updateCumulativeUsage(logger, txSeries, &cachedCumulativeValue.networkTx, stat.Network.TxBytes, 1)
	}

	for _, containerStats := range stat.Containers {
		if cpuSeries, found := findTimeSeries(system, aci.MetricTypeCPUUsage, &containerStats.Name); found && len(cpuSeries) > 0 {
			if containerStats.CPU.UsageCoreNanoSeconds == nil {
				containerStats.CPU.UsageCoreNanoSeconds = newUInt64Pointer(0)
			}
			v, found := cachedCumulativeValue.containersUsageCoreNanoSeconds[containerStats.Name]
			if !found {
				v = cumulativeUsage{
					value: *newUInt64Pointer(0),
				}
			}
			updateCumulativeUsage(logger, cpuSeries, &v, containerStats.CPU.UsageCoreNanoSeconds, 1000000)
			cachedCumulativeValue.containersUsageCoreNanoSeconds[containerStats.Name] = v
		}
	}

	// Pod's CPU value is the summary of all conainers
	podCPU := uint64(0)
	for _, c := range stat.Containers {
		podCPU += *c.CPU.UsageCoreNanoSeconds
	}
	stat.CPU.UsageCoreNanoSeconds = &podCPU
}

func updateCumulativeUsage(logger log.Logger, series []aci.TimeSeriesEntry, cumulative *cumulativeUsage, statsValue *uint64, multiple int) {
	// reset the cumulative value if there is gap between the metrics series and cached
	if series[0].Timestamp.Sub(cumulative.lastUpdateTime) > time.Minute {
		logger.Infof("there are some time gap between cached cumulative values and new metrics series, discard the cache")
		cumulative.value = 0
		cumulative.lastUpdateTime = time.Time{}
	}
	// make sure no time gap between the metrics series and cached cumulative
	for _, r := range series {
		if r.Timestamp.After(cumulative.lastUpdateTime) {
			cumulative.value += uint64(r.Average) * 60 * uint64(multiple)
			cumulative.lastUpdateTime = r.Timestamp
			*statsValue = cumulative.value
		}
	}
}

func truncateToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}
