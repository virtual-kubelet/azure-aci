package metrics

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func Test_getRealTimePodStats(t *testing.T) {
	ctx := context.Background()
	pod := v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "my-container-group-name",
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
			PodIP: "20.99.248.167",
		},
	}
	stats, err := getRealTimePodStats(ctx, &pod)
	if err != nil {
		fmt.Println("effffffffffffff", err)
	}
	fmt.Printf("%+v\n", stats)

	for _, containerStats := range stats.Containers {
		fmt.Println(containerStats.Name)
	}
}
