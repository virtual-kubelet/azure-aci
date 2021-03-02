package aci

import (
	"context"
)

// DeleteContainerGroup deletes an Azure Container Instance in the provided
// resource group with the given container group name.
func (c *Client) DeleteContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) error {
	future, err := c.containerGroupsClient.Delete(ctx, resourceGroup, containerGroupName)

	if err != nil {
		return err
	}

	err = future.WaitForCompletionRef(ctx, c.containerGroupsClient.Client)
	if err != nil {
		return err
	}

	return nil
}
