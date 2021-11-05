package metrics 

import (
	"context"
	v1 "k8s.io/api/core/v1"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
)

type PodMetricsProvider interface {
	GetPodMetrics(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error)
}

