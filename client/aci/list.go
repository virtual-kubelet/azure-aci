package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
)

// ListContainerGroups lists an Azure Container Instance Groups, if a resource
// group is given it will list by resource group.
// It optionally accepts a resource group name and will filter based off of it
// if it is not empty.
// From: https://docs.microsoft.com/en-us/rest/api/container-instances/containergroups/list
// From: https://docs.microsoft.com/en-us/rest/api/container-instances/containergroups/listbyresourcegroup
func (c *Client) ListContainerGroups(ctx context.Context, resourceGroup string) (containerinstance.ContainerGroupListResult, error) {
	var result containerinstance.ContainerGroupListResultPage
	var err error
	if resourceGroup == "" {
		result, err = c.containerGroupsClient.List(ctx)
	} else {
		result, err = c.containerGroupsClient.ListByResourceGroup(ctx, resourceGroup)
	}

	if err != nil {
		return containerinstance.ContainerGroupListResult{}, err
	}

	return result.Response(), nil
	//TODO: pagination!
}
