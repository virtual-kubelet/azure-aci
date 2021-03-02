package aci

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
)

// TerminalSizeRequest is the terminal size request
type TerminalSizeRequest struct {
	Width  int
	Height int
}

// LaunchExec starts the exec command for a specified container instance in a specified resource group and container group.
func (c *Client) LaunchExec(ctx context.Context, resourceGroup, containerGroupName, containerName, command string, terminalSize TerminalSizeRequest) (containerinstance.ContainerExecResponse, error) {

	containerExecRequest := containerinstance.ContainerExecRequest{
		Command: &command,
		TerminalSize: &containerinstance.ContainerExecRequestTerminalSize{
			Rows: to.Int32Ptr(int32(terminalSize.Height)),
			Cols: to.Int32Ptr(int32(terminalSize.Width)),
		},
	}
	return c.containersClient.ExecuteCommand(ctx, resourceGroup, containerGroupName, containerName, containerExecRequest)
}
