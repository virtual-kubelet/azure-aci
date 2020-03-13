package network

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	azure "github.com/virtual-kubelet/azure-aci/client"
	"github.com/virtual-kubelet/azure-aci/client/api"
)

const (
	defaultUserAgent = "virtual-kubelet/azure-arm-network/2018-08-01"
	apiVersion       = "2018-08-01"
)

// Client is a client for interacting with Azure networking
type Client struct {
	sc network.SubnetsClient
	hc *http.Client

	auth *azure.Authentication
}

// NewClient creates a new client for interacting with azure networking
func NewClient(azAuth *azure.Authentication, extraUserAgent string) (*Client, error) {
	if azAuth == nil {
		return nil, fmt.Errorf("Authentication is not supplied for the Azure client")
	}

	userAgent := []string{defaultUserAgent}
	if extraUserAgent != "" {
		userAgent = append(userAgent, extraUserAgent)
	}

	client, err := azure.NewClient(azAuth, userAgent)
	if err != nil {
		return nil, fmt.Errorf("Creating Azure client failed: %v", err)
	}

	var networkAuth autorest.Authorizer
	if !azAuth.UseUserIdentity {
		networkAuth, err = NewClientConfigByCloud(azAuth).Authorizer()
	} else {
		msiConfig := NewMSIConfigByCloud(azAuth)
		networkAuth, err = msiConfig.Authorizer()
	}

	if err != nil {
		return nil, err
	}

	sc := network.NewSubnetsClientWithBaseURI(azAuth.ResourceManagerEndpoint, azAuth.SubscriptionID)
	sc.Authorizer = networkAuth

	return &Client{
		sc:   sc,
		hc:   client.HTTPClient,
		auth: azAuth,
	}, nil
}

// IsNotFound determines if the passed in error is a not found error from the API.
func IsNotFound(err error) bool {
	switch e := err.(type) {
	case nil:
		return false
	case *api.Error:
		return e.StatusCode == http.StatusNotFound
	default:
		return false
	}
}

func NewClientConfigByCloud(azAuth *azure.Authentication) auth.ClientCredentialsConfig {
	return auth.ClientCredentialsConfig{
		ClientID:     azAuth.ClientID,
		ClientSecret: azAuth.ClientSecret,
		TenantID:     azAuth.TenantID,
		Resource:     azAuth.ResourceManagerEndpoint,
		AADEndpoint:  azAuth.ActiveDirectoryEndpoint,
	}
}

func NewMSIConfigByCloud(azAuth *azure.Authentication) auth.MSIConfig {
	return auth.MSIConfig{
		Resource: azAuth.ResourceManagerEndpoint,
		ClientID: azAuth.UserIdentityClientId,
	}
}
