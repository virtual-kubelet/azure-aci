package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
)

// UpdateContainerGroup updates an Azure Container Instance with the
// provided properties.
func (c *Client) UpdateContainerGroup(ctx context.Context, resourceGroup, containerGroupName string, containerGroup containerinstance.ContainerGroup) (containerinstance.ContainerGroup, error) {
	return c.CreateContainerGroup(ctx, resourceGroup, containerGroupName, containerGroup)
}
