package validation

import (
	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
)

func ValidateContainer(container *azaci.Container) error {

	if container.Name == nil {
		return errors.Errorf("container name cannot be nil")
	}
	if container.Properties.Ports == nil {
		return errors.Errorf("container %s Ports cannot be nil", *container.Name)
	}
	if container.Properties.Image == nil {
		return errors.Errorf("container %s Image cannot be nil", *container.Name)
	}
	if container.Properties == nil {
		return errors.Errorf("container %s properties cannot be nil", *container.Name)
	}
	if container.Properties.InstanceView == nil {
		return errors.Errorf("container %s properties InstanceView cannot be nil", *container.Name)
	}
	if container.Properties.InstanceView.CurrentState == nil {
		return errors.Errorf("container %s properties CurrentState cannot be nil", *container.Name)
	}
	if container.Properties.InstanceView.CurrentState.StartTime == nil {
		return errors.Errorf("container %s properties CurrentState StartTime cannot be nil", *container.Name)
	}
	if container.Properties.InstanceView.PreviousState == nil {
		pendingState := "Pending"
		container.Properties.InstanceView.PreviousState = &azaci.ContainerState{
			State:        &pendingState,
			DetailStatus: &pendingState,
		}
		return nil
	}
	if container.Properties.InstanceView.RestartCount == nil {
		return errors.Errorf("container %s properties RestartCount cannot be nil", *container.Name)
	}
	if container.Properties.InstanceView.Events == nil {
		return errors.Errorf("container %s properties Events cannot be nil", *container.Name)
	}

	return nil
}

func ValidateContainerGroup(cg azaci.ContainerGroup) error {
	if &cg == nil {
		return errors.Errorf("container group cannot be nil")
	}
	if cg.Name == nil {
		return errors.Errorf("container group Name cannot be nil")
	}
	if cg.ID == nil {
		return errors.Errorf("container group ID cannot be nil, name: %s", *cg.Name)
	}
	if cg.Properties == nil {
		return errors.Errorf("container group properties cannot be nil, name: %s", *cg.Name)
	}
	if cg.Properties.Containers == nil {
		return errors.Errorf("containers list cannot be nil for container group %s", *cg.Name)
	}
	if cg.Tags == nil {
		return errors.Errorf("tags list cannot be nil for container group %s", *cg.Name)
	}
	if cg.Properties.OSType != &util.WindowsType {
		if cg.Properties.IPAddress == nil {
			return errors.Errorf("IPAddress cannot be nil for container group %s", *cg.Name)
		} else {
			aciState := *cg.Properties.ProvisioningState
			if cg.Properties.IPAddress.IP == nil {
				if aciState == "Running" {
					return errors.Errorf("podIP cannot be nil for container group %s while state is %s ", *cg.Name, aciState)
				} else {
					emptyIP := ""
					cg.Properties.IPAddress.IP = &emptyIP
				}
			}
		}
	}
	return nil
}
