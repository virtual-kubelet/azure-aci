package aci

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestClient_GetContainerGroupMetrics(t *testing.T) {
	ctx := context.Background()
	// rg := "minsha-clusterincluster-1"
	// cg := "vk-vk-test-1"
	rg := "josephporter-cli-tests"
	cg := "acr-test"
	end := time.Now()
	start := end.Add(-10 * time.Minute)
	systemStats, err := client.GetContainerGroupMetrics(ctx, rg, cg, MetricsRequest{
		Dimension:    "containerName eq '*'",
		Start:        start,
		End:          end,
		Aggregations: []AggregationType{AggregationTypeAverage},
		Types:        []MetricType{MetricTypeCPUUsage, MetricTypeMemoryUsage},
	})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%+v", systemStats)
}
