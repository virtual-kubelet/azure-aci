package client

import (
	"context"
	"encoding/json"
	"net/http"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest"
)

const (
	DefaultUserAgent      = "virtual-kubelet/azure-arm-aci"
	APIVersion            = "2021-10-01"
	containerGroupURLPath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}"
)

type ContainerGroupPropertiesWrapper struct {
	ContainerGroupProperties *azaci.ContainerGroupProperties
	Extensions               []*Extension `json:"extensions,omitempty"`
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
	containerGroup.Name = &containerGroupName
	addReq, err := c.createOrUpdatePreparerWrapper(ctx, resourceGroupName, containerGroupName, containerGroup)
	if err != nil {
		return err
	}

	result, err := c.CGClient.CreateOrUpdateSender(addReq)
	if err != nil {
		err = autorest.NewErrorWithError(err, "containerinstance.ContainerGroupsClient", "CreateOrUpdate", result.Response(), "Failure sending request")
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

	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(c.CGClient.BaseURI),
		autorest.WithPathParameters(containerGroupURLPath, pathParameters),
		autorest.WithJSON(containerGroup),
		autorest.WithQueryParameters(queryParameters))

	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// MarshalJSON is the custom marshaler for ContainerGroupProperties.
func (cg ContainerGroupPropertiesWrapper) MarshalJSON() ([]byte, error) {
	objectMap := make(map[string]interface{})
	if cg.ContainerGroupProperties != nil {
		if cg.ContainerGroupProperties.Containers != nil {
			objectMap["containers"] = cg.ContainerGroupProperties.Containers
		}
		if cg.ContainerGroupProperties.ImageRegistryCredentials != nil {
			objectMap["imageRegistryCredentials"] = cg.ContainerGroupProperties.ImageRegistryCredentials
		}
		if cg.ContainerGroupProperties.RestartPolicy != "" {
			objectMap["restartPolicy"] = cg.ContainerGroupProperties.RestartPolicy
		}
		if cg.ContainerGroupProperties.IPAddress != nil {
			objectMap["ipAddress"] = cg.ContainerGroupProperties.IPAddress
		}
		if cg.ContainerGroupProperties.OsType != "" {
			objectMap["osType"] = cg.ContainerGroupProperties.OsType
		}
		if cg.ContainerGroupProperties.Volumes != nil {
			objectMap["volumes"] = cg.ContainerGroupProperties.Volumes
		}
		if cg.ContainerGroupProperties.Diagnostics != nil {
			objectMap["diagnostics"] = cg.ContainerGroupProperties.Diagnostics
		}
		if cg.ContainerGroupProperties.SubnetIds != nil {
			objectMap["subnetIds"] = cg.ContainerGroupProperties.SubnetIds
		}
		if cg.ContainerGroupProperties.DNSConfig != nil {
			objectMap["dnsConfig"] = cg.ContainerGroupProperties.DNSConfig
		}
		if cg.ContainerGroupProperties.Sku != "" {
			objectMap["sku"] = cg.ContainerGroupProperties.Sku
		}
		if cg.ContainerGroupProperties.EncryptionProperties != nil {
			objectMap["encryptionProperties"] = cg.ContainerGroupProperties.EncryptionProperties
		}
		if cg.ContainerGroupProperties.InitContainers != nil {
			objectMap["initContainers"] = cg.ContainerGroupProperties.InitContainers
		}
	}
	if cg.Extensions != nil {
		objectMap["extensions"] = cg.Extensions
	}
	return json.Marshal(objectMap)
}
