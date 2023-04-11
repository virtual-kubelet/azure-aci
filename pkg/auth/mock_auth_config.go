/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package auth

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
)

type GetMSICredentialFunc func(ctx context.Context) (*azidentity.ManagedIdentityCredential, error)
type GetSPCredentialFunc func(ctx context.Context) (*azidentity.ClientSecretCredential, error)
type GetAuthorizerFunc func(ctx context.Context, resource string) (autorest.Authorizer, error)

type MockConfig struct {
	MockGetMSICredential GetMSICredentialFunc
	MockGetSPCredential  GetSPCredentialFunc
	MockGetAuthorizer    GetAuthorizerFunc
}

func (m *MockConfig) GetMSICredential(ctx context.Context) (*azidentity.ManagedIdentityCredential, error) {
	if m.MockGetMSICredential != nil {
		return m.MockGetMSICredential(ctx)
	}
	return nil, nil
}

func (m *MockConfig) GetSPCredential(ctx context.Context) (*azidentity.ClientSecretCredential, error) {
	if m.MockGetSPCredential != nil {
		return m.MockGetSPCredential(ctx)
	}
	return nil, nil
}

func (m *MockConfig) GetAuthorizer(ctx context.Context, resource string) (autorest.Authorizer, error) {
	if m.MockGetAuthorizer != nil {
		return m.MockGetAuthorizer(ctx, resource)
	}
	return nil, nil
}
