/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package metrics

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCalculateUsageNanoCores(t *testing.T) {

	fake_container1 := "fake-container-name1"
	fake_container2 := "fake-container-name2"

	newPodStatus_containers := []containerStats{
		{
			Name: fake_container1,
			CPU: cpuStats{
				UsageCoreNanoSeconds: 2345678900,
			},
		},
		{
			Name: fake_container2,
			CPU: cpuStats{
				UsageCoreNanoSeconds: 1234567800,
			},
		},
	}

	lastPodStatus_containers := []containerStats{
		{
			Name: fake_container1,
			CPU: cpuStats{
				UsageCoreNanoSeconds: 1234567800,
			},
		},
		{
			Name: fake_container2,
			CPU: cpuStats{
				UsageCoreNanoSeconds: 2345678900,
			},
		},
	}

	testCases := []struct {
		desc          string
		containerName *string
		lastPodStatus *realtimeMetricsExtensionPodStats
		newPodStatus  *realtimeMetricsExtensionPodStats
		expectedUsage *uint64
	}{
		{
			desc:          "NewPodStatus timestamp is earlier than LastPodStatus",
			containerName: nil,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 2345000000,
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 1234000000,
			},
			expectedUsage: newUInt64Pointer(0),
		},
		{
			desc:          "New and Last Pod Status Timestamp difference value is very low",
			containerName: nil,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 1234,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567,
				},
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 2345,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678,
				},
			},
			expectedUsage: newUInt64Pointer(0),
		},
		{
			desc:          "Container Name is nil and lastPodStatus.CPU.UsageCoreNanoSeconds is greater than newPodStatus.CPU.UsageCoreNanoSeconds",
			containerName: nil,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 1234500000,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678000,
				},
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 2345600000,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567000,
				},
			},
			expectedUsage: newUInt64Pointer(0),
		},
		{
			desc:          "Container Name is nil and newPodStatus.CPU.UsageCoreNanoSeconds is greater than lastPodStatus.CPU.UsageCoreNanoSeconds",
			containerName: nil,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 1234500000,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567000,
				},
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp: 2345600000,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678000,
				},
			},
			//tc.newPodStatus.CPU.UsageCoreNanoSeconds-tc.lastPodStatus.CPU.UsageCoreNanoSeconds/((tc.newPodStatus.TimeStamp-tc.lastPodStatus.TimeStamp)/1000000000)
			expectedUsage: newUInt64Pointer(1111111000 / (1111100000 / 1000000000)),
		},
		{
			desc:          "Container Name is not nil and newPodStatus.CPU.UsageCoreNanoSeconds is greater than lastPodStatus.CPU.UsageCoreNanoSeconds",
			containerName: &fake_container1,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp:  1234560000,
				Containers: lastPodStatus_containers,
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp:  2345670000,
				Containers: newPodStatus_containers,
			},
			//tc.newPodStatus.containers[i=0].CPU.UsageCoreNanoSeconds-tc.lastPodStatus.containers[i=0].CPU.UsageCoreNanoSeconds/((tc.newPodStatus.TimeStamp-tc.lastPodStatus.TimeStamp)/1000000000)
			expectedUsage: newUInt64Pointer(1111111100 / (1111110000 / 1000000000)),
		},
		{
			desc:          "Container Name is not nil and lastPodStatus.CPU.UsageCoreNanoSeconds is greater than newPodStatus.CPU.UsageCoreNanoSeconds",
			containerName: &fake_container2,
			lastPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp:  1234500000,
				Containers: lastPodStatus_containers,
			},
			newPodStatus: &realtimeMetricsExtensionPodStats{
				Timestamp:  2345600000,
				Containers: lastPodStatus_containers,
			},
			expectedUsage: newUInt64Pointer(0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nanoCoreUsage := calculateUsageNanoCores(tc.containerName, tc.lastPodStatus, tc.newPodStatus)
			assert.DeepEqual(t, tc.expectedUsage, nanoCoreUsage)
		})
	}
}

func TestFilterOutContainerNotInPod(t *testing.T) {

	fake_container1 := "fake-container-name1"
	fake_container2 := "fake-container-name2"
	fake_container3 := "fake-infra-sidecar-container"
	fake_container4 := "fake-container-name4"
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: fake_container1,
				},
				{
					Name: fake_container2,
				},
				{
					Name: fake_container4,
				},
			},
		},
	}

	podStats := &realtimeMetricsExtensionPodStats{
		Timestamp: 1234560000,
		Containers: []containerStats{
			{
				Name: fake_container1,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678900,
				},
			},
			{
				Name: fake_container2,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567800,
				},
			},
			{
				Name: fake_container3,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 9876543210,
				},
			},
			{
				Name: fake_container4,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1237894560,
				},
			},
		},
	}

	testDescription := "Successfully filters out containerStats for containers not in pod.spec.containers"
	t.Run(testDescription, func(t *testing.T) {
		filterOutContainerNotInPod(podStats, pod)
		assert.DeepEqual(t, 3, len(podStats.Containers))
		assert.DeepEqual(t, fake_container1, podStats.Containers[0].Name)
		assert.DeepEqual(t, fake_container2, podStats.Containers[1].Name)
		assert.DeepEqual(t, fake_container4, podStats.Containers[2].Name)
	})
}

func TestGetRealTimeExtensionPodStats(t *testing.T) {

	fake_container1 := "fake-container-name1"
	fake_container2 := "fake-container-name2"
	fake_container3 := "fake-infra-sidecar-container"
	fake_container4 := "fake-container-name4"
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: fake_container1,
				},
				{
					Name: fake_container3,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: fake_container1,
				},
				{
					Name: fake_container2,
				},
				{
					Name: fake_container4,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	podStats := &realtimeMetricsExtensionPodStats{
		Timestamp: 1234560000,
		Containers: []containerStats{
			{
				Name: fake_container1,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678900,
				},
			},
			{
				Name: fake_container2,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567800,
				},
			},
			{
				Name: fake_container3,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 9876543210,
				},
			},
			{
				Name: fake_container4,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1237894560,
				},
			},
		},
	}

	respBodyJsonBytes, err := json.Marshal(podStats)

	if err != nil {
		t.Fatal("Unable to Marshal JSON", err)

	}

	respBody := string(respBodyJsonBytes)
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	// create a new http.Client with the Transport field set to the mock round tripper
	client := &http.Client{
		Transport: &mockRoundTripper{
			response: mockResp,
			err:      nil,
		},
	}

	// replace the default client with the mocked one
	originalClient := http.DefaultClient
	http.DefaultClient = client
	defer func() { http.DefaultClient = originalClient }()

	testCases := []struct {
		desc          string
		pod           *corev1.Pod
		expectedError error
	}{
		{
			desc:          "Pod.Status.Phase is not equal to Running",
			pod:           pendingPod,
			expectedError: errors.Errorf("invalid parameter in getRealTimePodStats, only Running pod allow to query realtime statistics"),
		},
		{
			desc:          "Pod.Status.Phase is equal to Running",
			pod:           runningPod,
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			realTimeExtPodStats, err := getRealTimeExtensionPodStats(context.TODO(), tc.pod)
			if tc.expectedError == nil {
				assert.NilError(t, err)
				assert.DeepEqual(t, 3, len(realTimeExtPodStats.Containers))
				assert.DeepEqual(t, fake_container1, realTimeExtPodStats.Containers[0].Name)
				assert.DeepEqual(t, fake_container2, realTimeExtPodStats.Containers[1].Name)
				assert.DeepEqual(t, fake_container4, realTimeExtPodStats.Containers[2].Name)
			} else {
				assert.DeepEqual(t, tc.expectedError.Error(), err.Error())
			}

		})
	}
}

type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestExtensionPodStatsToKubeletsPodStats(t *testing.T) {

	fake_container1 := "fake-container-name1"
	fake_container2 := "fake-container-name2"
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              podName,
			Namespace:         podNamespace,
			UID:               "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
			CreationTimestamp: metav1.NewTime(time.Date(2023, 4, 12, 12, 0, 0, 0, time.UTC)),
		},
	}

	podStats := &realtimeMetricsExtensionPodStats{
		Timestamp: 1234567890,
		CPU: cpuStats{
			UsageCoreNanoSeconds: 2345678900,
		},
		Memory: memoryStats{
			UsageBytes:      104857600,
			WorkingSetBytes: 73400320,
			RSSBytes:        98765400,
		},
		Network: networkStats{
			interfaceStats: interfaceStats{
				Name:     "eth0",
				RxBytes:  1024,
				RxErrors: 0,
				TxBytes:  2048,
				TxErrors: 0,
			},
			Interfaces: []interfaceStats{
				{
					Name:     "eth1",
					RxBytes:  64,
					RxErrors: 0,
					TxBytes:  128,
					TxErrors: 0,
				},
				{
					Name:     "wifi",
					RxBytes:  2048,
					RxErrors: 0,
					TxBytes:  4096,
					TxErrors: 0,
				},
			},
		},
		Containers: []containerStats{
			{
				Name: fake_container1,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 2345678900,
				},
				Memory: memoryStats{
					UsageBytes:      12345600,
					WorkingSetBytes: 23456700,
					RSSBytes:        98765400,
				},
			},
			{
				Name: fake_container2,
				CPU: cpuStats{
					UsageCoreNanoSeconds: 1234567800,
				},
				Memory: memoryStats{
					UsageBytes:      23456700,
					WorkingSetBytes: 23456789,
					RSSBytes:        98765413,
				},
			},
		},
	}

	expectedKubeletPodStats := stats.PodStats{
		PodRef: stats.PodReference{
			Name:      podName,
			Namespace: podNamespace,
			UID:       "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
		},
		StartTime: metav1.NewTime(time.Date(2023, 4, 12, 12, 0, 0, 0, time.UTC)),
		Containers: []stats.ContainerStats{
			{
				Name:      fake_container1,
				StartTime: metav1.NewTime(time.Date(2023, 4, 12, 12, 0, 0, 0, time.UTC)),
				CPU: &stats.CPUStats{
					Time:                 metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
					UsageNanoCores:       newUInt64Pointer(0),
					UsageCoreNanoSeconds: newUInt64Pointer(2345678900),
				},
				Memory: &stats.MemoryStats{
					Time:            metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
					UsageBytes:      newUInt64Pointer(12345600),
					WorkingSetBytes: newUInt64Pointer(23456700),
					RSSBytes:        newUInt64Pointer(98765400),
				},
			},
			{
				Name:      fake_container2,
				StartTime: metav1.NewTime(time.Date(2023, 4, 12, 12, 0, 0, 0, time.UTC)),
				CPU: &stats.CPUStats{
					Time:                 metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
					UsageNanoCores:       newUInt64Pointer(0),
					UsageCoreNanoSeconds: newUInt64Pointer(1234567800),
				},
				Memory: &stats.MemoryStats{
					Time:            metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
					UsageBytes:      newUInt64Pointer(23456700),
					WorkingSetBytes: newUInt64Pointer(23456789),
					RSSBytes:        newUInt64Pointer(98765413),
				},
			},
		},
		CPU: &stats.CPUStats{
			Time:                 metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
			UsageNanoCores:       newUInt64Pointer(0),
			UsageCoreNanoSeconds: newUInt64Pointer(2345678900),
		},
		Memory: &stats.MemoryStats{
			Time:            metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
			UsageBytes:      newUInt64Pointer(104857600),
			WorkingSetBytes: newUInt64Pointer(73400320),
			RSSBytes:        newUInt64Pointer(98765400),
		},
		Network: &stats.NetworkStats{
			Time: metav1.NewTime(time.Unix(0, int64(podStats.Timestamp))),
			InterfaceStats: stats.InterfaceStats{
				Name:     "eth0",
				RxBytes:  newUInt64Pointer(1024),
				RxErrors: newUInt64Pointer(0),
				TxBytes:  newUInt64Pointer(2048),
				TxErrors: newUInt64Pointer(0),
			},
			Interfaces: []stats.InterfaceStats{
				{
					Name:     "eth1",
					RxBytes:  newUInt64Pointer(64),
					RxErrors: newUInt64Pointer(0),
					TxBytes:  newUInt64Pointer(128),
					TxErrors: newUInt64Pointer(0),
				},
				{
					Name:     "wifi",
					RxBytes:  newUInt64Pointer(2048),
					RxErrors: newUInt64Pointer(0),
					TxBytes:  newUInt64Pointer(4096),
					TxErrors: newUInt64Pointer(0),
				},
			},
		},
	}

	cases := []struct {
		description string
		assertions  func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats)
	}{
		{
			description: "Actual and Expected Pod Reference Metadata should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.PodRef.Name, kubeletPodStats.PodRef.Name)
				assert.DeepEqual(t, expectedKubeletPodStats.PodRef.Namespace, kubeletPodStats.PodRef.Namespace)
				assert.DeepEqual(t, expectedKubeletPodStats.PodRef.UID, kubeletPodStats.PodRef.UID)
			},
		},
		{
			description: "Actual and Expected Start Time should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.StartTime, kubeletPodStats.StartTime)
			},
		},
		{
			description: "Actual and Expected CPU Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.CPU.Time, kubeletPodStats.CPU.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.CPU.UsageCoreNanoSeconds, kubeletPodStats.CPU.UsageCoreNanoSeconds)
				assert.DeepEqual(t, expectedKubeletPodStats.CPU.UsageNanoCores, kubeletPodStats.CPU.UsageNanoCores)
			},
		},
		{
			description: "Actual and Expected Memory Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.Memory.Time, kubeletPodStats.Memory.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.Memory.UsageBytes, kubeletPodStats.Memory.UsageBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Memory.WorkingSetBytes, kubeletPodStats.Memory.WorkingSetBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Memory.RSSBytes, kubeletPodStats.Memory.RSSBytes)
			},
		},
		{
			description: "Actual and Expected Network Interface Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.Network.InterfaceStats.Name, kubeletPodStats.Network.InterfaceStats.Name)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.InterfaceStats.RxBytes, kubeletPodStats.Network.InterfaceStats.RxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.InterfaceStats.RxErrors, kubeletPodStats.Network.InterfaceStats.RxErrors)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.InterfaceStats.TxBytes, kubeletPodStats.Network.InterfaceStats.TxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.InterfaceStats.TxErrors, kubeletPodStats.Network.InterfaceStats.TxErrors)
			},
		},
		{
			description: "Actual and Expected Network Interfaces array Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[0].Name, kubeletPodStats.Network.Interfaces[0].Name)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[0].RxBytes, kubeletPodStats.Network.Interfaces[0].RxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[0].RxErrors, kubeletPodStats.Network.Interfaces[0].RxErrors)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[0].TxBytes, kubeletPodStats.Network.Interfaces[0].TxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[0].TxErrors, kubeletPodStats.Network.Interfaces[0].TxErrors)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[1].Name, kubeletPodStats.Network.Interfaces[1].Name)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[1].RxBytes, kubeletPodStats.Network.Interfaces[1].RxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[1].RxErrors, kubeletPodStats.Network.Interfaces[1].RxErrors)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[1].TxBytes, kubeletPodStats.Network.Interfaces[1].TxBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Network.Interfaces[1].TxErrors, kubeletPodStats.Network.Interfaces[1].TxErrors)
			},
		},
		{
			description: "Actual and Expected Containers array CPU Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].CPU.Time, kubeletPodStats.Containers[0].CPU.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].CPU.UsageCoreNanoSeconds, kubeletPodStats.Containers[0].CPU.UsageCoreNanoSeconds)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].CPU.UsageNanoCores, kubeletPodStats.Containers[0].CPU.UsageNanoCores)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].CPU.Time, kubeletPodStats.Containers[1].CPU.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].CPU.UsageCoreNanoSeconds, kubeletPodStats.Containers[1].CPU.UsageCoreNanoSeconds)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].CPU.UsageNanoCores, kubeletPodStats.Containers[1].CPU.UsageNanoCores)
			},
		},
		{
			description: "Actual and Expected Containers array Memory Stats should match",
			assertions: func(expectedKubeletPodStats *stats.PodStats, kubeletPodStats *stats.PodStats) {
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].Memory.Time, kubeletPodStats.Containers[0].Memory.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].Memory.UsageBytes, kubeletPodStats.Containers[0].Memory.UsageBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].Memory.WorkingSetBytes, kubeletPodStats.Containers[0].Memory.WorkingSetBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[0].Memory.RSSBytes, kubeletPodStats.Containers[0].Memory.RSSBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].Memory.Time, kubeletPodStats.Containers[1].Memory.Time)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].Memory.UsageBytes, kubeletPodStats.Containers[1].Memory.UsageBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].Memory.WorkingSetBytes, kubeletPodStats.Containers[1].Memory.WorkingSetBytes)
				assert.DeepEqual(t, expectedKubeletPodStats.Containers[1].Memory.RSSBytes, kubeletPodStats.Containers[1].Memory.RSSBytes)
			},
		},
	}

	kubeletPodStats := extensionPodStatsToKubeletPodStats(pod, podStats)

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			tc.assertions(&expectedKubeletPodStats, kubeletPodStats)
		})
	}
}
