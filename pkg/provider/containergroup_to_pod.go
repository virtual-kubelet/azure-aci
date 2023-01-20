/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/tests"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	"github.com/virtual-kubelet/azure-aci/pkg/validation"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *ACIProvider) containerGroupToPod(cg *azaci.ContainerGroup) (*v1.Pod, error) {
	//cg is validated
	pod, err := p.resourceManager.GetPod(*cg.Name, *cg.Tags["Namespace"])
	if err != nil {
		return nil, err
	}

	updatedPod := pod.DeepCopy()

	podState, err := p.getPodStatusFromContainerGroup(cg)
	if err != nil {
		return nil, err
	}

	updatedPod.Status = *podState

	return updatedPod, nil
}

func (p *ACIProvider) getPodStatusFromContainerGroup(cg *azaci.ContainerGroup) (*v1.PodStatus, error) {
	// cg is validated
	allReady := true
	var firstContainerStartTime, lastUpdateTime time.Time

	containerStatuses := make([]v1.ContainerStatus, 0, len(cg.Properties.Containers))
	containersList := cg.Properties.Containers

	for i := range containersList {
		err := validation.ValidateContainer(containersList[i])
		if err != nil {
			return nil, err
		}
		firstContainerStartTime := containersList[0].Properties.InstanceView.CurrentState.StartTime
		lastUpdateTime = *firstContainerStartTime

		containerStatus := v1.ContainerStatus{
			Name:                 *containersList[i].Name,
			State:                aciContainerStateToContainerState(containersList[i].Properties.InstanceView.CurrentState),
			LastTerminationState: aciContainerStateToContainerState(containersList[i].Properties.InstanceView.PreviousState),
			Ready:                getPodPhaseFromACIState(*containersList[i].Properties.InstanceView.CurrentState.State) == v1.PodRunning,
			RestartCount:         *containersList[i].Properties.InstanceView.RestartCount,
			Image:                *containersList[i].Properties.Image,
			ImageID:              "",
			ContainerID:          util.GetContainerID(cg.ID, containersList[i].Name),
		}

		if getPodPhaseFromACIState(*containersList[i].Properties.InstanceView.CurrentState.State) != v1.PodRunning &&
			getPodPhaseFromACIState(*containersList[i].Properties.InstanceView.CurrentState.State) != v1.PodSucceeded {
			allReady = false
		}

		containerStartTime := containersList[i].Properties.InstanceView.CurrentState.StartTime
		if containerStartTime.After(lastUpdateTime) {
			lastUpdateTime = *containerStartTime
		}

		// Add to containerStatuses
		containerStatuses = append(containerStatuses, containerStatus)
	}

	aciState, creationTime, err := getACIResourceMetaFromContainerGroup(cg)
	if err != nil {
		return nil, err
	}

	podIp := ""
	if cg.Properties.OSType != &util.WindowsType {
		podIp = *cg.Properties.IPAddress.IP
	}
	return &v1.PodStatus{
		Phase:             getPodPhaseFromACIState(*aciState),
		Conditions:        getPodConditionsFromACIState(*aciState, creationTime, lastUpdateTime, allReady),
		Message:           "",
		Reason:            "",
		HostIP:            p.internalIP,
		PodIP:             podIp,
		StartTime:         &metav1.Time{Time: firstContainerStartTime},
		ContainerStatuses: containerStatuses,
	}, nil
}

func aciContainerStateToContainerState(cs *azaci.ContainerState) v1.ContainerState {
	// cg container state is validated
	startTime := *cs.StartTime
	finishTime := *cs.FinishTime
	switch *cs.State {
	case "Running":
		return v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: metav1.NewTime(startTime),
			},
		}
	// Handle the case of completion.
	case "Succeeded":
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				StartedAt:  metav1.NewTime(startTime),
				Reason:     "Completed",
				FinishedAt: metav1.NewTime(finishTime),
			},
		}
	// Handle the case where the container failed.
	case "Failed", "Canceled":
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   *cs.ExitCode,
				Reason:     *cs.State,
				Message:    *cs.DetailStatus,
				StartedAt:  metav1.NewTime(startTime),
				FinishedAt: metav1.NewTime(finishTime),
			},
		}
		// Handle windows container with no prev state
	case "Pending":
		return v1.ContainerState{
			Waiting: &v1.ContainerStateWaiting{
				Reason:  *cs.State,
				Message: *cs.DetailStatus,
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

func getPodConditionsFromACIState(state string, creationTime, lastUpdateTime time.Time, allReady bool) []v1.PodCondition {
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
				LastTransitionTime: metav1.Time{Time: readyConditionTime},
			}, {
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: creationTime},
			}, {
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: creationTime},
			},
		}
	}
	return []v1.PodCondition{}
}

func getACIResourceMetaFromContainerGroup(cg *azaci.ContainerGroup) (*string, time.Time, error) {
	// cg is validated

	// Use the Provisioning State if it's not Succeeded,
	// otherwise use the state of the instance.
	aciState := cg.Properties.ProvisioningState
	if *aciState == "Succeeded" {
		aciState = cg.Properties.InstanceView.State
	}

	var creationTime time.Time

	// cg tags is validated
	ts := *cg.Tags["CreationTimestamp"]

	if ts != "" {
		t, err := time.Parse(tests.TimeLayout, ts)
		if err != nil {
			return nil, time.Now(), errors.Errorf("unable to parse the creation timestamp for container group %s", *cg.Name)
		}
		creationTime = t
	}

	return aciState, creationTime, nil
}
