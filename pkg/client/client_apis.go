package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/validation"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"k8s.io/client-go/util/retry"
)

type AzClientsInterface interface {
	ContainerGroupGetter
	CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error
	GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error)
	GetContainerGroupListResult(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error)
	ListCapabilities(ctx context.Context, region string) (*[]azaci.Capabilities, error)
	DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error
	ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) (*string, error)
	ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaci.ContainerExecRequest) (*azaci.ContainerExecResponse, error)
}

type AzClientsAPIs struct {
	ContainersClient     azaci.ContainersClient
	ContainerGroupClient ContainerGroupsClientWrapper
	LocationClient       azaci.LocationClient
}

func NewAzClientsAPIs(ctx context.Context, azConfig auth.Config) *AzClientsAPIs {
	obj := AzClientsAPIs{}

	cClient := azaci.NewContainersClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)
	cClient.Authorizer = azConfig.Authorizer
	//needed for metrics
	cClient.Client.Authorizer = azConfig.Authorizer
	obj.ContainersClient = cClient

	cgClient := ContainerGroupsClientWrapper{CGClient: azaci.NewContainerGroupsClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)}
	cgClient.CGClient.Authorizer = azConfig.Authorizer
	obj.ContainerGroupClient = cgClient

	lClient := azaci.NewLocationClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)
	lClient.Authorizer = azConfig.Authorizer
	//needed for metadata
	lClient.Client.Authorizer = azConfig.Authorizer
	obj.LocationClient = lClient

	obj.setUserAgent(ctx)

	return &obj
}

func (a *AzClientsAPIs) setUserAgent(ctx context.Context) {
	ua := os.Getenv("ACI_EXTRA_USER_AGENT")
	if ua != "" {
		err := a.ContainersClient.AddToUserAgent(ua)
		if err != nil {
			log.G(ctx).Warnf("an error has occurred while setting user agent to ContainersClient", err)
			return
		}
		err = a.ContainerGroupClient.CGClient.AddToUserAgent(ua)
		if err != nil {
			log.G(ctx).Warnf("an error has occurred while setting user agent to ContainerGroupClient", err)
			return
		}
		err = a.LocationClient.AddToUserAgent(ua)
		if err != nil {
			log.G(ctx).Warnf("an error has occurred while setting user agent to LocationClient", err)
			return
		}
	}
}

func (a *AzClientsAPIs) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*ContainerGroupWrapper, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroup")
	defer span.End()

	getPreparer, err := a.ContainerGroupClient.CGClient.GetPreparer(ctx, resourceGroup, containerGroupName)
	if err != nil {
		return nil, err
	}
	result, err := a.ContainerGroupClient.CGClient.GetSender(getPreparer)
	if err != nil {
		return nil, err
	}

	if result.Body == nil {
		return nil, errors.New("get container group returned an empty body in the response")
	}

	logger.Infof("GetContainerGroup status code: %d", result.StatusCode)
	if result.StatusCode != http.StatusOK {
		return nil, errors.Errorf("get container group failed with status code %d", result.StatusCode)
	}
	var cgw ContainerGroupWrapper
	if err := json.NewDecoder(result.Body).Decode(&cgw); err != nil {
		return nil, fmt.Errorf("decoding get container group response body failed: %v", err)
	}
	return &cgw, nil
}

func (a *AzClientsAPIs) CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error {
	logger := log.G(ctx).WithField("method", "CreateContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.CreateContainerGroup")
	defer span.End()

	cgName := containerGroupName(podNS, podName)
	cg.Name = &cgName
	logger.Infof("creating container group with name: %s", *cg.Name)
	err := a.ContainerGroupClient.CreateCG(ctx, resourceGroup, *cg)
	if err != nil {
		return err
	}

	return nil
}

// GetContainerGroupInfo returns a container group from ACI.
func (a *AzClientsAPIs) GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroupInfo")
	defer span.End()

	cgName := containerGroupName(namespace, name)

	cg, err := a.ContainerGroupClient.CGClient.Get(ctx, resourceGroup, cgName)
	if err != nil {
		if cg.StatusCode == http.StatusNotFound {
			return nil, errdefs.NotFound(fmt.Sprintf("container group %s is not found", name))
		}
		return nil, err
	}

	err = validation.ValidateContainerGroup(&cg)
	if err != nil {
		return nil, err
	}
	if *cg.Tags["NodeName"] != nodeName {
		return nil, errors.Wrapf(err, "container group %s found with mismatching node", name)
	}

	return &cg, nil
}

func (a *AzClientsAPIs) GetContainerGroupListResult(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroupListResult")
	defer span.End()

	cgs, err := a.ContainerGroupClient.CGClient.ListByResourceGroup(ctx, resourceGroup)
	if err != nil {
		return nil, err
	}

	list := cgs.Values()
	return &list, nil
}

func (a *AzClientsAPIs) ListCapabilities(ctx context.Context, region string) (*[]azaci.Capabilities, error) {
	logger := log.G(ctx).WithField("method", "ListCapabilities")
	ctx, span := trace.StartSpan(ctx, "aci.ListCapabilities")
	defer span.End()

	capabilities, err := a.LocationClient.ListCapabilitiesComplete(ctx, region)

	if err != nil {
		return nil, errors.Wrapf(err, "Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
	}

	logger.Infof("ListCapabilitiesComplete status code: %d", capabilities.Response().StatusCode)
	if capabilities.Response().StatusCode != http.StatusOK {
		logger.Warn("Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
		return nil, nil
	}

	result := capabilities.Response().Value
	if result == nil {
		logger.Warn("ACI GPU capacity is not enabled. GPU capacity will be disabled")
		return nil, nil
	}
	return result, nil
}

func (a *AzClientsAPIs) DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error {
	logger := log.G(ctx).WithField("method", "DeleteContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.DeleteContainerGroup")
	defer span.End()

	deleteFuture, err := a.ContainerGroupClient.CGClient.Delete(ctx, resourceGroup, cgName)
	if err != nil {
		logger.Errorf("failed to delete container group %v", cgName)
		return err
	}
	err = deleteFuture.WaitForCompletionRef(ctx, a.ContainerGroupClient.CGClient.Client)
	if err != nil {
		return err
	}

	logger.Infof("container group %s has deleted successfully", cgName)
	return nil
}

func (a *AzClientsAPIs) ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) (*string, error) {
	logger := log.G(ctx).WithField("method", "ListLogs")
	ctx, span := trace.StartSpan(ctx, "aci.ListLogs")
	defer span.End()

	enableTimestamp := true

	// tail should be > 0, otherwise, set to nil
	var logTail *int32
	tail := int32(opts.Tail)
	if opts.Tail == 0 {
		logTail = nil
	} else {
		logTail = &tail
	}
	var err error
	var result azaci.Logs
	err = retry.OnError(retry.DefaultBackoff,
		func(err error) bool {
			return ctx.Err() == nil
		}, func() error {
			result, err = a.ContainersClient.ListLogs(ctx, resourceGroup, cgName, containerName, logTail, &enableTimestamp)
			if err != nil {
				logger.Debug("error getting container logs, name: %s , container group:  %s, retrying", containerName, cgName)
				return err
			}
			return nil
		})
	if err != nil {
		logger.Errorf("error getting container logs, name: %s , container group:  %s", containerName, cgName)
		return nil, err
	}
	logger.Infof("ListLogs status code: %d", result.StatusCode)

	return result.Content, nil
}

func (a *AzClientsAPIs) ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaci.ContainerExecRequest) (*azaci.ContainerExecResponse, error) {
	logger := log.G(ctx).WithField("method", "ExecuteContainerCommand")
	ctx, span := trace.StartSpan(ctx, "aci.ExecuteContainerCommand")
	defer span.End()

	result, err := a.ContainersClient.ExecuteCommand(ctx, resourceGroup, cgName, containerName, containerReq)
	if err != nil {
		return nil, err
	}
	logger.Infof("ExecuteContainerCommand status code: %d", result.StatusCode)
	return &result, nil
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}
