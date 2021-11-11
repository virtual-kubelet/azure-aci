package aci

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestClient_GetContainerGroupMetrics(t *testing.T) {
	ctx := context.Background()
	rg := "minsha-clusterincluster-1"
	cg := "vk-vk-test-1"
	// rg := "josephporter-cli-tests"
	// cg := "acr-test"
	// end := time.Now()
	// start := end.Add(-1 * time.Minute)
	start, _ := time.Parse(time.RFC3339, "2021-11-10T19:51:59Z")
	end, _ := time.Parse(time.RFC3339, "2021-11-10T19:52:59Z")
	fmt.Println(start)
	fmt.Println(end)
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
	// fmt.Printf("%+v", systemStats)
	b, _ := json.Marshal(systemStats)
	fmt.Println(string(b))
}
