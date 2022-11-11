/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"testing"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/date"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	cgCreationTime, _ = time.Parse(timeLayout, time.Now().String())
	cgName            = "testCG"
	containerName     = "testContainer"
	fakeIP            = "127.0.0.1"
	restartCount      = int32(0)
)

func TestContainerGroupToPodStatus(t *testing.T) {
	startTime := cgCreationTime.Add(time.Second * 3)
	finishTime := startTime.Add(time.Second * 3)

	cases := []struct {
		description           string
		containerGroup        *azaci.ContainerGroup
		expectedPodPhase      v1.PodPhase
		expectedPodConditions []v1.PodCondition
	}{
		{
			description: "Container is Running/Succeeded",
			containerGroup: getContainerGroup(cgName, "Succeeded", &[]azaci.Container{
				{
					Name: &containerName,
					ContainerProperties: &azaci.ContainerProperties{
						Image: &containerName,
						Ports: &[]azaci.ContainerPort{},
						InstanceView: &azaci.ContainerPropertiesInstanceView{
							CurrentState:  getContainerState("Succeeded", startTime, finishTime, 0),
							PreviousState: getContainerState("Running", cgCreationTime, startTime, 0),
							RestartCount:  &restartCount,
							Events:        &[]azaci.Event{},
						},
						LivenessProbe:  &azaci.ContainerProbe{},
						ReadinessProbe: &azaci.ContainerProbe{},
					},
				},
			}, "Succeeded"),
			expectedPodPhase:      getPodPhaseFromACIState("Succeeded"),
			expectedPodConditions: getPodConditions(metav1.NewTime(cgCreationTime), metav1.NewTime(finishTime), v1.ConditionTrue),
		},
		{
			description: "Container Failed",
			containerGroup: getContainerGroup(cgName, "Failed", &[]azaci.Container{
				{
					Name: &containerName,
					ContainerProperties: &azaci.ContainerProperties{
						Image: &containerName,
						Ports: &[]azaci.ContainerPort{},
						InstanceView: &azaci.ContainerPropertiesInstanceView{
							CurrentState:  getContainerState("Failed", startTime, finishTime, 0),
							PreviousState: getContainerState("Running", startTime, finishTime, 400),
							RestartCount:  &restartCount,
							Events:        &[]azaci.Event{},
						},
						LivenessProbe:  &azaci.ContainerProbe{},
						ReadinessProbe: &azaci.ContainerProbe{},
					},
				},
			}, "Succeeded"),
			expectedPodPhase:      getPodPhaseFromACIState("Failed"),
			expectedPodConditions: []v1.PodCondition{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			expectedStatus, err := getPodStatusFromContainerGroup(tc.containerGroup)
			assert.NilError(t, err, "no errors should be returned")
			assert.Equal(t, tc.expectedPodPhase, expectedStatus.Phase, "Pod phase is not as expected as current container group phase")
			assert.Equal(t, len(tc.expectedPodConditions), len(expectedStatus.Conditions), "Pod conditions are not as expected")
		})
	}
}

func getContainerGroup(cgName, cgState string, containers *[]azaci.Container, provisioningState string) *azaci.ContainerGroup {
	fakeIPAddress := azaci.IPAddress{
		IP: &fakeIP,
	}
	timeAsString := metav1.NewTime(cgCreationTime).String()

	return &azaci.ContainerGroup{
		Tags: map[string]*string{
			"CreationTimestamp": &timeAsString,
		},
		Name: &cgName,
		ID:   &cgName,
		ContainerGroupProperties: &azaci.ContainerGroupProperties{
			Containers: containers,
			InstanceView: &azaci.ContainerGroupPropertiesInstanceView{
				State: &cgState,
			},
			ProvisioningState: &provisioningState,
			IPAddress:         &fakeIPAddress,
		},
	}
}

func getContainerState(state string, startTime, finishTime time.Time, exitCode int32) *azaci.ContainerState {
	return &azaci.ContainerState{
		State: &state,
		StartTime: &date.Time{
			Time: startTime,
		},
		ExitCode: &exitCode,
		FinishTime: &date.Time{
			Time: finishTime,
		},
		DetailStatus: &emptyStr,
	}
}

func getPodConditions(creationTime, readyConditionTime metav1.Time, readyConditionStatus v1.ConditionStatus) []v1.PodCondition {
	return []v1.PodCondition{
		{
			Type:               v1.PodReady,
			Status:             readyConditionStatus,
			LastTransitionTime: readyConditionTime,
		}, {
			Type:               v1.PodInitialized,
			Status:             v1.ConditionTrue,
			LastTransitionTime: creationTime,
		}, {
			Type:               v1.PodScheduled,
			Status:             v1.ConditionTrue,
			LastTransitionTime: creationTime,
		},
	}
}
