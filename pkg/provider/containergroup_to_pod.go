/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"fmt"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func containerGroupToPod(cg *azaci.ContainerGroup) (*v1.Pod, error) {
	_, creationTime := getACIResourceMetaFromContainerGroup(cg)

	containers := make([]v1.Container, 0, len(*cg.Containers))
	containersList := *cg.Containers
	for i := range containersList {
		container := &v1.Container{
			Name:  *containersList[i].Name,
			Image: *containersList[i].Image,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", *containersList[i].Resources.Requests.CPU)),
					v1.ResourceMemory: resource.MustParse(fmt.Sprintf("%gG", *containersList[i].Resources.Requests.MemoryInGB)),
				},
			},
		}
		if containersList[i].Command != nil {
			container.Command = *containersList[i].Command
		}

		if containersList[i].Resources.Requests.Gpu != nil {
			container.Resources.Requests[gpuResourceName] = resource.MustParse(fmt.Sprintf("%d", *containersList[i].Resources.Requests.Gpu.Count))
		}

		if containersList[i].Resources.Limits != nil {
			container.Resources.Limits = v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", *containersList[i].Resources.Limits.CPU)),
				v1.ResourceMemory: resource.MustParse(fmt.Sprintf("%gG", *containersList[i].Resources.Limits.MemoryInGB)),
			}

			if containersList[i].Resources.Limits.Gpu != nil {
				container.Resources.Limits[gpuResourceName] = resource.MustParse(fmt.Sprintf("%d", *containersList[i].Resources.Requests.Gpu.Count))
			}
		}

		containers = append(containers, *container)
	}

	p := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              *cg.Tags["PodName"],
			Namespace:         *cg.Tags["Namespace"],
			ClusterName:       *cg.Tags["ClusterName"],
			UID:               types.UID(*cg.Tags["UID"]),
			CreationTimestamp: creationTime,
		},
		Spec: v1.PodSpec{
			NodeName:   *cg.Tags["NodeName"],
			Volumes:    []v1.Volume{},
			Containers: containers,
		},
		Status: *getPodStatusFromContainerGroup(cg),
	}

	return &p, nil
}

func getPodStatusFromContainerGroup(cg *azaci.ContainerGroup) *v1.PodStatus {
	allReady := true

	aciState, creationTime := getACIResourceMetaFromContainerGroup(cg)
	containerStatuses := make([]v1.ContainerStatus, 0, len(*cg.Containers))

	lastUpdateTime := creationTime
	firstContainerStartTime := creationTime
	containerStartTime := creationTime
	containerStatus := v1.ContainerStatus{}
	if *aciState == "Succeeded" {
		aciState = cg.ContainerGroupProperties.InstanceView.State
		firstContainerStartTime = metav1.NewTime((*cg.Containers)[0].ContainerProperties.InstanceView.CurrentState.StartTime.Time)
		lastUpdateTime = firstContainerStartTime

		for _, c := range *cg.Containers {
			containerStartTime = metav1.NewTime(c.ContainerProperties.InstanceView.CurrentState.StartTime.Time)
			containerStatus = v1.ContainerStatus{
				Name:                 *c.Name,
				State:                aciContainerStateToContainerState(*c.InstanceView.CurrentState),
				LastTerminationState: aciContainerStateToContainerState(*c.InstanceView.PreviousState),
				Ready:                getPodPhaseFromACIState(*c.InstanceView.CurrentState.State) == v1.PodRunning,
				RestartCount:         *c.InstanceView.RestartCount,
				Image:                *c.Image,
				ImageID:              "",
				ContainerID:          getContainerID(*cg.ID, *c.Name),
			}

			if getPodPhaseFromACIState(*c.InstanceView.CurrentState.State) != v1.PodRunning &&
				getPodPhaseFromACIState(*c.InstanceView.CurrentState.State) != v1.PodSucceeded {
				allReady = false
			}
		}
		if containerStartTime.Time.After(lastUpdateTime.Time) {
			lastUpdateTime = containerStartTime
		}

		// Add to containerStatuses
		containerStatuses = append(containerStatuses, containerStatus)
	}

	ip := ""
	if cg.IPAddress != nil {
		ip = *cg.IPAddress.IP
	}

	return &v1.PodStatus{
		Phase:             getPodPhaseFromACIState(*aciState),
		Conditions:        getPodConditionsFromACIState(*aciState, creationTime, lastUpdateTime, allReady),
		Message:           "",
		Reason:            "",
		HostIP:            "",
		PodIP:             ip,
		StartTime:         &firstContainerStartTime,
		ContainerStatuses: containerStatuses,
	}
}

func aciContainerStateToContainerState(cs azaci.ContainerState) v1.ContainerState {
	startTime := metav1.NewTime(cs.StartTime.Time)
	state := *cs.State
	// Handle the case where the container is running.
	if state == "Running" {
		return v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: startTime,
			},
		}
	}

	// Handle the case of completion.
	if state == "Succeeded" {
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				StartedAt:  startTime,
				Reason:     "Completed",
				FinishedAt: metav1.NewTime(cs.FinishTime.Time),
			},
		}
	}

	// Handle the case where the container failed.
	if state == "Failed" || state == "Canceled" {
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   *cs.ExitCode,
				Reason:     *cs.State,
				Message:    *cs.DetailStatus,
				StartedAt:  startTime,
				FinishedAt: metav1.NewTime(cs.FinishTime.Time),
			},
		}
	}

	if state == "" {
		state = "Creating"
	}

	// Handle the case where the container is pending.
	// Which should be all other aci states.
	return v1.ContainerState{
		Waiting: &v1.ContainerStateWaiting{
			Reason:  state,
			Message: *cs.DetailStatus,
		},
	}
}

func getPodPhaseFromACIState(state string) v1.PodPhase {
	switch state {
	case "Running":
		return v1.PodRunning
	case "Succeeded":
		return v1.PodSucceeded
	case "Failed":
		return v1.PodFailed
	case "Canceled":
		return v1.PodFailed
	case "Creating":
		return v1.PodPending
	case "Repairing":
		return v1.PodPending
	case "Pending":
		return v1.PodPending
	case "Accepted":
		return v1.PodPending
	}

	return v1.PodUnknown
}

func getPodConditionsFromACIState(state string, creationTime, lastUpdateTime metav1.Time, allReady bool) []v1.PodCondition {
	switch state {
	case "Running", "Succeeded":
		readyConditionStatus := v1.ConditionFalse
		readyConditionTime := creationTime
		if allReady {
			readyConditionStatus = v1.ConditionTrue
			readyConditionTime = lastUpdateTime
		}

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
	return []v1.PodCondition{}
}
