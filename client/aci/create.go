package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
)

// CreateContainerGroup creates a new Azure Container Instance with the
// provided properties.
// From: https://docs.microsoft.com/en-us/rest/api/container-instances/containergroups/createorupdate
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

	/*
		urlParams := url.Values{
			"api-version": []string{apiVersion},
		}

		// Create the url.
		uri := api.ResolveRelative(c.auth.ResourceManagerEndpoint, containerGroupURLPath)
		uri += "?" + url.Values(urlParams).Encode()

		// Create the body for the request.
		b := new(bytes.Buffer)
		if err := json.NewEncoder(b).Encode(containerGroup); err != nil {
			return nil, fmt.Errorf("Encoding create container group body request failed: %v", err)
		}

		// Create the request.
		req, err := http.NewRequest("PUT", uri, b)
		if err != nil {
			return nil, fmt.Errorf("Creating create/update container group uri request failed: %v", err)
		}
		req = req.WithContext(ctx)

		// Add the parameters to the url.
		if err := api.ExpandURL(req.URL, map[string]string{
			"subscriptionId":     c.auth.SubscriptionID,
			"resourceGroup":      resourceGroup,
			"containerGroupName": containerGroupName,
		}); err != nil {
			return nil, fmt.Errorf("Expanding URL with parameters failed: %v", err)
		}

		// Send the request.
		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Sending create container group request failed: %v", err)
		}
		defer resp.Body.Close()

		// 200 (OK) and 201 (Created) are a successful responses.
		if err := api.CheckResponse(resp); err != nil {
			return nil, err
		}

		// Decode the body from the response.
		if resp.Body == nil {
			return nil, errors.New("Create container group returned an empty body in the response")
		}
		var cg ContainerGroup
		if err := json.NewDecoder(resp.Body).Decode(&cg); err != nil {
			return nil, fmt.Errorf("Decoding create container group response body failed: %v", err)
		}

		return &cg, nil
	*/
}
