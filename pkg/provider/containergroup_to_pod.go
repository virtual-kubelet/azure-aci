/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"fmt"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/tests"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func containerGroupToPod(cg *azaci.ContainerGroup) (*v1.Pod, error) {
	_, creationTime, err := getACIResourceMetaFromContainerGroup(cg)
	if err != nil {
		return nil, err
	}

	if cg == nil {
		return nil, errors.Errorf("container group %s cannot be nil", *cg.Name)
	}
	if cg.Containers == nil {
		return nil, errors.Errorf("containers list cannot be nil for container group %s", *cg.Name)
	}
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

	podState, err := getPodStatusFromContainerGroup(cg)
	if err != nil {
		return nil, err
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
		Status: *podState,
	}

	return &p, nil
}

func getPodStatusFromContainerGroup(cg *azaci.ContainerGroup) (*v1.PodStatus, error) {
	allReady := true
	var firstContainerStartTime, lastUpdateTime metav1.Time

	if cg.Containers == nil {
		return nil, errors.Errorf("containers list cannot be nil for container group %s", *cg.Name)
	}
	containerStatuses := make([]v1.ContainerStatus, 0, len(*cg.Containers))
	containersList := *cg.Containers

	for i := range containersList {
		err := validateContainer(containersList[i])
		if err != nil {
			return nil, err
		}
		firstContainerStartTime := metav1.NewTime(containersList[0].InstanceView.CurrentState.StartTime.Time)
		lastUpdateTime := firstContainerStartTime

		containerStatus := v1.ContainerStatus{
			Name:                 *containersList[i].Name,
			State:                aciContainerStateToContainerState(containersList[i].InstanceView.CurrentState),
			LastTerminationState: aciContainerStateToContainerState(containersList[i].InstanceView.PreviousState),
			Ready:                getPodPhaseFromACIState(*containersList[i].InstanceView.CurrentState.State) == v1.PodRunning,
			RestartCount:         *containersList[i].InstanceView.RestartCount,
			Image:                *containersList[i].Image,
			ImageID:              "",
			ContainerID:          getContainerID(cg.ID, containersList[i].Name),
		}

		if getPodPhaseFromACIState(*containersList[i].InstanceView.CurrentState.State) != v1.PodRunning &&
			getPodPhaseFromACIState(*containersList[i].InstanceView.CurrentState.State) != v1.PodSucceeded {
			allReady = false
		}

		containerStartTime := metav1.NewTime(containersList[i].ContainerProperties.InstanceView.CurrentState.StartTime.Time)
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

	aciState, creationTime, err := getACIResourceMetaFromContainerGroup(cg)
	if err != nil {
		return nil, err
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
	}, nil
}

func aciContainerStateToContainerState(cs *azaci.ContainerState) v1.ContainerState {
	switch *cs.State {
	case "Running":
		return v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: metav1.NewTime(cs.StartTime.Time),
			},
		}
	// Handle the case of completion.
	case "Succeeded":
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				StartedAt:  metav1.NewTime(cs.StartTime.Time),
				Reason:     "Completed",
				FinishedAt: metav1.NewTime(cs.FinishTime.Time),
			},
		}
	// Handle the case where the container failed.
	case "Failed", "Canceled":
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   *cs.ExitCode,
				Reason:     *cs.State,
				Message:    *cs.DetailStatus,
				StartedAt:  metav1.NewTime(cs.StartTime.Time),
				FinishedAt: metav1.NewTime(cs.FinishTime.Time),
			},
		}
	default:
		// Handle the case where the container is pending.
		// Which should be all other aci states.
		return v1.ContainerState{
			Waiting: &v1.ContainerStateWaiting{
				Reason:  *cs.State,
				Message: *cs.DetailStatus,
			},
		}
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

func getACIResourceMetaFromContainerGroup(cg *azaci.ContainerGroup) (*string, metav1.Time, error) {
	if cg == nil {
		return nil, metav1.Now(), errors.Errorf("container group %s cannot be nil", *cg.Name)
	}
	if cg.ContainerGroupProperties == nil {
		return nil, metav1.Now(), errors.Errorf("container group properties %s cannot be nil", *cg.Name)
	}

	// Use the Provisioning State if it's not Succeeded,
	// otherwise use the state of the instance.
	aciState := cg.ContainerGroupProperties.ProvisioningState
	if *aciState == "Succeeded" {
		aciState = cg.ContainerGroupProperties.InstanceView.State
	}

	var creationTime metav1.Time

	if cg.Tags == nil {
		return nil, metav1.Now(), errors.Errorf("container group tags %s cannot be nil", *cg.Name)
	}
	ts := *cg.Tags["CreationTimestamp"]

	if ts != "" {
		t, err := time.Parse(tests.TimeLayout, ts)
		if err != nil {
			return nil, metav1.Now(), errors.Errorf("unable to parse the creation timestamp for container group %s", *cg.Name)
		}
		creationTime = metav1.NewTime(t)
	}

	return aciState, creationTime, nil
}

func validateContainer(container azaci.Container) error {
	emptyStr := ""
	if container.Name == nil {
		return errors.Errorf("container name cannot be nil")
	}
	if container.Ports == nil {
		return errors.Errorf("container %s Ports cannot be nil", *container.Name)
	}
	if container.Image == nil {
		return errors.Errorf("container %s Image cannot be nil", *container.Name)
	}
	if container.ContainerProperties == nil {
		return errors.Errorf("container %s properties cannot be nil", *container.Name)
	}
	if container.InstanceView == nil {
		return errors.Errorf("container %s properties InstanceView cannot be nil", *container.Name)
	}
	if container.InstanceView.CurrentState == nil {
		return errors.Errorf("container %s properties CurrentState cannot be nil", *container.Name)
	}
	if container.InstanceView.CurrentState.StartTime == nil {
		return errors.Errorf("container %s properties CurrentState StartTime cannot be nil", *container.Name)
	}
	if container.InstanceView.PreviousState == nil {
		container.InstanceView.PreviousState = &azaci.ContainerState{
			State:        &emptyStr,
			StartTime:    container.InstanceView.CurrentState.StartTime,
			DetailStatus: &emptyStr,
		}
		return nil
	}
	if container.InstanceView.RestartCount == nil {
		return errors.Errorf("container %s properties RestartCount cannot be nil", *container.Name)
	}
	if container.InstanceView.Events == nil {
		return errors.Errorf("container %s properties Events cannot be nil", *container.Name)
	}

	return nil
}
