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
	"github.com/virtual-kubelet/azure-aci/pkg"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
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

func NewAzClientsAPIs(azConfig auth.Config, retryOpt *HTTPRetryConfig) *AzClientsAPIs {
	obj := AzClientsAPIs{}

	cClient := azaci.NewContainersClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)
	cClient.Authorizer = azConfig.Authorizer
	cClient.RetryAttempts = retryOpt.RetryMax
	cClient.RetryDuration = retryOpt.RetryWaitMax - retryOpt.RetryWaitMin
	obj.ContainersClient = cClient

	cgClient := ContainerGroupsClientWrapper{CGClient: azaci.NewContainerGroupsClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)}
	cgClient.CGClient.Authorizer = azConfig.Authorizer
	cClient.RetryAttempts = retryOpt.RetryMax
	cClient.RetryDuration = retryOpt.RetryWaitMax - retryOpt.RetryWaitMin
	obj.ContainerGroupClient = cgClient

	lClient := azaci.NewLocationClientWithBaseURI(azConfig.Cloud.Services[cloud.ResourceManager].Endpoint, azConfig.AuthConfig.SubscriptionID)
	lClient.Client.Authorizer = azConfig.Authorizer
	cClient.RetryAttempts = retryOpt.RetryMax
	cClient.RetryDuration = retryOpt.RetryWaitMax - retryOpt.RetryWaitMin
	obj.LocationClient = lClient

	obj.setUserAgent()

	return &obj
}

func (a *AzClientsAPIs) setUserAgent() {
	ua := os.Getenv("ACI_EXTRA_USER_AGENT")
	if ua != "" {
		a.ContainersClient.AddToUserAgent(ua)
		a.ContainerGroupClient.CGClient.AddToUserAgent(ua)
		a.LocationClient.AddToUserAgent(ua)
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
	err := a.ContainerGroupClient.CreateCG(ctx, resourceGroup, resourceGroup, *cg)

	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to create container group %v", cgName)
	}

	return err
}

// GetContainerGroupInfo returns a container group from ACI.
func (a *AzClientsAPIs) GetContainerGroupInfo(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
	cg, err := a.ContainerGroupClient.CGClient.Get(ctx, resourceGroup, fmt.Sprintf("%s-%s", namespace, name))
	if err != nil {
		if cg.StatusCode == http.StatusNotFound {
			return nil, errdefs.NotFound("cg not found")
		}
		return nil, err
	}

	if *cg.Tags["NodeName"] != nodeName {
		return nil, errdefs.NotFound("cg found with mismatching node")
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
		logger.WithError(err).Errorf("Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", region)
		return nil, err
	}
	result := capabilities.Response().Value
	return result, err
}

func (a *AzClientsAPIs) GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options pkg.MetricsRequest) (*pkg.ContainerGroupMetricsResult, error) {
	return nil, nil

	//TODO fix and uncomment with metrics migration PR
	//if len(options.Types) == 0 {
	//	return nil, errors.New("must provide metrics types to fetch")
	//}
	//if options.Start.After(options.End) || options.Start.Equal(options.End) && !options.Start.IsZero() {
	//	return nil, errors.Errorf("end parameter must be after start: start=%s, end=%s", options.Start, options.End)
	//}
	//
	//var metricNames string
	//for _, t := range options.Types {
	//	if len(metricNames) > 0 {
	//		metricNames += ","
	//	}
	//	metricNames += string(t)
	//}
	//
	//var ag string
	//for _, a := range options.Aggregations {
	//	if len(ag) > 0 {
	//		ag += ","
	//	}
	//	ag += string(a)
	//}
	//
	//urlParams := url.Values{
	//	"api-version": []string{"2018-01-01"},
	//	"aggregation": []string{ag},
	//	"metricnames": []string{metricNames},
	//	"interval":    []string{"PT1M"}, // TODO: make configurable?
	//}
	//
	//if options.Dimension != "" {
	//	urlParams.Add("$filter", options.Dimension)
	//}
	//
	//if !options.Start.IsZero() || !options.End.IsZero() {
	//	urlParams.Add("timespan", path.Join(options.Start.Format(time.RFC3339), options.End.Format(time.RFC3339)))
	//}
	//
	//// Create the url.
	//uri := api.ResolveRelative(c.auth.ResourceManagerEndpoint, containerGroupMetricsURLPath)
	//uri += "?" + url.Values(urlParams).Encode()
	//
	//// Create the request.
	//req, err := http.NewRequest("GET", uri, nil)
	//if err != nil {
	//	return nil, errors.Wrap(err, "creating get container group metrics uri request failed")
	//}
	//req = req.WithContext(ctx)
	//
	//// Add the parameters to the url.
	//if err := api.ExpandURL(req.URL, map[string]string{
	//	"subscriptionId":     c.auth.SubscriptionID,
	//	"resourceGroup":      resourceGroup,
	//	"containerGroupName": containerGroup,
	//}); err != nil {
	//	return nil, errors.Wrap(err, "expanding URL with parameters failed")
	//}
	//
	//// Send the request.
	//resp, err := c.hc.Do(req)
	//if err != nil {
	//	return nil, errors.Wrap(err, "sending get container group metrics request failed")
	//}
	//defer resp.Body.Close()
	//
	//// 200 (OK) is a success response.
	//if err := api.CheckResponse(resp); err != nil {
	//	return nil, err
	//}
	//
	//// Decode the body from the response.
	//if resp.Body == nil {
	//	return nil, errors.New("container group metrics returned an empty body in the response")
	//}
	//var metrics ContainerGroupMetricsResult
	//if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
	//	return nil, errors.Wrap(err, "decoding get container group metrics response body failed")
	//}
	//
	//return &metrics, nil
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
