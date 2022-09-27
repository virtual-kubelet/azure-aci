package client

import (
	"context"
	"net/http"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest"
)

const (
	DefaultUserAgent = "virtual-kubelet/azure-arm-aci"
)

type ContainerGroupPropertiesWrapper struct {
	azaci.ContainerGroupProperties
	Extensions []*Extension `json:"client,omitempty"`
}

type ContainerGroupWrapper struct {
	autorest.Response `json:"-"`
	// Identity - The identity of the container group, if configured.
	Identity *azaci.ContainerGroupIdentity `json:"identity,omitempty"`
	// ContainerGroupProperties - The container group properties
	*ContainerGroupPropertiesWrapper `json:"properties,omitempty"`
	// ID - READ-ONLY; The resource id.
	ID *string `json:"id,omitempty"`
	// Name - READ-ONLY; The resource name.
	Name *string `json:"name,omitempty"`
	// Type - READ-ONLY; The resource type.
	Type *string `json:"type,omitempty"`
	// Location - The resource location.
	Location *string `json:"location,omitempty"`
	// Tags - The resource tags.
	Tags map[string]*string `json:"tags"`
	// Zones - The zones for the container group.
	Zones *[]string `json:"zones,omitempty"`
}

type ContainerGroupsClientWrapper struct {
	CGClient azaci.ContainerGroupsClient
}

func (c *ContainerGroupsClientWrapper) CreateCG(ctx context.Context, resourceGroupName, containerGroupName string, containerGroup ContainerGroupWrapper) error {

	addReq, err := c.createOrUpdatePreparerWrapper(ctx, resourceGroupName, containerGroupName, containerGroup)
	if err != nil {
		return err
	}

	result, err := c.CGClient.CreateOrUpdateSender(addReq)
	if err != nil {
		err = autorest.NewErrorWithError(err, "containerinstance.ContainerGroupsClient", "CreateOrUpdate", result.Response(), "Failure sending request")
		return err
	}
	if result.Response().StatusCode != http.StatusOK {
		err = autorest.NewErrorWithError(err, "containerinstance.ContainerGroupsClient", "CreateOrUpdate", result.Response(), "Failure Creating or updating container group")
		return err
	}
	return nil
}

// createOrUpdatePreparerWrapper prepares the CreateOrUpdate request for the wrapper.
func (c *ContainerGroupsClientWrapper) createOrUpdatePreparerWrapper(ctx context.Context, resourceGroupName string, containerGroupName string, containerGroup ContainerGroupWrapper) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"containerGroupName": autorest.Encode("path", containerGroupName),
		"resourceGroupName":  autorest.Encode("path", resourceGroupName),
		"subscriptionId":     autorest.Encode("path", c.CGClient.SubscriptionID),
	}

	const APIVersion = "2021-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(c.CGClient.BaseURI),
		autorest.WithPathParameters("/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}", pathParameters),
		autorest.WithJSON(containerGroup),
		autorest.WithQueryParameters(queryParameters))

	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}
