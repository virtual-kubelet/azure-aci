package aci

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"go.opencensus.io/plugin/ochttp/propagation/b3"

	"go.opencensus.io/plugin/ochttp"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2019-06-01/insights"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	azureclient "github.com/virtual-kubelet/azure-aci/client"
)

const (
	defaultUserAgent = "virtual-kubelet/azure-arm-aci/2018-10-01"
	apiVersion       = "2018-10-01"

	containerGroupURLPath                    = "subscriptions/{{.subscriptionId}}/resourceGroups/{{.resourceGroup}}/providers/Microsoft.ContainerInstance/containerGroups/{{.containerGroupName}}"
	containerGroupListURLPath                = "subscriptions/{{.subscriptionId}}/providers/Microsoft.ContainerInstance/containerGroups"
	containerGroupListByResourceGroupURLPath = "subscriptions/{{.subscriptionId}}/resourceGroups/{{.resourceGroup}}/providers/Microsoft.ContainerInstance/containerGroups"
	containerLogsURLPath                     = containerGroupURLPath + "/containers/{{.containerName}}/logs"
	containerExecURLPath                     = containerGroupURLPath + "/containers/{{.containerName}}/exec"
	containerGroupMetricsURLPath             = containerGroupURLPath + "/providers/microsoft.Insights/metrics"
)

// Client is a client for interacting with Azure Container Instances.
//
// Clients should be reused instead of created as needed.
// The methods of Client are safe for concurrent use by multiple goroutines.
type Client struct {
	hc                    *http.Client
	auth                  *azureclient.Authentication
	containerGroupsClient containerinstance.ContainerGroupsClient
	containersClient      containerinstance.ContainersClient
	metricsClient         insights.MetricsClient
}

// NewClient creates a new Azure Container Instances client with extra user agent.
func NewClient(auth *azureclient.Authentication, extraUserAgent string) (*Client, error) {
	if auth == nil {
		return nil, fmt.Errorf("Authentication is not supplied for the Azure client")
	}

	userAgent := []string{defaultUserAgent}
	if extraUserAgent != "" {
		userAgent = append(userAgent, extraUserAgent)
	}

	client, err := azureclient.NewClient(auth, userAgent)
	if err != nil {
		return nil, fmt.Errorf("Creating Azure client failed: %v", err)
	}
	hc := client.HTTPClient
	hc.Transport = &ochttp.Transport{
		Base:           hc.Transport,
		Propagation:    &b3.HTTPFormat{},
		NewClientTrace: ochttp.NewSpanAnnotatingClientTrace,
	}

	cloudEnv, err := azure.EnvironmentFromName(auth.AzureCloud)
	var authorizer autorest.Authorizer
	if auth.UseUserIdentity {
		glog.Infof("using MSI")
		msiEP, err := adal.GetMSIVMEndpoint()
		if err != nil {
			return nil, fmt.Errorf("unable to get MSI endpoint: %s", err)
		}

		spt, err := adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(
			msiEP, cloudEnv.ResourceManagerEndpoint, auth.UserIdentityClientId)
		if err != nil {
			glog.Fatalf("unable to create MSI authorizer: %s", err)
		}

		authorizer = autorest.NewBearerAuthorizer(spt)
	} else {
		oauthConfig, err := adal.NewOAuthConfig(cloudEnv.ActiveDirectoryEndpoint, auth.TenantID)
		if err != nil {
			return nil, fmt.Errorf("unable to create oauth config: %s", err)
		}
		spt, err := adal.NewServicePrincipalToken(*oauthConfig, auth.ClientID, auth.ClientSecret, cloudEnv.ResourceManagerEndpoint)
		if err != nil {
			return nil, fmt.Errorf("unable to create service principal token: %s", err)
		}
		authorizer = autorest.NewBearerAuthorizer(spt)
	}
	fmt.Println(authorizer)
	containerGroupsClient := containerinstance.NewContainerGroupsClient(auth.SubscriptionID)
	containerGroupsClient.Authorizer = authorizer
	containerGroupsClient.AddToUserAgent(strings.Join(userAgent, " "))

	containersClient := containerinstance.NewContainersClient(auth.SubscriptionID)
	containersClient.Authorizer = authorizer
	containersClient.AddToUserAgent(strings.Join(userAgent, " "))

	metricsClient := insights.NewMetricsClient(auth.SubscriptionID)
	metricsClient.Authorizer = authorizer
	metricsClient.AddToUserAgent(strings.Join(userAgent, " "))

	return &Client{hc: client.HTTPClient,
		auth:                  auth,
		containerGroupsClient: containerGroupsClient,
		containersClient:      containersClient,
		metricsClient:         metricsClient}, nil
}
