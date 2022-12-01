package validation

import (
	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/pkg/errors"
)

func ValidateContainer(container containerinstance.Container) error {

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
		pendingState := "Pending"
		container.InstanceView.PreviousState = &containerinstance.ContainerState{
			State:        &pendingState,
			DetailStatus: &pendingState,
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

func ValidateContainerGroup(cg *containerinstance.ContainerGroup) error {
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
	if cg.OsType != containerinstance.OperatingSystemTypesWindows {
		if cg.IPAddress == nil {
			return errors.Errorf("IPAddress cannot be nil for container group %s", *cg.Name)
		} else {
			aciState := *cg.ContainerGroupProperties.ProvisioningState
			if cg.IPAddress.IP == nil {
				if aciState == "Running" {
					return errors.Errorf("podIP cannot be nil for container group %s while state is %s ", *cg.Name, aciState)
				} else {
					emptyIP := ""
					cg.IPAddress.IP = &emptyIP
				}
			}
		}
	}
	return nil
}
