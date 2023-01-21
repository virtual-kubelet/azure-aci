package client

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
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
	GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error)
	GetContainerGroupListResult(ctx context.Context, resourceGroup string) ([]*azaciv2.ContainerGroup, error)
	ListCapabilities(ctx context.Context, region string) ([]*azaciv2.Capabilities, error)
	DeleteContainerGroup(ctx context.Context, resourceGroup, cgName string) error
	ListLogs(ctx context.Context, resourceGroup, cgName, containerName string, opts api.ContainerLogOpts) (*string, error)
	ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaciv2.ContainerExecRequest) (*azaciv2.ContainerExecResponse, error)
}

type AzClientsAPIs struct {
	ContainersClient     *azaciv2.ContainersClient
	ContainerGroupClient *azaciv2.ContainerGroupsClient
	LocationClient       *azaciv2.LocationClient
}

func NewAzClientsAPIs(ctx context.Context, azConfig auth.Config) (*AzClientsAPIs, error) {
	logger := log.G(ctx).WithField("method", "NewAzClientsAPIs")
	ctx, span := trace.StartSpan(ctx, "client.NewAzClientsAPIs")
	defer span.End()

	obj := AzClientsAPIs{}

	logger.Debug("getting azure credential")

	var err error
	var credential azcore.TokenCredential
	isUserIdentity := len(azConfig.AuthConfig.ClientID) == 0

	if isUserIdentity {
		credential, err = azConfig.GetMSICredential(ctx)
	} else {
		credential, err = azConfig.GetSPCredential(ctx)
	}
	if err != nil {
		return nil, errors.Wrap(err, "an error has occurred while creating getting credential ")
	}

	logger.Debug("setting aci user agent")
	userAgent := os.Getenv("ACI_EXTRA_USER_AGENT")
	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: azConfig.Cloud,
			Telemetry: policy.TelemetryOptions{
				ApplicationID: userAgent,
			},
		},
	}

	logger.Debug("initializing aci clients")
	cClient, err := azaciv2.NewContainersClient(azConfig.AuthConfig.SubscriptionID, credential, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create container client ")
	}

	cgClient, err := azaciv2.NewContainerGroupsClient(azConfig.AuthConfig.SubscriptionID, credential, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create container group client ")
	}

	lClient, err := azaciv2.NewLocationClient(azConfig.AuthConfig.SubscriptionID, credential, &options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create location client ")
	}

	obj.ContainersClient = cClient
	obj.ContainerGroupClient = cgClient
	obj.LocationClient = lClient

	logger.Debug("aci clients have been initialized successfully")
	return &obj, nil
}

func (a *AzClientsAPIs) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*azaciv2.ContainerGroup, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroup")
	ctx, span := trace.StartSpan(ctx, "client.GetContainerGroup")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	result, err := a.ContainerGroupClient.Get(ctxWithResp, resourceGroup, containerGroupName, nil)
	if err != nil {
		if rawResponse.StatusCode == http.StatusNotFound {
			logger.Errorf("failed to query Container Group %s, not found", containerGroupName)
			return nil, err
		}
		return nil, err
	}

	return &result.ContainerGroup, nil
}

func (a *AzClientsAPIs) CreateContainerGroup(ctx context.Context, resourceGroup, podNS, podName string, cg *ContainerGroupWrapper) error {
	logger := log.G(ctx).WithField("method", "CreateContainerGroup")
	ctx, span := trace.StartSpan(ctx, "client.CreateContainerGroup")
	defer span.End()
	cgName := containerGroupName(podNS, podName)

	containerGroup := azaciv2.ContainerGroup{
		Properties: cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Properties,
		Name:       &cgName,
		Type:       cg.Type,
		Identity:   cg.Identity,
		Location:   cg.Location,
		Tags:       cg.Tags,
		ID:         cg.ID,
	}

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	logger.Infof("creating container group with name: %s", cgName)
	_, err := a.ContainerGroupClient.BeginCreateOrUpdate(ctxWithResp, resourceGroup, cgName, containerGroup, nil)
	if err != nil {
		logger.Errorf("an error has occurred while creating container group %s, status code %d", cgName, rawResponse.StatusCode)
		return err
	}

	return nil
}

// GetContainerGroupInfo returns a container group from ACI.
func (a *AzClientsAPIs) GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaciv2.ContainerGroup, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroupInfo")
	ctx, span := trace.StartSpan(ctx, "client.GetContainerGroupInfo")
	defer span.End()
	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	cgName := containerGroupName(namespace, name)

	var err error
	var response azaciv2.ContainerGroupsClientGetResponse
	retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return true
	}, func() error {
		response, err = a.ContainerGroupClient.Get(ctxWithResp, resourceGroup, cgName, nil)
		return err
	})
	if err != nil {
		logger.Errorf("an error has occurred while getting container group info %s, status code %d", cgName, rawResponse.StatusCode)
		return nil, err
	}

	retry.OnError(retry.DefaultBackoff,
		func(err error) bool {
			return true
		}, func() error {
			err = validation.ValidateContainerGroup(ctx, &response.ContainerGroup)
			logger.Debugf("container group %s has missing fields. retrying the validation...", cgName)
			return err
		})
	if err != nil {
		return nil, err
	}
	if nodeName != "" && *response.Tags["NodeName"] != nodeName {
		return nil, errors.Wrapf(err, "container group %s found with mismatching node", cgName)
	}

	return &response.ContainerGroup, nil
}

func (a *AzClientsAPIs) GetContainerGroupListResult(ctx context.Context, resourceGroup string) ([]*azaciv2.ContainerGroup, error) {
	logger := log.G(ctx).WithField("method", "GetContainerGroupListResult")
	ctx, span := trace.StartSpan(ctx, "client.GetContainerGroupListResult")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	pager := a.ContainerGroupClient.NewListByResourceGroupPager(resourceGroup, nil)

	var cgList []*azaciv2.ContainerGroup
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

func (a *AzClientsAPIs) ListCapabilities(ctx context.Context, region string) ([]*azaciv2.Capabilities, error) {
	logger := log.G(ctx).WithField("method", "ListCapabilities")
	ctx, span := trace.StartSpan(ctx, "client.ListCapabilities")
	defer span.End()

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	pager := a.LocationClient.NewListCapabilitiesPager(region, nil)

	var capList []*azaciv2.Capabilities
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
	ctx, span := trace.StartSpan(ctx, "client.DeleteContainerGroup")
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
	ctx, span := trace.StartSpan(ctx, "client.ListLogs")
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

	options := azaciv2.ContainersClientListLogsOptions{
		Tail:       logTail,
		Timestamps: &enableTimestamp,
	}
	var err error
	var result azaciv2.Logs
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

func (a *AzClientsAPIs) ExecuteContainerCommand(ctx context.Context, resourceGroup, cgName, containerName string, containerReq azaciv2.ContainerExecRequest) (*azaciv2.ContainerExecResponse, error) {
	logger := log.G(ctx).WithField("method", "ExecuteContainerCommand")
	ctx, span := trace.StartSpan(ctx, "client.ExecuteContainerCommand")
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
