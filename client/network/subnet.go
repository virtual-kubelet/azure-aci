package network

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
)

const (
	subnetPath   = "subscriptions/{{.subscriptionId}}/resourcegroups/{{.resourceGroupName}}/providers/Microsoft.Network/virtualNetworks/{{.vnetName}}/subnets/{{.subnetName}}"
	subnetAction = "Microsoft.Network/virtualNetworks/subnets/action"
)

var (
	delegationName = "aciDelegation"
	serviceName    = "Microsoft.ContainerInstance/containerGroups"
)

// NewSubnetWithContainerInstanceDelegation creates the subnet instance with ACI delegation
func NewSubnetWithContainerInstanceDelegation(name, addressPrefix string) network.Subnet {
	return network.Subnet{
		Name: &name,
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: &addressPrefix,
			Delegations: &[]network.Delegation{
				{
					Name: &delegationName,
					ServiceDelegationPropertiesFormat: &network.ServiceDelegationPropertiesFormat{
						ServiceName: &serviceName,
						Actions:     &[]string{subnetAction},
					},
				},
			},
		},
	}
}

// GetSubnet gets the subnet from the specified resourcegroup/vnet
func (c *Client) GetSubnet(ctx context.Context, resourceGroup, vnet, name string) (network.Subnet, error) {
	return c.sc.Get(ctx, resourceGroup, vnet, name, "")
	/*
		urlParams := url.Values{
			"api-version": []string{apiVersion},
		}

		// Create the url.
		uri := api.ResolveRelative(c.sc.BaseClient.BaseURI, subnetPath)
		uri += "?" + url.Values(urlParams).Encode()

		req, err := http.NewRequest("GET", uri, nil)
		if err != nil {
			return nil, errors.Wrap(err, "creating subnet get uri request failed")
		}

		if err := api.ExpandURL(req.URL, map[string]string{
			"subscriptionId":    c.auth.SubscriptionID,
			"resourceGroupName": resourceGroup,
			"subnetName":        name,
			"vnetName":          vnet,
		}); err != nil {
			return nil, errors.Wrap(err, "expanding URL with parameters failed")
		}

		// Send the request.
		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, "sending subnet get request failed")
		}
		defer resp.Body.Close()

		// 200 (OK) is a success response.
		if err := api.CheckResponse(resp); err != nil {
			return nil, err
		}

		var subnet network.Subnet
		if err := json.NewDecoder(resp.Body).Decode(&subnet); err != nil {
			return nil, err
		}
		return &subnet, nil
	*/
}

// CreateOrUpdateSubnet creates a new or updates an existing subnet in the defined resourcegroup/vnet
func (c *Client) CreateOrUpdateSubnet(ctx context.Context, resourceGroup, vnet string, s network.Subnet) (network.Subnet, error) {
	future, err := c.sc.CreateOrUpdate(ctx, resourceGroup, vnet, *s.Name, s)
	if err != nil {
		return network.Subnet{}, err
	}

	err = future.WaitForCompletionRef(ctx, c.sc.Client)
	if err != nil {
		return network.Subnet{}, err
	}

	return future.Result(c.sc)
	/*
		urlParams := url.Values{
			"api-version": []string{apiVersion},
		}

		// Create the url.
		uri := api.ResolveRelative(c.sc.BaseClient.BaseURI, subnetPath)
		uri += "?" + url.Values(urlParams).Encode()

		// Create the request.
		b, err := json.Marshal(s)
		if err != nil {
			return nil, errors.Wrap(err, "marshallig networking profile failed")
		}

		req, err := http.NewRequest("PUT", uri, bytes.NewReader(b))
		if err != nil {
			return nil, errors.New("creating subnet create uri request failed")
		}

		// Add the parameters to the url.
		if err := api.ExpandURL(req.URL, map[string]string{
			"subscriptionId":    c.auth.SubscriptionID,
			"resourceGroupName": resourceGroup,
			"subnetName":        *s.Name,
			"vnetName":          vnet,
		}); err != nil {
			return nil, errors.Wrap(err, "expanding URL with parameters failed")
		}

		// Send the request.
		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, "sending create subnet request failed")
		}
		defer resp.Body.Close()

		// 200 (OK) is a success response.
		if err := api.CheckResponse(resp); err != nil {
			return nil, err
		}

		var subnet network.Subnet
		if err := json.NewDecoder(resp.Body).Decode(&subnet); err != nil {
			return nil, err
		}
		return &subnet, nil
	*/
}
