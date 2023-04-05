package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			assert.EqualValues(t, tc.expectedUsage, nanoCoreUsage, tc.desc)
		})
	}

}
