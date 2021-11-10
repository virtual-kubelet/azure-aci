package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_realTimeMetrics_getPodStats(t *testing.T) {
	realTIme := NewRealTimeMetrics()
	ctx := context.Background()
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pod-1",
			Namespace:         "ns",
			UID:               "pod-uid",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
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
	stats, _ := realTIme.getPodStats(ctx, pod)
	b1, _ := json.Marshal(stats)
	fmt.Println(string(b1))
	time.Sleep(time.Second * 3)
	stats, _ = realTIme.getPodStats(ctx, pod)
	b2, _ := json.Marshal(stats)
	fmt.Println(string(b2))
}
