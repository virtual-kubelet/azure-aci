package client

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/pkg/errors"
)

const (
	containerGroupMetricsURLPath = containerGroupURLPath + "/providers/microsoft.Insights/metrics"
)

// AggregationType is an enum type for defining supported aggregation types.
type AggregationType string

// AggregationTypeAverage Supported metric aggregation types.
const (
	AggregationTypeAverage AggregationType = "average"
)

// TimeSeriesEntry is the metric data for a given timestamp/metric type
type TimeSeriesEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Average   float64   `json:"average"`
	Total     float64   `json:"total"`
	Count     float64   `json:"count"`
}

// MetricsRequestOptions is a struct used when getting container group metrics
type MetricsRequestOptions struct {
	Start        time.Time
	End          time.Time
	Types        []MetricType
	Aggregations []AggregationType

	// Note that a dimension may not be available for certain metrics.
	// In such cases, you will need to make separate requests.
	Dimension string
}

// MetricType is an enum type for defining supported metric types.
type MetricType string

// Supported metric types.
const (
	MetricTypeCPUUsage                          MetricType = "CpuUsage"
	MetricTypeMemoryUsage                       MetricType = "MemoryUsage"
	MetricTyperNetworkBytesRecievedPerSecond    MetricType = "NetworkBytesReceivedPerSecond"
	MetricTyperNetworkBytesTransmittedPerSecond MetricType = "NetworkBytesTransmittedPerSecond"
)

// ContainerGroupMetricsResult stores all the results for a container group metrics request.
type ContainerGroupMetricsResult struct {
	Value []MetricValue `json:"value"`
}

// MetricValue stores metrics results
type MetricValue struct {
	ID         string             `json:"id"`
	Desc       MetricDescriptor   `json:"name"`
	Timeseries []MetricTimeSeries `json:"timeseries"`
	Type       string             `json:"type"`
	Unit       string             `json:"unit"`
}

// MetricDescriptor stores the name for a given metric and the localized version of that name.
type MetricDescriptor struct {
	Value          MetricType `json:"value"`
	LocalizedValue string     `json:"localizedValue"`
}

// MetricMetadataValue stores extra metadata about a metric
// In particular it is used to provide details about the breakdown of a metric dimension.
type MetricMetadataValue struct {
	Name  ValueDescriptor `json:"name"`
	Value string          `json:"value"`
}

// ValueDescriptor describes a generic value. It is used to describe metadata fields.
type ValueDescriptor struct {
	Value          string `json:"value"`
	LocalizedValue string `json:"localizedValue"`
}

// MetricTimeSeries is the time series for a given metric.
// It contains all the metrics values and other details for the dimension the metrics are aggregated on.
type MetricTimeSeries struct {
	Data           []TimeSeriesEntry     `json:"data"`
	MetadataValues []MetricMetadataValue `json:"metadatavalues,omitempty"`
}

func (c *ContainerGroupsClientWrapper) GetMetrics(ctx context.Context, resourceGroup, cgName string, metricsRequest MetricsRequestOptions) (*ContainerGroupMetricsResult, error) {
	req, err := c.getMetricsPreparer(ctx, resourceGroup, cgName, metricsRequest)
	if err != nil {
		return nil, err
	}
	result, err := c.CGClient.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if result.StatusCode != http.StatusOK {
		return nil, errors.Errorf("an error has occurred while trying to get container group metrics request failed. Status code: %d", result.StatusCode)
	}

	if result.Body == nil {
		return nil, errors.New("container group metrics returned an empty body in the response")
	}

	var metrics ContainerGroupMetricsResult
	if err := json.NewDecoder(result.Body).Decode(&metrics); err != nil {
		return nil, errors.Wrap(err, "an error has occurred while trying to decode get container group metrics response body failed")
	}

	return &metrics, nil

}

func (c *ContainerGroupsClientWrapper) getMetricsPreparer(ctx context.Context, resourceGroupName, cgName string, metricsRequest MetricsRequestOptions) (*http.Request, error) {
	if len(metricsRequest.Types) == 0 {
		return nil, errors.New("must provide metrics types to fetch")
	}
	if metricsRequest.Start.After(metricsRequest.End) || metricsRequest.Start.Equal(metricsRequest.End) && !metricsRequest.Start.IsZero() {
		return nil, errors.Errorf("end parameter must be after start: start=%s, end=%s", metricsRequest.Start, metricsRequest.End)
	}

	var metricNames string
	for _, t := range metricsRequest.Types {
		if len(metricNames) > 0 {
			metricNames += ","
		}
		metricNames += string(t)
	}

	var aggregations string
	for _, a := range metricsRequest.Aggregations {
		if len(aggregations) > 0 {
			aggregations += ","
		}
		aggregations += string(a)
	}

	pathParameters := map[string]interface{}{
		"containerGroupName": autorest.Encode("path", cgName),
		"resourceGroup":      autorest.Encode("path", resourceGroupName),
		"subscriptionId":     autorest.Encode("path", c.CGClient.SubscriptionID),
	}

	queryParameters := make(map[string]interface{})

	queryParameters["api-version"] = []string{APIVersion}
	queryParameters["aggregation"] = []string{aggregations}
	queryParameters["metricnames"] = []string{metricNames}
	queryParameters["interval"] = []string{"PT1M"} // TODO: make configurable?

	if metricsRequest.Dimension != "" {
		queryParameters["$filter"] = metricsRequest.Dimension
	}

	if !metricsRequest.Start.IsZero() || !metricsRequest.End.IsZero() {
		queryParameters["timespan"] = path.Join(metricsRequest.Start.Format(time.RFC3339), metricsRequest.End.Format(time.RFC3339))
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsGet(),
		autorest.WithBaseURL(c.CGClient.BaseURI),
		autorest.WithPathParameters(containerGroupMetricsURLPath, pathParameters),
		autorest.WithQueryParameters(queryParameters))

	return preparer.Prepare((&http.Request{}).WithContext(ctx))

}
