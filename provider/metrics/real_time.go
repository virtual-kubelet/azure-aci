package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ### Begin: real time metrics data types. this is map to JSON from from real time API
type PodStats struct {
	// Timestamp in nanoseconds at which the information were collected. Must be > 0.
	Timestamp  uint64           `json:"timestamp,omitempty"`
	Containers []ContainerStats `json:"containers" patchStrategy:"merge" patchMergeKey:"name"`
	// Stats pertaining to CPU resources consumed by pod cgroup (which includes all containers' resource usage and pod overhead).
	CPU CPUStats `json:"cpu,omitempty"`
	// Stats pertaining to memory (RAM) resources consumed by pod cgroup (which includes all containers' resource usage and pod overhead).
	Memory MemoryStats `json:"memory,omitempty"`
	// Stats pertaining to network resources.
	// +optional
	Network NetworkStats `json:"network,omitempty"`
}

type ContainerStats struct {
	// Reference to the measured container.
	Name string `json:"name"`
	// Stats pertaining to CPU resources.
	// +optional
	CPU CPUStats `json:"cpu"`
	// Stats pertaining to memory (RAM) resources.
	// +optional
	Memory MemoryStats `json:"memory"`
}

type CPUStats struct {
	// Cumulative CPU usage (sum across all cores) since object creation.
	UsageCoreNanoSeconds uint64 `json:"usageCoreNanoSeconds"`
}

// MemoryStats contains data about memory usage.
type MemoryStats struct {
	// Total memory in use. This includes all memory regardless of when it was accessed.
	UsageBytes uint64 `json:"usageBytes"`
	// The amount of working set memory. This includes recently accessed memory,
	// dirty memory, and kernel memory. WorkingSetBytes is <= UsageBytes
	WorkingSetBytes uint64 `json:"workingSetBytes"`
	// The amount of anonymous and swap cache memory (includes transparent
	// hugepages).
	RSSBytes uint64 `json:"rssBytes"`
}

// NetworkStats contains data about network resources.
type NetworkStats struct {
	// Stats for the default interface, if found
	InterfaceStats `json:",inline"`
	Interfaces     []InterfaceStats `json:"interfaces"`
}

type InterfaceStats struct {
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
}

func NewRealTimeMetrics() *realTimeMetrics {
	return &realTimeMetrics{}
}

// real-time implementation of podStatsGetter interface
func (realTime *realTimeMetrics) getPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	realtimePodStats, err := getRealTimePodStats(ctx, pod)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching pod '%s' statsistics from Real Time Extension", pod.Name)
	}
	return extensionPodStatsToKubeletPodStats(pod, realtimePodStats), nil
}

func getRealTimePodStats(ctx context.Context, pod *v1.Pod) (*PodStats, error) {
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
	result := &PodStats{}
	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal Real Time Metrics Extension body")
	}
	filterOutContainerNotInPod(result, pod)
	return result, nil
}

func filterOutContainerNotInPod(podStats *PodStats, pod *v1.Pod) {
	extractContainerNameFromPod := func() map[string]string {
		r := make(map[string]string)
		for _, container := range pod.Spec.Containers {
			r[container.Name] = container.Name
		}
		return r
	}
	containerNameIndex := extractContainerNameFromPod()
	containersStats := podStats.Containers
	podStats.Containers = make([]ContainerStats, 0)
	for _, c := range containersStats {
		if _, found := containerNameIndex[c.Name]; found {
			podStats.Containers = append(podStats.Containers, c)
		}
	}
}

func extensionPodStatsToKubeletPodStats(pod *v1.Pod, extensionPodStats *PodStats) *stats.PodStats {
	statsTime := metav1.NewTime(time.Unix(0, int64(extensionPodStats.Timestamp)))
	result := stats.PodStats{
		PodRef: stats.PodReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		},
		StartTime: pod.CreationTimestamp,
		CPU: &stats.CPUStats{
			Time:                 statsTime,
			UsageNanoCores:       &extensionPodStats.CPU.UsageCoreNanoSeconds,
			UsageCoreNanoSeconds: &extensionPodStats.CPU.UsageCoreNanoSeconds,
		},
		Memory: &stats.MemoryStats{
			Time:            statsTime,
			UsageBytes:      &extensionPodStats.Memory.UsageBytes,
			RSSBytes:        &extensionPodStats.Memory.RSSBytes,
			WorkingSetBytes: &extensionPodStats.Memory.WorkingSetBytes,
		},
		Network: &stats.NetworkStats{
			Time: statsTime,
			InterfaceStats: stats.InterfaceStats{
				Name:     extensionPodStats.Network.Name,
				RxBytes:  &extensionPodStats.Network.RxBytes,
				RxErrors: &extensionPodStats.Network.RxErrors,
				TxBytes:  &extensionPodStats.Network.TxBytes,
				TxErrors: &extensionPodStats.Network.TxErrors,
			},
		},
	}
	for _, extensionContainer := range extensionPodStats.Containers {
		result.Containers = make([]stats.ContainerStats, 0)
		result.Containers = append(result.Containers, stats.ContainerStats{
			Name:      extensionContainer.Name,
			StartTime: pod.CreationTimestamp,
			CPU: &stats.CPUStats{
				Time:                 statsTime,
				UsageNanoCores:       &extensionContainer.CPU.UsageCoreNanoSeconds,
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

	for _, extensionNetworkInterface := range extensionPodStats.Network.Interfaces {
		result.Network.Interfaces = make([]stats.InterfaceStats, 0)
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
