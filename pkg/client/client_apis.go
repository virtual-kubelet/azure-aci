package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

type AzClientsInterface interface {
	MetricsGetter
	ContainerGroupGetter
	CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error
	GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error)
	GetContainerGroupListResult(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error)
	ListCapabilities(ctx context.Context, region string) (*[]azaci.Capabilities, error)
	DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error
	ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) *string
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
			log.G(ctx).Warnf("an error has been occurred while setting user agent to ContainersClient", err)
			return
		}
		err = a.ContainerGroupClient.CGClient.AddToUserAgent(ua)
		if err != nil {
			log.G(ctx).Warnf("an error has been occurred while setting user agent to ContainerGroupClient", err)
			return
		}
		err = a.LocationClient.AddToUserAgent(ua)
		if err != nil {
			log.G(ctx).Warnf("an error has been occurred while setting user agent to LocationClient", err)
			return
		}
	}
}

func (a *AzClientsAPIs) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*ContainerGroupWrapper, error) {
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
	ctx, span := trace.StartSpan(ctx, "aci.CreateCG")
	defer span.End()

	cgName := containerGroupName(podNS, podName)
	err := a.ContainerGroupClient.CreateCG(ctx, resourceGroup, cgName, *cg)

	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to create container group %v", cgName)
	}

	return err
}

// GetContainerGroupInfo returns a container group from ACI.
func (a *AzClientsAPIs) GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
	cgName := containerGroupName(namespace, name)

	cg, err := a.ContainerGroupClient.CGClient.Get(ctx, resourceGroup, cgName)
	if err != nil {
		if cg.StatusCode == http.StatusNotFound {
			return nil, errors.Wrapf(err, "container group %s is not found", name)
		}
		return nil, err
	}

	if *cg.Tags["NodeName"] != nodeName {
		return nil, errors.Wrapf(err, "container group %s found with mismatching node", name)
	}

	return &cg, nil
}
func (a *AzClientsAPIs) GetContainerGroupListResult(ctx context.Context, resourceGroup string) (*[]azaci.ContainerGroup, error) {
	cgs, err := a.ContainerGroupClient.CGClient.ListByResourceGroup(ctx, resourceGroup)
	list := cgs.Values()
	return &list, err
}

func (a *AzClientsAPIs) ListCapabilities(ctx context.Context, region string) (*[]azaci.Capabilities, error) {
	logger := log.G(ctx).WithField("method", "ListCapabilities")

	capabilities, err := a.LocationClient.ListCapabilitiesComplete(ctx, region)

	if err != nil {
		return nil, errors.Wrapf(err, "Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
	}

	if capabilities.Response().StatusCode != http.StatusOK {
		logger.Warn("Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
		return nil, nil
	}

	result := capabilities.Response().Value
	if result == nil {
		logger.Warn("ACI GPU capacity is not enabled. GPU capacity will be disabled")
	}
	return result, nil
}

func (a *AzClientsAPIs) GetContainerGroupMetrics(ctx context.Context, resourceGroup, cgName string, requestOptions MetricsRequestOptions) (*ContainerGroupMetricsResult, error) {
	metrics, err := a.ContainerGroupClient.GetMetrics(ctx, resourceGroup, cgName, requestOptions)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

func (a *AzClientsAPIs) DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error {
	deleteFuture, err := a.ContainerGroupClient.CGClient.Delete(ctx, resourceGroup, cgName)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to delete container group %v", cgName)
		return err
	}
	err = deleteFuture.WaitForCompletionRef(ctx, a.ContainerGroupClient.CGClient.Client)
	if err != nil {
		return err
	}
	return nil
}

func (a *AzClientsAPIs) ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) *string {
	enableTimestamp := true
	logTail := int32(opts.Tail)
	retry := 10
	logContent := ""
	var retries int
	for retries = 0; retries < retry; retries++ {
		cLogs, err := a.ContainersClient.ListLogs(ctx, resourceGroup, cgName, containerName, &logTail, &enableTimestamp)
		if err != nil {
			log.G(ctx).WithField("method", "GetContainerLogs").WithError(err).Debug("Error getting container logs, retrying")
			time.Sleep(5000 * time.Millisecond)
		} else {
			logContent = *cLogs.Content
			break
		}
	}

	return &logContent
}

func (a *AzClientsAPIs) ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaci.ContainerExecRequest) (*azaci.ContainerExecResponse, error) {
	result, err := a.ContainersClient.ExecuteCommand(ctx, resourceGroup, cgName, containerName, containerReq)
	return &result, err
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}
