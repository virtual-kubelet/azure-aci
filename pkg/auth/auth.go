package auth

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"unicode/utf16"

	_ "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/dimchansky/utfbom"
	"github.com/virtual-kubelet/virtual-kubelet/log"
)

type CloudEnvironmentName string

const (
	AzurePublicCloud       CloudEnvironmentName = "AzurePublicCloud"
	AzureUSGovernmentCloud CloudEnvironmentName = "AzureUSGovernment"
	AzureChinaCloud        CloudEnvironmentName = "AzureChina"
)

type Config struct {
	AKSCredential *aksCredential
	AuthConfig    *Authentication
	Cloud         cloud.Configuration
	Authorizer    autorest.Authorizer
}

// getAuthorizer return autorest authorizer.
func (c *Config) getAuthorizer(resource string) (autorest.Authorizer, error) {
	var auth autorest.Authorizer
	var err error

	var token *adal.ServicePrincipalToken
	isUserIdentity := len(c.AuthConfig.ClientID) == 0

	if isUserIdentity {
		token, err = adal.NewServicePrincipalTokenFromManagedIdentity(
			resource, &adal.ManagedIdentityOptions{ClientID: c.AuthConfig.UserIdentityClientId})
		if err != nil {
			return nil, err
		}
	} else {
		oauthConfig, err := adal.NewOAuthConfig(
			c.Cloud.ActiveDirectoryAuthorityHost, c.AuthConfig.TenantID)
		if err != nil {
			return nil, err
		}
		token, err = adal.NewServicePrincipalToken(
			*oauthConfig, c.AuthConfig.ClientID, c.AuthConfig.ClientSecret, resource)
		if err != nil {
			return nil, err
		}
	}

	auth = autorest.NewBearerAuthorizer(token)
	return auth, err
}

// SetAuthConfig sets the configuration needed for Authentication.
func (c *Config) SetAuthConfig() error {
	var err error
	c.Cloud = cloud.AzurePublic

	if authFilepath := os.Getenv("AZURE_AUTH_LOCATION"); authFilepath != "" {
		auth := &Authentication{}
		err = auth.newAuthenticationFromFile(authFilepath)
		if err != nil {
			return err
		}
		c.AuthConfig = auth
	}

	if aksCredFilepath := os.Getenv("AKS_CREDENTIAL_LOCATION"); aksCredFilepath != "" {
		c.AKSCredential, err = newAKSCredential(aksCredFilepath)
		if err != nil {
			return err
		}

		var clientId string
		if !strings.EqualFold(c.AKSCredential.ClientID, "msi") {
			clientId = c.AKSCredential.ClientID
		}

		//Set Azure cloud environment
		c.Cloud = getCloudConfiguration(c.AKSCredential.Cloud)
		c.AuthConfig = NewAuthentication(
			clientId,
			c.AKSCredential.ClientSecret,
			c.AKSCredential.SubscriptionID,
			c.AKSCredential.TenantID,
			c.AKSCredential.UserAssignedIdentityID)

		if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
			c.AuthConfig.ClientID = clientID
		}

		if clientSecret := os.Getenv("AZURE_CLIENT_SECRET"); clientSecret != "" {
			c.AuthConfig.ClientSecret = clientSecret
		}

		if userIdentityClientId := os.Getenv("VIRTUALNODE_USER_IDENTITY_CLIENTID"); userIdentityClientId != "" {
			c.AuthConfig.UserIdentityClientId = userIdentityClientId
		}

		isUserIdentity := len(c.AuthConfig.ClientID) == 0

		if isUserIdentity {
			if len(c.AuthConfig.UserIdentityClientId) == 0 {
				return fmt.Errorf("neither AZURE_CLIENT_ID or VIRTUALNODE_USER_IDENTITY_CLIENTID is being set")
			}

			log.G(context.TODO()).Info("Using user identity for Authentication")
		}

		if tenantID := os.Getenv("AZURE_TENANT_ID"); tenantID != "" {
			c.AuthConfig.TenantID = tenantID
		}

		if subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subscriptionID != "" {
			c.AuthConfig.SubscriptionID = subscriptionID
		}
	}

	resource := c.Cloud.Services[cloud.ResourceManager].Endpoint

	c.Authorizer, err = c.getAuthorizer(resource)
	if err != nil {
		return err
	}

	return nil
}

// Authentication represents the Authentication file for Azure.
type Authentication struct {
	ClientID             string `json:"clientId,omitempty"`
	ClientSecret         string `json:"clientSecret,omitempty"`
	SubscriptionID       string `json:"subscriptionId,omitempty"`
	TenantID             string `json:"tenantId,omitempty"`
	UserIdentityClientId string `json:"userIdentityClientId,omitempty"`
}

// newAuthenticationFromFile returns an Authentication struct from file path.
func (a *Authentication) newAuthenticationFromFile(filepath string) error {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("reading Authentication file %q failed: %v", filepath, err)
	}

	// Authentication file might be encoded.
	decoded, err := a.decode(b)
	if err != nil {
		return fmt.Errorf("decoding Authentication file %q failed: %v", filepath, err)
	}

	// Unmarshal the Authentication file.
	if err := json.Unmarshal(decoded, &a); err != nil {
		return err
	}
	return nil
}

// NewAuthentication returns an Authentication struct from user provided credentials.
func NewAuthentication(clientID, clientSecret, subscriptionID, tenantID, userAssignedIdentityID string) *Authentication {

	return &Authentication{
		ClientID:             clientID,
		ClientSecret:         clientSecret,
		SubscriptionID:       subscriptionID,
		TenantID:             tenantID,
		UserIdentityClientId: userAssignedIdentityID,
	}
}

// aksCredential represents the credential file for AKS
type aksCredential struct {
	Cloud                  string `json:"cloud"`
	TenantID               string `json:"tenantId"`
	SubscriptionID         string `json:"subscriptionId"`
	ClientID               string `json:"aadClientId"`
	ClientSecret           string `json:"aadClientSecret"`
	ResourceGroup          string `json:"resourceGroup"`
	Region                 string `json:"location"`
	VNetName               string `json:"vnetName"`
	VNetResourceGroup      string `json:"vnetResourceGroup"`
	UserAssignedIdentityID string `json:"userAssignedIdentityID"`
}

// newAKSCredential returns an aksCredential struct from file path.
func newAKSCredential(filePath string) (*aksCredential, error) {
	logger := log.G(context.TODO()).WithField("method", "newAKSCredential").WithField("file", filePath)
	logger.Debug("Reading AKS credential file")

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading AKS credential file %q failed: %v", filePath, err)
	}

	// Unmarshal the Authentication file.
	var cred aksCredential
	if err := json.Unmarshal(b, &cred); err != nil {
		return nil, err
	}

	logger.Debug("load AKS credential file successfully")
	return &cred, nil
}

func (a *Authentication) decode(b []byte) ([]byte, error) {
	reader, enc := utfbom.Skip(bytes.NewReader(b))

	switch enc {
	case utfbom.UTF16LittleEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.LittleEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	case utfbom.UTF16BigEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.BigEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	}
	return ioutil.ReadAll(reader)
}

func getCloudConfiguration(cloudName string) cloud.Configuration {
	switch cloudName {
	case string(AzurePublicCloud):
		return cloud.AzurePublic
	case string(AzureUSGovernmentCloud):
		return cloud.AzureGovernment
	case string(AzureChinaCloud):
		return cloud.AzureChina
	}
	panic("cloud config does not exist")
}
