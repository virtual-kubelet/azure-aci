package resourcegroups

import "context"

// UpdateResourceGroup updates an Azure resource group with the
// provided properties.
// From: https://docs.microsoft.com/en-us/rest/api/resources/resourcegroups/createorupdate
func (c *Client) UpdateResourceGroup(ctx context.Context, resourceGroup string, properties Group) (*Group, error) {
	return c.CreateResourceGroup(ctx, resourceGroup, properties)
}
