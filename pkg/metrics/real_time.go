package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ### Begin: real time metrics data types. this is map to JSON from from real time API
type realtimeMetricsExtensionPodStats struct {
	// Timestamp in nanoseconds at which the information were collected. Must be > 0.
	Timestamp  uint64           `json:"timestamp,omitempty"`
	Containers []containerStats `json:"containers" patchStrategy:"merge" patchMergeKey:"name"`
	// Stats pertaining to CPU resources consumed by pod cgroup (which includes all containers' resource usage and pod overhead).
	CPU cpuStats `json:"cpu,omitempty"`
	// Stats pertaining to memory (RAM) resources consumed by pod cgroup (which includes all containers' resource usage and pod overhead).
	Memory memoryStats `json:"memory,omitempty"`
	// Stats pertaining to network resources.
	// +optional
	Network networkStats `json:"network,omitempty"`
}

type containerStats struct {
	// Reference to the measured container.
	Name string `json:"name"`
	// Stats pertaining to CPU resources.
	// +optional
	CPU cpuStats `json:"cpu"`
	// Stats pertaining to memory (RAM) resources.
	// +optional
	Memory memoryStats `json:"memory"`
}

type cpuStats struct {
	// Cumulative CPU usage (sum across all cores) since object creation.
	UsageCoreNanoSeconds uint64 `json:"usageCoreNanoSeconds"`
}

// memoryStats contains data about memory usage.
type memoryStats struct {
	// Total memory in use. This includes all memory regardless of when it was accessed.
	UsageBytes uint64 `json:"usageBytes"`
	// The amount of working set memory. This includes recently accessed memory,
	// dirty memory, and kernel memory. WorkingSetBytes is <= UsageBytes
	WorkingSetBytes uint64 `json:"workingSetBytes"`
	// The amount of anonymous and swap cache memory (includes transparent
	// hugepages).
	RSSBytes uint64 `json:"rssBytes"`
}

// networkStats contains data about network resources.
type networkStats struct {
	// Stats for the default interface, if found
	interfaceStats `json:",inline"`
	Interfaces     []interfaceStats `json:"interfaces"`
}

type interfaceStats struct {
	// The name of the interface
	Name string `json:"name"`
	// Cumulative count of bytes received.
	// +optional
	RxBytes uint64 `json:"rxBytes"`
	// Cumulative count of receive errors encountered.
	// +optional
	RxErrors uint64 `json:"rxErrors"`
	// Cumulative count of bytes transmitted.
	// +optional
	TxBytes uint64 `json:"txBytes"`
	// Cumulative count of transmit errors encountered.
	// +optional
	TxErrors uint64 `json:"txErrors"`
}

type realTimeMetrics struct {
	// this cache is for calculating UsageNanoCores. Real Time Metrics Extension
	// only return cumulative UsageCoreNanoSeconds. However UsageNanoCores is a
	// average CPU usage per seconds in a time windows.
	// So we need to cache the last value of UsageCoreNanoSeconds and calcuate the average during
	// the last time windows
	cpuStatsCache *cache.Cache
}

func NewRealTimeMetrics() *realTimeMetrics {
	return &realTimeMetrics{
		cpuStatsCache: cache.New(time.Minute*10, time.Minute*10),
	}
}

// GetPodStats the implementation of podStatsGetter interface base on ACI's Real-Time Metrics Extension
func (realTime *realTimeMetrics) GetPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	realtimeExtensionPodStats, err := getRealTimeExtensionPodStats(ctx, pod)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching pod '%s' statsistics from Real Time Extension", pod.Name)
	}
	result := extensionPodStatsToKubeletPodStats(pod, realtimeExtensionPodStats)
	realTime.populateUsageNanocores(pod, realtimeExtensionPodStats, result)
	return result, nil
}

func getRealTimeExtensionPodStats(ctx context.Context, pod *v1.Pod) (*realtimeMetricsExtensionPodStats, error) {
	if pod.Status.Phase != v1.PodRunning {
		return nil, errors.Errorf("invalid parameter in getRealTimePodStats, only Running pod allow to query realtime statistics")
	}
	url := fmt.Sprintf("http://%s:18899/v1/stats", pod.Status.PodIP)
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to request to Real Time Metrics Extension endpoint %s", url)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read data from Real Time Metrics Extension's response")
	}
	result := &realtimeMetricsExtensionPodStats{}
	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal Real Time Metrics Extension body")
	}
	filterOutContainerNotInPod(result, pod)
	return result, nil
}

func extensionPodStatsToKubeletPodStats(pod *v1.Pod, realtimePodStats *realtimeMetricsExtensionPodStats) *stats.PodStats {
	statsTime := metav1.NewTime(time.Unix(0, int64(realtimePodStats.Timestamp)))
	result := stats.PodStats{
		PodRef: stats.PodReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		},
		StartTime: pod.CreationTimestamp,
		CPU: &stats.CPUStats{
			Time:                 statsTime,
			UsageNanoCores:       newUInt64Pointer(0),
			UsageCoreNanoSeconds: &realtimePodStats.CPU.UsageCoreNanoSeconds,
		},
		Memory: &stats.MemoryStats{
			Time:            statsTime,
			UsageBytes:      &realtimePodStats.Memory.UsageBytes,
			RSSBytes:        &realtimePodStats.Memory.RSSBytes,
			WorkingSetBytes: &realtimePodStats.Memory.WorkingSetBytes,
		},
		Network: &stats.NetworkStats{
			Time: statsTime,
			InterfaceStats: stats.InterfaceStats{
				Name:     realtimePodStats.Network.Name,
				RxBytes:  &realtimePodStats.Network.RxBytes,
				RxErrors: &realtimePodStats.Network.RxErrors,
				TxBytes:  &realtimePodStats.Network.TxBytes,
				TxErrors: &realtimePodStats.Network.TxErrors,
			},
		},
	}
	result.Containers = make([]stats.ContainerStats, 0)
	for _, extensionContainer := range realtimePodStats.Containers {
		result.Containers = append(result.Containers, stats.ContainerStats{
			Name:      extensionContainer.Name,
			StartTime: pod.CreationTimestamp,
			CPU: &stats.CPUStats{
				Time:                 statsTime,
				UsageNanoCores:       newUInt64Pointer(0),
				UsageCoreNanoSeconds: &extensionContainer.CPU.UsageCoreNanoSeconds,
			},
			Memory: &stats.MemoryStats{
				Time:            statsTime,
				UsageBytes:      &extensionContainer.Memory.UsageBytes,
				RSSBytes:        &extensionContainer.Memory.RSSBytes,
				WorkingSetBytes: &extensionContainer.Memory.WorkingSetBytes,
			},
		})
	}

	result.Network.Interfaces = make([]stats.InterfaceStats, 0)
	for _, extensionNetworkInterface := range realtimePodStats.Network.Interfaces {
		extensionNetworkInterface := extensionNetworkInterface
		result.Network.Interfaces = append(result.Network.Interfaces, stats.InterfaceStats{
			Name:     extensionNetworkInterface.Name,
			RxBytes:  &extensionNetworkInterface.RxBytes,
			RxErrors: &extensionNetworkInterface.RxErrors,
			TxBytes:  &extensionNetworkInterface.TxBytes,
			TxErrors: &extensionNetworkInterface.TxErrors,
		})
	}
	return &result
}

func (realTime *realTimeMetrics) populateUsageNanocores(pod *v1.Pod, realTimePodStats *realtimeMetricsExtensionPodStats, podStats *stats.PodStats) {
	defer realTime.cpuStatsCache.Set(string(pod.UID), realTimePodStats, cache.DefaultExpiration)
	lastRealtimePodStatus, found := realTime.cpuStatsCache.Get(string(pod.UID))
	if !found {
		return
	}
	podStats.CPU.UsageNanoCores = calculateUsageNanoCores(nil, lastRealtimePodStatus.(*realtimeMetricsExtensionPodStats), realTimePodStats)
	for _, containerStat := range podStats.Containers {
		containerStat.CPU.UsageNanoCores = calculateUsageNanoCores(&containerStat.Name, lastRealtimePodStatus.(*realtimeMetricsExtensionPodStats), realTimePodStats)
	}
}

func calculateUsageNanoCores(containerName *string, lastPodStatus *realtimeMetricsExtensionPodStats, newPodStatus *realtimeMetricsExtensionPodStats) *uint64 {
	if lastPodStatus == nil {
		return newUInt64Pointer(0)
	}
	if newPodStatus == nil {
		return newUInt64Pointer(0)
	}
	timeWindowsNanoSeconds := newPodStatus.Timestamp - lastPodStatus.Timestamp
	if timeWindowsNanoSeconds <= 0 {
		return newUInt64Pointer(0)
	}
	var timeWindowsSeconds uint64 = timeWindowsNanoSeconds / 1000000000
	if containerName == nil {
		// calculate for Pod
		v := (newPodStatus.CPU.UsageCoreNanoSeconds - lastPodStatus.CPU.UsageCoreNanoSeconds) / timeWindowsSeconds
		return &v
	} else {
		// calcuate for specified container
		var oldContainerUsageCoreNanoSeconds *uint64 = nil
		for _, container := range lastPodStatus.Containers {
			if container.Name == *containerName {
				oldContainerUsageCoreNanoSeconds = &container.CPU.UsageCoreNanoSeconds
			}
		}
		if oldContainerUsageCoreNanoSeconds == nil {
			return newUInt64Pointer(0)
		}
		var newContainerUsageCoreNanoSeconds *uint64 = nil
		for _, container := range newPodStatus.Containers {
			if container.Name == *containerName {
				newContainerUsageCoreNanoSeconds = &container.CPU.UsageCoreNanoSeconds
			}
		}
		if newContainerUsageCoreNanoSeconds == nil {
			return newUInt64Pointer(0)
		}
		v := (*newContainerUsageCoreNanoSeconds - *oldContainerUsageCoreNanoSeconds) / timeWindowsSeconds
		return &v
	}
}

// there are some containers in Real Time Metrics Extension but not in Pod
// for example some infra sidecar container. We need to filter out those containers
func filterOutContainerNotInPod(podStats *realtimeMetricsExtensionPodStats, pod *v1.Pod) {
	extractContainerNameFromPod := func() map[string]string {
		r := make(map[string]string)
		for _, container := range pod.Spec.Containers {
			r[container.Name] = container.Name
		}
		return r
	}
	containerNameIndex := extractContainerNameFromPod()
	containersStats := podStats.Containers
	podStats.Containers = make([]containerStats, 0)
	for _, c := range containersStats {
		if _, found := containerNameIndex[c.Name]; found {
			podStats.Containers = append(podStats.Containers, c)
		}
	}
}
