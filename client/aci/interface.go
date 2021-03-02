package aci

//go:generate sh -c "mockgen -destination mock_$GOPACKAGE/interface.go github.com/virtual-kubelet/azure-aci/client/$GOPACKAGE Interface"

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2019-06-01/insights"
)

// Interface is the client interface for aci.
type Interface interface {
	CreateContainerGroup(ctx context.Context, resourceGroup, containerGroupName string, containerGroup containerinstance.ContainerGroup) (containerinstance.ContainerGroup, error)

	DeleteContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) error

	LaunchExec(ctx context.Context, resourceGroup, containerGroupName, containerName, command string, terminalSize TerminalSizeRequest) (containerinstance.ContainerExecResponse, error)

	GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (containerinstance.ContainerGroup, error)

	ListContainerGroups(ctx context.Context, resourceGroup string) (containerinstance.ContainerGroupListResult, error)

	GetContainerLogs(ctx context.Context, resourceGroup, containerGroupName, containerName string, tail int) (*containerinstance.Logs, error)

	GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options MetricsRequest) (insights.Response, error)

	GetResourceProviderMetadata(ctx context.Context) (*ResourceProviderMetadata, error)

	UpdateContainerGroup(ctx context.Context, resourceGroup, containerGroupName string, containerGroup containerinstance.ContainerGroup) (containerinstance.ContainerGroup, error)
}
