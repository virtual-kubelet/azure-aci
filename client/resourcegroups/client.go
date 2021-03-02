package resourcegroups

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/golang/glog"
	azureclient "github.com/virtual-kubelet/azure-aci/client"
)

const (
	defaultUserAgent = "virtual-kubelet/azure-arm-resourcegroups/2017-12-01"
	apiVersion       = "2017-08-01"

	resourceGroupURLPath = "subscriptions/{{.subscriptionId}}/resourcegroups/{{.resourceGroupName}}"
)

// Client is a client for interacting with Azure resource groups.
//
// Clients should be reused instead of created as needed.
// The methods of Client are safe for concurrent use by multiple goroutines.
type Client struct {
	hc           *http.Client
	auth         *azureclient.Authentication
	groupsClient resources.GroupsClient
}

// NewClient creates a new Azure resource groups client.
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

	var authorizer autorest.Authorizer
	cloudEnv, err := azure.EnvironmentFromName(auth.AzureCloud)
	if err != nil {
		return nil, fmt.Errorf("unable to get cloudEnv: %s", err)
	}
	if auth.UseUserIdentity {
		glog.Infof("using MSI")
		msiEP, err := adal.GetMSIVMEndpoint()
		if err != nil {
			return nil, fmt.Errorf("unable to get MSI endpoint: %s", err)
		}

		spt, err := adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(
			msiEP, cloudEnv.ResourceManagerEndpoint, auth.UserIdentityClientId)
		if err != nil {
			return nil, fmt.Errorf("unable to create MSI authorizer: %s", err)
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

	groupsClient := resources.NewGroupsClientWithBaseURI(auth.ResourceManagerEndpoint, auth.SubscriptionID)
	groupsClient.Authorizer = authorizer
	if extraUserAgent != "" {
		if err := groupsClient.AddToUserAgent(extraUserAgent); err != nil {
			return nil, fmt.Errorf("unable to add user agent: %s", err)
		}
	}

	return &Client{
		hc:           client.HTTPClient,
		auth:         auth,
		groupsClient: groupsClient,
	}, nil
}
