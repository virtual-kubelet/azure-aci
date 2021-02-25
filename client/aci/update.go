package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
)

// UpdateContainerGroup updates an Azure Container Instance with the
// provided properties.
// From: https://docs.microsoft.com/en-us/rest/api/container-instances/containergroups/createorupdate
func (c *Client) UpdateContainerGroup(ctx context.Context, resourceGroup, containerGroupName string, containerGroup containerinstance.ContainerGroup) (containerinstance.ContainerGroup, error) {
	return c.CreateContainerGroup(ctx, resourceGroup, containerGroupName, containerGroup)
}
