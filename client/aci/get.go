package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
)

// GetContainerGroup gets an Azure Container Instance in the provided
// resource group with the given container group name.
func (c *Client) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (containerinstance.ContainerGroup, error) {
	return c.containerGroupsClient.Get(ctx, resourceGroup, containerGroupName)
}
