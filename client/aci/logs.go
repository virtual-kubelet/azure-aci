package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
)

// GetContainerLogs returns the logs from an Azure Container Instance
// in the provided resource group with the given container group name.
func (c *Client) GetContainerLogs(ctx context.Context, resourceGroup, containerGroupName, containerName string, tail int) (*containerinstance.Logs, error) {
	var tailPtr *int32
	if tail != 0 {
		tailPtr = to.Int32Ptr(int32(tail))
	}
	result, err := c.containersClient.ListLogs(ctx, resourceGroup, containerGroupName, containerName, tailPtr)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
