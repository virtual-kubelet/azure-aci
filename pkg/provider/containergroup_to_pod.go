/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/tests"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	emptyStr = ""
)

func (p *ACIProvider) containerGroupToPod(cg *azaci.ContainerGroup) (*v1.Pod, error) {
	//cg is validated
	var pod *v1.Pod
	pod, err := p.resourceManager.GetPod(*cg.Name, *cg.Tags["Namespace"])
	if err != nil {
		return nil, err
	}

	pod.DeepCopy()

	podState, err := p.getPodStatusFromContainerGroup(cg)
	if err != nil {
		return nil, err
	}

	pod.Status = *podState

	return pod, nil
}

func (p *ACIProvider) getPodStatusFromContainerGroup(cg *azaci.ContainerGroup) (*v1.PodStatus, error) {
	// cg is validated
	allReady := true
	var firstContainerStartTime, lastUpdateTime metav1.Time

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

	aciState, creationTime, err := getACIResourceMetaFromContainerGroup(cg)
	if err != nil {
		return nil, err
	}

	return &v1.PodStatus{
		Phase:             getPodPhaseFromACIState(*aciState),
		Conditions:        getPodConditionsFromACIState(*aciState, creationTime, lastUpdateTime, allReady),
		Message:           "",
		Reason:            "",
		HostIP:            p.internalIP,
		PodIP:             *cg.IPAddress.IP,
		StartTime:         &firstContainerStartTime,
		ContainerStatuses: containerStatuses,
	}, nil
}

func aciContainerStateToContainerState(cs *azaci.ContainerState) v1.ContainerState {
	// cg container state is validated
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
	// cg state is validated
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
	// cg is validated

	// Use the Provisioning State if it's not Succeeded,
	// otherwise use the state of the instance.
	aciState := cg.ContainerGroupProperties.ProvisioningState
	if *aciState == "Succeeded" {
		aciState = cg.ContainerGroupProperties.InstanceView.State
	}

	var creationTime metav1.Time

	// cg tags is validated
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

func validateContainerGroup(cg *azaci.ContainerGroup) error {
	if cg == nil {
		return errors.Errorf("container group cannot be nil")
	}
	if cg.Name == nil {
		return errors.Errorf("container group Name cannot be nil")
	}
	if cg.ID == nil {
		return errors.Errorf("container group ID cannot be nil, name: %s", *cg.Name)
	}
	if cg.ContainerGroupProperties == nil {
		return errors.Errorf("container group properties cannot be nil, name: %s", *cg.Name)
	}
	if cg.Containers == nil {
		return errors.Errorf("containers list cannot be nil for container group %s", *cg.Name)
	}
	if cg.Tags == nil {
		return errors.Errorf("tags list cannot be nil for container group %s", *cg.Name)
	}
	return nil
}

func validateContainer(container azaci.Container) error {

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
