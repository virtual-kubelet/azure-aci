package azure

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/virtual-kubelet/virtual-kubelet/log"
)

const (
	// DefaultRetryIntervalMin - the default minimum retry wait interval
	DefaultRetryIntervalMin = 1 * time.Second
	// DefaultRetryIntervalMax - the default maximum retry wait interval
	DefaultRetryIntervalMax = 60 * time.Second
	// DefaultRetryMax - defalut retry max count
	DefaultRetryMax = 40
)

// HTTPRetryConfig - retry config for http reqeusts
type HTTPRetryConfig struct {
	RetryWaitMin time.Duration
	RetryWaitMax time.Duration
	RetryMax     int
}

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
	userAgent   []string
	base        http.RoundTripper
	client      *Client
	retryConfig HTTPRetryConfig
}

var (
	concurrentConnections = 200
)

var (
	// A regular expression to match the error returned by net/http when the
	// configured number of redirects is exhausted. This error isn't typed
	// specifically so we resort to matching on the error string.
	redirectsErrorRe = regexp.MustCompile(`stopped after \d+ redirects\z`)

	// A regular expression to match the error returned by net/http when the
	// scheme specified in the URL is invalid. This error isn't typed
	// specifically so we resort to matching on the error string.
	schemeErrorRe = regexp.MustCompile(`unsupported protocol scheme`)
)

var (
	// StatusCodesForRetry are a defined group of status code for which the client will retry
	// refer to https://docs.microsoft.com/en-us/azure/architecture/best-practices/retry-service-specific#general-rest-and-retry-guidelines
	StatusCodesForRetry = []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
)

// NewClient creates a new Azure API client from an Authentication struct and BaseURI.
func NewClient(auth *Authentication, userAgent []string, retryConfig HTTPRetryConfig) (*Client, error) {
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
		userAgent:   nonEmptyUserAgent,
		client:      client,
		retryConfig: retryConfig,
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
	for retries = 0; retries < t.retryConfig.RetryMax; retries++ {
		response, err := t.base.RoundTrip(&newReq)

		shouldRetry, rerr := retryPolicy(response, err)
		if !shouldRetry {
			return response, err
		}
		wait := backoff(t.retryConfig.RetryWaitMin, t.retryConfig.RetryWaitMax, retries, response)

		l := log.G(context.TODO()).WithField("URL", newReq.URL)
		l.Infof("would retry on error %s, waiting for %s", rerr, wait)

		time.Sleep(wait)
	}

	return t.base.RoundTrip(&newReq)
}

func retryPolicy(resp *http.Response, err error) (bool, error) {
	if err != nil {
		if v, ok := err.(*url.Error); ok {
			// Don't retry if the error was due to too many redirects.
			if redirectsErrorRe.MatchString(v.Error()) {
				return false, v
			}

			// Don't retry if the error was due to an invalid protocol scheme.
			if schemeErrorRe.MatchString(v.Error()) {
				return false, v
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
				return false, v
			}
		}

		// The error is likely recoverable so retry. this includes
		// conection closed, connection failure, timeout, request canceled, etc.
		return true, err
	}

	for _, retriableCode := range StatusCodesForRetry {
		if resp.StatusCode == retriableCode {
			return true, fmt.Errorf("unexpected HTTP result, StatusCode %d, status %s", resp.StatusCode, resp.Status)
		}
	}

	return false, nil
}

func backoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	if resp != nil {
		if resp.StatusCode == http.StatusTooManyRequests {
			if s, ok := resp.Header["Retry-After"]; ok {
				if sleep, err := strconv.ParseInt(s[0], 10, 64); err == nil {
					return time.Second * time.Duration(sleep)
				}
			}
		}
	}

	mult := math.Pow(2, float64(attemptNum)) * float64(min)
	sleep := time.Duration(mult)
	if float64(sleep) != mult || sleep > max {
		sleep = max
	}
	return sleep
}
