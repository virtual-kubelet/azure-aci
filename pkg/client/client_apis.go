package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/validation"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"k8s.io/client-go/util/retry"
)

type AzClientsInterface interface {
	ContainerGroupGetter
	CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error
	GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error)
	GetContainerGroupListResult(ctx context.Context, resourceGroup string) ([]*azaci.ContainerGroup, error)
	ListCapabilities(ctx context.Context, region string) ([]*azaci.Capabilities, error)
	DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error
	ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) (*string, error)
	ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaci.ContainerExecRequest) (*azaci.ContainerExecResponse, error)
}

type AzClientsAPIs struct {
	ContainersClient     *azaci.ContainersClient
	ContainerGroupClient *azaci.ContainerGroupsClient
	LocationClient       *azaci.LocationClient
}

func NewAzClientsAPIs(ctx context.Context, azConfig auth.Config) (*AzClientsAPIs, error) {
	obj := AzClientsAPIs{}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, errors.Wrap(err, "an error has occurred while creating getting credential ")
	}
	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: azConfig.Cloud,
		},
	}

	cClient, err := azaci.NewContainersClient(azConfig.AuthConfig.SubscriptionID, cred, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create container client ")
	}

	cgClient, err := azaci.NewContainerGroupsClient(azConfig.AuthConfig.SubscriptionID, cred, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create container group client ")
	}

	lClient, err := azaci.NewLocationClient(azConfig.AuthConfig.SubscriptionID, cred, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create location client ")
	}

	obj.ContainersClient = cClient
	obj.ContainerGroupClient = cgClient
	obj.LocationClient = lClient

	return &obj, nil
}

func (a *AzClientsAPIs) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*azaci.ContainerGroup, error) {
	_ = log.G(ctx).WithField("method", "GetContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroup")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	result, err := a.ContainerGroupClient.Get(ctxWithResp, resourceGroup, containerGroupName, nil)
	if err != nil {
		if rawResponse.StatusCode == http.StatusNotFound {
			return nil, errors.Wrapf(err, "failed to query Container Group %s, not found it", containerGroupName)

		}
		return nil, err
	}

	return &result.ContainerGroup, nil
}

func (a *AzClientsAPIs) CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error {
	logger := log.G(ctx).WithField("method", "CreateContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.CreateContainerGroup")
	defer span.End()

	containerGroup := azaci.ContainerGroup{
		Properties: cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Properties,
		Name:       cg.Name,
		Type:       cg.Type,
		Identity:   cg.Identity,
		Location:   cg.Location,
		Tags:       cg.Tags,
		ID:         cg.ID,
	}
	cgName := containerGroupName(podNS, podName)
	cg.Name = &cgName
	var rawResponse *http.Response

	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)
	logger.Infof("creating container group with name: %s", *cg.Name)
	_, err := a.ContainerGroupClient.BeginCreateOrUpdate(ctxWithResp, resourceGroup, *cg.Name, containerGroup, nil)
	if err != nil {
		logger.Errorf("an error has occurred while creating container group %s, status code %d", cg.Name, rawResponse.StatusCode)
		return err
	}

	return nil
}

// GetContainerGroupInfo returns a container group from ACI.
func (a *AzClientsAPIs) GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroupInfo")
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroupInfo")
	defer span.End()
	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	cgName := containerGroupName(namespace, name)
	response, err := a.ContainerGroupClient.Get(ctxWithResp, resourceGroup, cgName, nil)
	if err != nil {
		logger.Errorf("an error has occurred while getting container group info %s, status code %d", cgName, rawResponse.StatusCode)

		return nil, err
	}

	err = validation.ValidateContainerGroup(response.ContainerGroup)
	if err != nil {
		return nil, err
	}
	if nodeName != "" && *response.Tags["NodeName"] != nodeName {
		return nil, errors.Wrapf(err, "container group %s found with mismatching node", cgName)
	}

	return &response.ContainerGroup, nil
}

func (a *AzClientsAPIs) GetContainerGroupListResult(ctx context.Context, resourceGroup string) ([]*azaci.ContainerGroup, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroupListResult")
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerGroupListResult")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	pager := a.ContainerGroupClient.NewListByResourceGroupPager(resourceGroup, nil)

	var cgList []*azaci.ContainerGroup
	for pager.More() {
		page, err := pager.NextPage(ctxWithResp)
		if err != nil {
			logger.Errorf("an error has occurred while getting list of container groups, status code %d", rawResponse.StatusCode)
			return nil, err
		}
		cgList = append(cgList, page.Value...)
	}
	return cgList, nil
}

func (a *AzClientsAPIs) ListCapabilities(ctx context.Context, region string) ([]*azaci.Capabilities, error) {
	logger := log.G(ctx).WithField("method", "ListCapabilities")
	ctx, span := trace.StartSpan(ctx, "aci.ListCapabilities")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	pager := a.LocationClient.NewListCapabilitiesPager(region, nil)

	var capList []*azaci.Capabilities
	for pager.More() {
		page, err := pager.NextPage(ctxWithResp)
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
		}
		capList = append(capList, page.Value...)
	}

	if capList == nil {
		logger.Warn("ACI GPU capacity is not enabled. GPU capacity will be disabled")
		return nil, nil
	}
	return capList, nil
}

func (a *AzClientsAPIs) DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error {
	logger := log.G(ctx).WithField("method", "DeleteContainerGroup")
	ctx, span := trace.StartSpan(ctx, "aci.DeleteContainerGroup")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	_, err := a.ContainerGroupClient.BeginDelete(ctxWithResp, resourceGroup, cgName, nil)
	if err != nil {
		logger.Errorf("failed to delete container group %s, status code %d", cgName, rawResponse.StatusCode)
		return err
	}

	logger.Infof("container group %s has deleted successfully", cgName)
	return nil
}

func (a *AzClientsAPIs) ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) (*string, error) {
	logger := log.G(ctx).WithField("method", "ListLogs")
	ctx, span := trace.StartSpan(ctx, "aci.ListLogs")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	enableTimestamp := true

	// tail should be > 0, otherwise, set to nil
	var logTail *int32
	tail := int32(opts.Tail)
	if opts.Tail == 0 {
		logTail = nil
	} else {
		logTail = &tail
	}

	options := azaci.ContainersClientListLogsOptions{
		Tail:       logTail,
		Timestamps: &enableTimestamp,
	}
	var err error
	var result azaci.Logs
	err = retry.OnError(retry.DefaultBackoff,
		func(err error) bool {
			return ctx.Err() == nil
		}, func() error {
			response, err := a.ContainersClient.ListLogs(ctxWithResp, resourceGroup, cgName, containerName, &options)
			if err != nil {
				logger.Debug("error getting container logs, name: %s , container group:  %s, status code %d, retrying",
					containerName, cgName, rawResponse.StatusCode)
				return err
			}
			result = response.Logs
			return nil
		})
	if err != nil {
		logger.Errorf("error getting container logs, name: %s , container group:  %s, status code %d", containerName, cgName, rawResponse.StatusCode)
		return nil, err
	}

	return result.Content, nil
}

func (a *AzClientsAPIs) ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaci.ContainerExecRequest) (*azaci.ContainerExecResponse, error) {
	logger := log.G(ctx).WithField("method", "ExecuteContainerCommand")
	ctx, span := trace.StartSpan(ctx, "aci.ExecuteContainerCommand")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	result, err := a.ContainersClient.ExecuteCommand(ctxWithResp, resourceGroup, cgName, containerName, containerReq, nil)
	if err != nil {
		logger.Errorf("an error has occurred while executing command for container group %s, status code %d", cgName, rawResponse.StatusCode)
		return nil, err
	}

	logger.Debug("ExecuteContainerCommand is successful")
	return &result.ContainerExecResponse, nil
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}
