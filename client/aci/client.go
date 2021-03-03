package aci

import (
	"fmt"
	"net/http"

	"go.opencensus.io/plugin/ochttp/propagation/b3"

	"go.opencensus.io/plugin/ochttp"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2019-06-01/insights"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	azureaciclient "github.com/virtual-kubelet/azure-aci/client"
)

const (
	defaultUserAgent = "virtual-kubelet/azure-arm-aci/2018-10-01"
)

// Client is a client for interacting with Azure Container Instances.
//
// Clients should be reused instead of created as needed.
// The methods of Client are safe for concurrent use by multiple goroutines.
type Client struct {
	hc                    *http.Client
	auth                  *azureaciclient.Authentication
	containerGroupsClient containerinstance.ContainerGroupsClient
	containersClient      containerinstance.ContainersClient
	metricsClient         insights.MetricsClient
}

// NewClient creates a new Azure Container Instances client with extra user agent.
func NewClient(auth *azureaciclient.Authentication, extraUserAgent string) (*Client, error) {
	if auth == nil {
		return nil, fmt.Errorf("Authentication is not supplied for the Azure client")
	}

	userAgent := []string{defaultUserAgent}
	if extraUserAgent != "" {
		userAgent = append(userAgent, extraUserAgent)
	}

	client, err := azureaciclient.NewClient(auth, userAgent)
	if err != nil {
		return nil, fmt.Errorf("Creating Azure client failed: %v", err)
	}
	hc := client.HTTPClient
	hc.Transport = &ochttp.Transport{
		Base:           hc.Transport,
		Propagation:    &b3.HTTPFormat{},
		NewClientTrace: ochttp.NewSpanAnnotatingClientTrace,
	}

	var authorizer autorest.Authorizer
	if !auth.UseUserIdentity {
		config, err := adal.NewOAuthConfig(auth.ActiveDirectoryEndpoint, auth.TenantID)
		if err != nil {
			return nil, fmt.Errorf("Creating new OAuth config for active directory failed: %v", err)
		}

		spt, err := adal.NewServicePrincipalToken(*config, auth.ClientID, auth.ClientSecret, auth.ResourceManagerEndpoint)
		if err != nil {
			return nil, fmt.Errorf("Creating new service principal token failed: %v", err)
		}

		authorizer = autorest.NewBearerAuthorizer(spt)
	} else {
		endpoint, err := adal.GetMSIVMEndpoint()
		if err != nil {
			return nil, fmt.Errorf("Unable to retrieve managed identity endpoint: %v", err)
		}

		spt, err := adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(
			endpoint,
			auth.ManagementEndpoint,
			auth.UserIdentityClientId)
		if err != nil {
			return nil, fmt.Errorf("Unable to create token provider with managed identity: %v", err)
		}

		authorizer = autorest.NewBearerAuthorizer(spt)
	}

	containerGroupsClient := containerinstance.NewContainerGroupsClientWithBaseURI(auth.ResourceManagerEndpoint, auth.SubscriptionID)
	containerGroupsClient.Authorizer = authorizer

	containersClient := containerinstance.NewContainersClientWithBaseURI(auth.ResourceManagerEndpoint, auth.SubscriptionID)
	containersClient.Authorizer = authorizer

	metricsClient := insights.NewMetricsClientWithBaseURI(auth.ResourceManagerEndpoint, auth.SubscriptionID)
	metricsClient.Authorizer = authorizer

	if extraUserAgent != "" {
		if err := containerGroupsClient.AddToUserAgent(extraUserAgent); err != nil {
			return nil, fmt.Errorf("unable to add user agent: %s", err)
		}
		if err := containersClient.AddToUserAgent(extraUserAgent); err != nil {
			return nil, fmt.Errorf("unable to add user agent: %s", err)
		}
		if err := metricsClient.AddToUserAgent(extraUserAgent); err != nil {
			return nil, fmt.Errorf("unable to add user agent: %s", err)
		}
	}

	return &Client{
		hc:                    client.HTTPClient,
		auth:                  auth,
		containerGroupsClient: containerGroupsClient,
		containersClient:      containersClient,
		metricsClient:         metricsClient,
	}, nil
}
