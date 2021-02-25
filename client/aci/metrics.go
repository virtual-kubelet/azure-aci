package aci

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/preview/monitor/mgmt/2019-06-01/insights"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
)

// GetContainerGroupMetrics gets metrics for the provided container group
func (c *Client) GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options MetricsRequest) (insights.Response, error) {
	//result, err := c.metricsClient.List(ctx, )

	if len(options.Types) == 0 {
		return insights.Response{}, errors.New("must provide metrics types to fetch")
	}
	if options.Start.After(options.End) || options.Start.Equal(options.End) && !options.Start.IsZero() {
		return insights.Response{}, errors.Errorf("end parameter must be after start: start=%s, end=%s", options.Start, options.End)
	}

	var metricNames string
	for _, t := range options.Types {
		if len(metricNames) > 0 {
			metricNames += ","
		}
		metricNames += string(t)
	}

	var ag string
	for _, a := range options.Aggregations {
		if len(ag) > 0 {
			ag += ","
		}
		ag += string(a)
	}

	var filter string
	if options.Dimension != "" {
		filter = options.Dimension
	}

	var timespan string
	if !options.Start.IsZero() || !options.End.IsZero() {
		timespan = path.Join(options.Start.Format(time.RFC3339), options.End.Format(time.RFC3339))
	}

	resourceURI := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerInstance/containerGroups/%s", c.auth.SubscriptionID, resourceGroup, containerGroup)
	return c.metricsClient.List(ctx,
		resourceURI,
		timespan,
		to.StringPtr("PT1M"),
		metricNames,
		ag,
		nil,    // top?
		"",     //orderyby
		filter, //filter
		"",     //resultType
		"",     //metricnamespace
	)
}
