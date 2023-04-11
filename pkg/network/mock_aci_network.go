/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package network

import (
	"context"

	aznetworkv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
)

type GetSubnetClientFunc func(ctx context.Context, azConfig *auth.Config) (*aznetworkv2.SubnetsClient, error)
type GetACISubnetFunc func(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient) (aznetworkv2.Subnet, error)
type CreateACISubnetFunc func(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient) error

type MockProviderNetwork struct {
	MockGetSubnetClient GetSubnetClientFunc
	MockGetACISubnet    GetACISubnetFunc
	MockCreateACISubnet CreateACISubnetFunc
}

func NewMockACINetwork() *MockProviderNetwork {
	return &MockProviderNetwork{}
}

func (m *MockProviderNetwork) GetSubnetClient(ctx context.Context, azConfig *auth.Config) (*aznetworkv2.SubnetsClient, error) {
	if m.MockGetSubnetClient != nil {
		return m.MockGetSubnetClient(ctx, azConfig)
	}
	return nil, nil
}

func (m *MockProviderNetwork) GetACISubnet(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient) (aznetworkv2.Subnet, error) {
	if m.MockGetACISubnet != nil {
		return m.MockGetACISubnet(ctx, subnetsClient)
	}
	return aznetworkv2.Subnet{}, nil
}

func (m *MockProviderNetwork) CreateACISubnet(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient) error {
	if m.MockCreateACISubnet != nil {
		return m.MockCreateACISubnet(ctx, subnetsClient)
	}
	return nil
}
