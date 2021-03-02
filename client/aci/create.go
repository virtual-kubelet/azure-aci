package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
)

// CreateContainerGroup creates a new Azure Container Instance with the
// provided properties.
func (c *Client) CreateContainerGroup(ctx context.Context, resourceGroup, containerGroupName string, containerGroup containerinstance.ContainerGroup) (containerinstance.ContainerGroup, error) {
	future, err := c.containerGroupsClient.CreateOrUpdate(ctx, resourceGroup, containerGroupName, containerGroup)

	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}

	err = future.WaitForCompletionRef(ctx, c.containerGroupsClient.Client)
	if err != nil {
		return containerinstance.ContainerGroup{}, err
	}

	return future.Result(c.containerGroupsClient)
}
