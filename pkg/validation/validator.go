/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package validation

import (
	"context"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
)

func ValidateContainer(ctx context.Context, container *azaciv2.Container) error {
	if container.Name == nil {
		return errors.Errorf("container name cannot be nil")
	}
	if container.Properties == nil {
		return errors.Errorf("container %s properties cannot be nil", *container.Name)
	}
	if container.Properties.Ports == nil {
		return errors.Errorf("container %s Ports cannot be nil", *container.Name)
	}
	if container.Properties.Image == nil {
		return errors.Errorf("container %s Image cannot be nil", *container.Name)
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
		container.Properties.InstanceView.PreviousState = &azaciv2.ContainerState{
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
	log.G(ctx).Debugf("container %s was validated successfully!", *container.Name)
	return nil
}

func ValidateContainerGroup(ctx context.Context, cg *azaciv2.ContainerGroup) error {
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
	if cg.Properties.InstanceView == nil {
		return errors.Errorf("InstanceView cannot be nil for container group %s", *cg.Name)
	}
	if cg.Properties.InstanceView.State == nil {
		return errors.Errorf("InstanceView state cannot be nil for container group %s", *cg.Name)
	}
	if cg.Properties.OSType != nil &&
		*cg.Properties.OSType != azaciv2.OperatingSystemTypesWindows {
		if cg.Properties.IPAddress == nil {
			// In some use cases, ACI sets IPAddress as nil which can cause issues. We have to patch the struct to make the workflow continue.
			emptyIP := ""
			cg.Properties.IPAddress = &azaciv2.IPAddress{
				IP: &emptyIP,
			}
		} else {
			if cg.Properties.ProvisioningState == nil {
				return errors.Errorf("ProvisioningState cannot be nil for container group %s", *cg.Name)
			}
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
	log.G(ctx).Debugf("container group %s was validated successfully!", *cg.Name)
	return nil
}
