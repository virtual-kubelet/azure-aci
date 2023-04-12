/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	_ "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

type CloudEnvironmentName string

const (
	AzurePublicCloud       CloudEnvironmentName = "AzurePublicCloud"
	AzureUSGovernmentCloud CloudEnvironmentName = "AzureUSGovernmentCloud"
	AzureChinaCloud        CloudEnvironmentName = "AzureChinaCloud"
)

type ConfigInterface interface {
	GetMSICredential(ctx context.Context) (*azidentity.ManagedIdentityCredential, error)
	GetSPCredential(ctx context.Context) (*azidentity.ClientSecretCredential, error)
	GetAuthorizer(ctx context.Context, resource string) (autorest.Authorizer, error)
}

type Config struct {
	AKSCredential *aksCredential
	AuthConfig    *Authentication
	Cloud         cloud.Configuration
	Authorizer    autorest.Authorizer
}

// GetMSICredential retrieve MSI credential
func (c *Config) GetMSICredential(ctx context.Context) (*azidentity.ManagedIdentityCredential, error) {
	log.G(ctx).Debug("getting token using user identity")
	opts := &azidentity.ManagedIdentityCredentialOptions{
		ID: azidentity.ClientID(c.AuthConfig.UserIdentityClientId),
		ClientOptions: azcore.ClientOptions{
			Cloud: c.Cloud,
		}}
	msiCredential, err := azidentity.NewManagedIdentityCredential(opts)
	if err != nil {
		return nil, err
	}

	return msiCredential, nil
}

// GetSPCredential retrieve SP credential
func (c *Config) GetSPCredential(ctx context.Context) (*azidentity.ClientSecretCredential, error) {
	log.G(ctx).Debug("getting token using service principal")
	opts := &azidentity.ClientSecretCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: c.Cloud,
		},
	}
	spCredential, err := azidentity.NewClientSecretCredential(c.AuthConfig.TenantID, c.AuthConfig.ClientID, c.AuthConfig.ClientSecret, opts)
	if err != nil {
		return nil, err
	}

	return spCredential, nil
}

// GetAuthorizer return autorest authorizer.
func (c *Config) GetAuthorizer(ctx context.Context, resource string) (autorest.Authorizer, error) {
	var auth autorest.Authorizer
	var err error

	var token *adal.ServicePrincipalToken
	isUserIdentity := len(c.AuthConfig.ClientID) == 0

	if isUserIdentity {
		log.G(ctx).Debug("getting token using user identity")

		token, err = adal.NewServicePrincipalTokenFromManagedIdentity(
			resource, &adal.ManagedIdentityOptions{ClientID: c.AuthConfig.UserIdentityClientId})
		if err != nil {
			return nil, err
		}
	} else {
		log.G(ctx).Debug("getting token using service principal")

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
func (c *Config) SetAuthConfig(ctx context.Context, configInterface ConfigInterface) error {
	ctx, span := trace.StartSpan(ctx, "auth.SetAuthConfig")
	defer span.End()

	var err error
	c.AuthConfig = &Authentication{}
	c.Cloud = cloud.AzurePublic

	if authFilepath := os.Getenv("AZURE_AUTH_LOCATION"); authFilepath != "" {
		log.G(ctx).Debug("getting Azure auth config from file, path: %s", authFilepath)
		auth := &Authentication{}
		err = auth.newAuthenticationFromFile(authFilepath)
		if err != nil {
			return errors.Wrap(err, "cannot get Azure auth config. Please make sure AZURE_AUTH_LOCATION env variable is set correctly")
		}
		c.AuthConfig = auth
	}

	if aksCredFilepath := os.Getenv("AKS_CREDENTIAL_LOCATION"); aksCredFilepath != "" {
		log.G(ctx).Debug("getting AKS cred from file, path: %s", aksCredFilepath)
		c.AKSCredential, err = newAKSCredential(ctx, aksCredFilepath)
		if err != nil {
			return errors.Wrap(err, "cannot get AKS credential config. Please make sure AKS_CREDENTIAL_LOCATION env variable is set correctly")
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
	}

	if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
		log.G(ctx).Debug("azure client ID env variable AZURE_CLIENT_ID is set")
		c.AuthConfig.ClientID = clientID
	}

	if clientSecret := os.Getenv("AZURE_CLIENT_SECRET"); clientSecret != "" {
		log.G(ctx).Debug("azure client secret env variable AZURE_CLIENT_SECRET is set")
		c.AuthConfig.ClientSecret = clientSecret
	}

	if userIdentityClientId := os.Getenv("VIRTUALNODE_USER_IDENTITY_CLIENTID"); userIdentityClientId != "" {
		log.G(ctx).Debug("user identity client ID env variable VIRTUALNODE_USER_IDENTITY_CLIENTID is set")
		c.AuthConfig.UserIdentityClientId = userIdentityClientId
	}

	isUserIdentity := len(c.AuthConfig.ClientID) == 0

	if isUserIdentity {
		if len(c.AuthConfig.UserIdentityClientId) == 0 {
			return fmt.Errorf("neither AZURE_CLIENT_ID or VIRTUALNODE_USER_IDENTITY_CLIENTID is being set")
		}

		log.G(ctx).Info("using user identity for Authentication")
	}

	if tenantID := os.Getenv("AZURE_TENANT_ID"); tenantID != "" {
		log.G(ctx).Debug("azure tenant ID env variable AZURE_TENANT_ID is set")
		c.AuthConfig.TenantID = tenantID
	}

	if subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subscriptionID != "" {
		log.G(ctx).Debug("azure subscription ID env variable AZURE_SUBSCRIPTION_ID is set")
		c.AuthConfig.SubscriptionID = subscriptionID
	}

	resource := c.Cloud.Services[cloud.ResourceManager].Endpoint

	c.Authorizer, err = configInterface.GetAuthorizer(ctx, resource)
	if err != nil {
		return err
	}

	return nil
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
