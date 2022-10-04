package pkg

import "time"

// TimeSeriesEntry is the metric data for a given timestamp/metric type
type TimeSeriesEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Average   float64   `json:"average"`
	Total     float64   `json:"total"`
	Count     float64   `json:"count"`
}

// MetricsRequest is an options struct used when getting container group metrics
type MetricsRequest struct {
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

// AggregationType is an enum type for defining supported aggregation types.
type AggregationType string

// AggregationTypeAverage Supported metric aggregation types.
const (
	AggregationTypeAverage AggregationType = "average"
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
