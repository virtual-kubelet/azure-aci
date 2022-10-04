package metrics

import (
	"context"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/thoas/go-funk"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// container insights implementation of podStatsGetter interface
type insightsGetter struct {
	metricsGetter        client.MetricsGetter
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

func NewContainerInsightsMetricsProvider(metricsGetter client.MetricsGetter, resourceGroup string) *insightsGetter {
	return &insightsGetter{
		metricsGetter:        metricsGetter,
		resourceGroup:        resourceGroup,
		cumulativeUsageCache: cache.New(time.Minute*30, time.Minute*30),
	}
}

func (g *insightsGetter) GetPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	logger := log.G(ctx).WithFields(log.Fields{
		"UID":       string(pod.UID),
		"Name":      pod.Name,
		"Namespace": pod.Namespace,
	})
	logger.Debug("Acquired semaphore")
	end := time.Now()
	start := end.Add(-5 * time.Minute)
	cgName := containerGroupName(pod.Namespace, pod.Name)

	metrics, err := queryContainerInsightsMetrics(logger, ctx, g.metricsGetter, g.resourceGroup, cgName, start, end)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching metrics from Container Insights for container group %s", cgName)
	}

	var podStats stats.PodStats = collectMetrics(logger, pod, metrics)
	g.updateCumulativeValues(logger, pod, metrics, &podStats)
	return &podStats, nil
}

func collectMetrics(logger log.Logger, pod *v1.Pod, metrics *containerInsightsMetricsWrapper) stats.PodStats {
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

		if cpuSeries, found := metrics.getCPU(containerName); found && len(cpuSeries) > 0 {
			data := cpuSeries[len(cpuSeries)-1]
			nanoCores := uint64(data.Average * 1000000)
			cs.CPU.UsageNanoCores = &nanoCores
			cs.CPU.Time = metav1.NewTime(data.Timestamp)

			podCPUCore := *stat.CPU.UsageNanoCores
			podCPUCore += nanoCores
			stat.CPU.UsageNanoCores = &podCPUCore
			stat.CPU.Time = metav1.NewTime(data.Timestamp)
		}
		if memorySeries, found := metrics.getMemory(containerName); found && len(memorySeries) > 0 {
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

func (g *insightsGetter) updateCumulativeValues(logger log.Logger, pod *v1.Pod, metrics *containerInsightsMetricsWrapper, stat *stats.PodStats) {
	cacheKey := string(pod.UID)
	c, found := g.cumulativeUsageCache.Get(cacheKey)
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
	defer g.cumulativeUsageCache.Set(cacheKey, cachedCumulativeValue, cache.DefaultExpiration)

	logger.Debugf("container_insights.updateCumulativeValues, cachedCumulativeValue=%v", cachedCumulativeValue)
	if rxSeries := metrics.getNetworkRx(); len(rxSeries) > 0 {
		data := rxSeries[len(rxSeries)-1]
		stat.Network.Time = metav1.NewTime(data.Timestamp)
		updateCumulativeUsage(logger, rxSeries, &cachedCumulativeValue.networkRx, stat.Network.RxBytes, 1)
	}

	if txSeries := metrics.getNetworkTx(); len(txSeries) > 0 {
		data := txSeries[len(txSeries)-1]
		stat.Network.Time = metav1.NewTime(data.Timestamp)
		updateCumulativeUsage(logger, txSeries, &cachedCumulativeValue.networkTx, stat.Network.TxBytes, 1)
	}

	for _, containerStats := range stat.Containers {
		if cpuSeries, found := metrics.getCPU(containerStats.Name); found && len(cpuSeries) > 0 {
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

func updateCumulativeUsage(logger log.Logger, series []client.TimeSeriesEntry, cumulative *cumulativeUsage, statsValue *uint64, multiple int) {
	// reset the cumulative value if there is gap between the metrics series and cached
	if series[0].Timestamp.Sub(cumulative.lastUpdateTime) > time.Minute {
		logger.Infof("container_insights.updateCumulativeUsage, there are some time gap between cached cumulative values and new metrics series, discard the cache")
		cumulative.value = 0
		cumulative.lastUpdateTime = time.Time{}
	}
	// make sure no time gap between the metrics series and cached cumulative
	for _, r := range series {
		if r.Timestamp.After(cumulative.lastUpdateTime) {
			cumulative.value += uint64(r.Average) * 60 * uint64(multiple)
			cumulative.lastUpdateTime = r.Timestamp
		}
	}
	*statsValue = cumulative.value
}

// encapsulate the logic of:
// 1. interaction with Container Insights
// 2. cache and index the result from Container Insights
// 3. lookup the metrics by metrics type and container name
type containerInsightsMetricsWrapper struct {
	networkRxMetricSeries []client.TimeSeriesEntry
	networkTxMetricSeries []client.TimeSeriesEntry
	cpuMetricsSeries      map[string][]client.TimeSeriesEntry
	memoryMetricsSeries   map[string][]client.TimeSeriesEntry
}

func queryContainerInsightsMetrics(logger log.Logger, ctx context.Context, metricsGetter client.MetricsGetter, resourceGroup, containerGroup string, start, end time.Time) (*containerInsightsMetricsWrapper, error) {
	wrapper := &containerInsightsMetricsWrapper{
		cpuMetricsSeries:    make(map[string][]client.TimeSeriesEntry),
		memoryMetricsSeries: make(map[string][]client.TimeSeriesEntry),
	}
	systemStats, err := metricsGetter.GetContainerGroupMetrics(ctx, resourceGroup, containerGroup, client.MetricsRequestOptions{
		Dimension:    "containerName eq '*'",
		Start:        start,
		End:          end,
		Aggregations: []client.AggregationType{client.AggregationTypeAverage},
		Types:        []client.MetricType{client.MetricTypeCPUUsage, client.MetricTypeMemoryUsage},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching cpu/mem stats for container group %s", containerGroup)
	}

	netStats, err := metricsGetter.GetContainerGroupMetrics(ctx, resourceGroup, containerGroup, client.MetricsRequestOptions{
		Start:        start,
		End:          end,
		Aggregations: []client.AggregationType{client.AggregationTypeAverage},
		Types:        []client.MetricType{client.MetricTyperNetworkBytesRecievedPerSecond, client.MetricTyperNetworkBytesTransmittedPerSecond},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching network stats for container group %s", containerGroup)
	}

	logger.Debug("container_insights.queryContainerInsightsMetrics, systemStats: %v", systemStats)
	logger.Debug("container_insights.queryContainerInsightsMetrics, netStats: %v", netStats)

	for _, metrics := range systemStats.Value {
		if metrics.Desc.Value == client.MetricTypeCPUUsage {
			for _, timeSeries := range metrics.Timeseries {
				containerNameMeta := funk.Find(timeSeries.MetadataValues, func(meta client.MetricMetadataValue) bool {
					return meta.Name.Value == "containername"
				})
				if containerNameMeta != nil {
					containerName := containerNameMeta.(client.MetricMetadataValue).Value
					wrapper.cpuMetricsSeries[containerName] = timeSeries.Data
				}
			}
		}
		if metrics.Desc.Value == client.MetricTypeMemoryUsage {
			for _, timeSeries := range metrics.Timeseries {
				containerNameMeta := funk.Find(timeSeries.MetadataValues, func(meta client.MetricMetadataValue) bool {
					return meta.Name.Value == "containername"
				})
				if containerNameMeta != nil {
					containerName := containerNameMeta.(client.MetricMetadataValue).Value
					wrapper.memoryMetricsSeries[containerName] = timeSeries.Data
				}
			}
		}
	}

	for _, metrics := range netStats.Value {
		if metrics.Desc.Value == client.MetricTyperNetworkBytesRecievedPerSecond && len(metrics.Timeseries) > 0 {
			wrapper.networkRxMetricSeries = metrics.Timeseries[0].Data
		}
		if metrics.Desc.Value == client.MetricTyperNetworkBytesTransmittedPerSecond && len(metrics.Timeseries) > 0 {
			wrapper.networkTxMetricSeries = metrics.Timeseries[0].Data
		}
	}
	return wrapper, nil
}

func (wrapper *containerInsightsMetricsWrapper) getCPU(containerName string) ([]client.TimeSeriesEntry, bool) {
	val, found := wrapper.cpuMetricsSeries[containerName]
	return val, found
}

func (wrapper *containerInsightsMetricsWrapper) getMemory(containerName string) ([]client.TimeSeriesEntry, bool) {
	val, found := wrapper.memoryMetricsSeries[containerName]
	return val, found
}

func (wrapper *containerInsightsMetricsWrapper) getNetworkTx() []client.TimeSeriesEntry {
	return wrapper.networkTxMetricSeries
}

func (wrapper *containerInsightsMetricsWrapper) getNetworkRx() []client.TimeSeriesEntry {
	return wrapper.networkRxMetricSeries
}
