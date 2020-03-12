package azure

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/go-autorest/autorest/adal"
)

// Client represents authentication details and cloud specific parameters for
// Azure Resource Manager clients.
type Client struct {
	Authentication   *Authentication
	BaseURI          string
	HTTPClient       *http.Client
	BearerAuthorizer *BearerAuthorizer
	spToken          *adal.ServicePrincipalToken
}

// BearerAuthorizer implements the bearer authorization.
type BearerAuthorizer struct {
	tokenProvider adal.OAuthTokenProvider
}

type userAgentTransport struct {
	userAgent []string
	base      http.RoundTripper
	client    *Client
}

var (
	concurrentConnections          = 200
	throttlingAdditionalRetryCount = 3
)

// NewClient creates a new Azure API client from an Authentication struct and BaseURI.
func NewClient(auth *Authentication, userAgent []string) (*Client, error) {
	client := &Client{
		Authentication: auth,
		BaseURI:        auth.ResourceManagerEndpoint,
	}

	if !auth.UseUserIdentity {
		config, err := adal.NewOAuthConfig(auth.ActiveDirectoryEndpoint, auth.TenantID)
		if err != nil {
			return nil, fmt.Errorf("Creating new OAuth config for active directory failed: %v", err)
		}

		client.spToken, err = adal.NewServicePrincipalToken(*config, auth.ClientID, auth.ClientSecret, auth.ResourceManagerEndpoint)
		if err != nil {
			return nil, fmt.Errorf("Creating new service principal token failed: %v", err)
		}
	} else {
		endpoint, err := adal.GetMSIVMEndpoint()
		if err != nil {
			return nil, fmt.Errorf("Unable to retrieve managed identity endpoint: %v", err)
		}

		client.spToken, err = adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(
			endpoint,
			auth.ManagementEndpoint,
			auth.UserIdentityClientId)
		if err != nil {
			return nil, fmt.Errorf("Unable to create token provider with managed identity: %v", err)
		}
	}

	client.BearerAuthorizer = &BearerAuthorizer{tokenProvider: client.spToken}

	nonEmptyUserAgent := userAgent[:0]
	for _, ua := range userAgent {
		if ua != "" {
			nonEmptyUserAgent = append(nonEmptyUserAgent, ua)
		}
	}

	// As go transport doesn't support a away to force close (not reuse) a specific connection in a selective way
	// after rountrip completes, we'll disable keepalives.
	uat := userAgentTransport{
		base: &http.Transport{
			DisableKeepAlives:   true,
			MaxIdleConnsPerHost: concurrentConnections,
		},
		userAgent: nonEmptyUserAgent,
		client:    client,
	}

	client.HTTPClient = &http.Client{
		Transport: uat,
	}

	return client, nil
}

func (c *Client) SetTokenProviderTestSender(s adal.Sender) {
	c.spToken.SetSender(s)
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		return nil, errors.New("RoundTrip: no Transport specified")
	}

	newReq := *req
	newReq.Header = make(http.Header)
	for k, vv := range req.Header {
		newReq.Header[k] = vv
	}

	// Add the user agent header.
	newReq.Header["User-Agent"] = []string{strings.Join(t.userAgent, " ")}

	// Add the content-type header.
	newReq.Header["Content-Type"] = []string{"application/json"}

	// Refresh the token if necessary
	// TODO: don't refresh the token everytime
	refresher, ok := t.client.BearerAuthorizer.tokenProvider.(adal.Refresher)
	if ok {
		if err := refresher.EnsureFresh(); err != nil {
			return nil, fmt.Errorf("Failed to refresh the authorization token for request to %s: %v", newReq.URL, err)
		}
	}

	// Add the authorization header.
	newReq.Header["Authorization"] = []string{fmt.Sprintf("Bearer %s", t.client.BearerAuthorizer.tokenProvider.OAuthToken())}

	var retries int
	for retries = 0; retries < throttlingAdditionalRetryCount; retries++ {
		response, err := t.base.RoundTrip(&newReq)
		if err == nil && response.StatusCode == 429 {
			// We hit throttling, retry to hopefully hit another ARM instance.
			continue
		}

		return response, err
	}

	return t.base.RoundTrip(&newReq)
}
