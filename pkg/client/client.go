package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"

	"github.com/Azure/go-autorest/autorest"
	"github.com/virtual-kubelet/virtual-kubelet/log"
)

const (
	APIVersion            = "2021-10-01"
	containerGroupURLPath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerInstance/containerGroups/{containerGroupName}"
)

type ContainerGroupPropertiesWrapper struct {
	ContainerGroupProperties *azaciv2.ContainerGroupProperties
	Extensions               []*azaciv2.DeploymentExtensionSpec
}

type ContainerGroupWrapper struct {
	autorest.Response `json:"-"`
	// Identity - The identity of the container group, if configured.
	Identity *azaciv2.ContainerGroupIdentity `json:"identity,omitempty"`
	// ContainerGroupProperties - The container group properties
	ContainerGroupPropertiesWrapper *ContainerGroupPropertiesWrapper `json:"properties,omitempty"`
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

func (c *ContainerGroupsClientWrapper) CreateCG(ctx context.Context, resourceGroupName string, containerGroup ContainerGroupWrapper) error {
	logger := log.G(ctx).WithField("method", "CreateCG")

	addReq, err := c.createOrUpdatePreparerWrapper(ctx, resourceGroupName, *containerGroup.Name, containerGroup)
	if err != nil {
		return err
	}

	result, err := c.CGClient.CreateOrUpdateSender(addReq)
	logger.Infof("CreateCG status code: %s", result.Status())

	if err != nil {
		err = autorest.NewErrorWithError(err, "containerinstance.ContainerGroupsClient", "CreateOrUpdateSender", result.Response(), "Failure sending request")
		return err
	}

	// 200 (OK) and 201 (Created) are a successful responses.
	if result.Response() != nil {
		if result.Response().StatusCode != http.StatusOK && result.Response().StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to create container group %s, status code %d ", *containerGroup.Name, result.Response().StatusCode)
		}
	}

	logger.Infof("container group %s has created successfully", *containerGroup.Name)
	return nil
}

// createOrUpdatePreparerWrapper prepares the CreateOrUpdateSender request for the wrapper.
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

// MarshalJSON is the custom marshal for ContainerGroupProperties.
func (cg ContainerGroupPropertiesWrapper) MarshalJSON() ([]byte, error) {
	objectMap := make(map[string]interface{})
	if cg.ContainerGroupProperties.Properties.Containers != nil {
		objectMap["containers"] = cg.ContainerGroupProperties.Properties.Containers
	}
	if cg.ContainerGroupProperties.Properties.ImageRegistryCredentials != nil {
		objectMap["imageRegistryCredentials"] = cg.ContainerGroupProperties.Properties.ImageRegistryCredentials
	}
	if cg.ContainerGroupProperties.Properties.RestartPolicy != nil {
		objectMap["restartPolicy"] = cg.ContainerGroupProperties.Properties.RestartPolicy
	}
	if cg.ContainerGroupProperties.Properties.IPAddress != nil {
		objectMap["ipAddress"] = cg.ContainerGroupProperties.Properties.IPAddress
	}
	if cg.ContainerGroupProperties.Properties.OSType != nil {
		objectMap["osType"] = cg.ContainerGroupProperties.Properties.OSType
	}
	if cg.ContainerGroupProperties.Properties.Volumes != nil {
		objectMap["volumes"] = cg.ContainerGroupProperties.Properties.Volumes
	}
	if cg.ContainerGroupProperties.Properties.Diagnostics != nil {
		objectMap["diagnostics"] = cg.ContainerGroupProperties.Properties.Diagnostics
	}
	if cg.ContainerGroupProperties.Properties.SubnetIDs != nil {
		objectMap["subnetIds"] = cg.ContainerGroupProperties.Properties.SubnetIDs
	}
	if cg.ContainerGroupProperties.Properties.DNSConfig != nil {
		objectMap["dnsConfig"] = cg.ContainerGroupProperties.Properties.DNSConfig
	}
	if cg.ContainerGroupProperties.Properties.SKU != nil {
		objectMap["sku"] = cg.ContainerGroupProperties.Properties.SKU
	}
	if cg.ContainerGroupProperties.Properties.EncryptionProperties != nil {
		objectMap["encryptionProperties"] = cg.ContainerGroupProperties.Properties.EncryptionProperties
	}
	if cg.ContainerGroupProperties.Properties.InitContainers != nil {
		objectMap["initContainers"] = cg.ContainerGroupProperties.Properties.InitContainers
	}
	if cg.Extensions != nil {
		objectMap["extensions"] = cg.Extensions
	}
	return json.Marshal(objectMap)
}
