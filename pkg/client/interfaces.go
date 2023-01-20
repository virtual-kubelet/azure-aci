package client

import (
	"context"

	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
)

// PodGetter package dependency: query the Pods in current virtual nodes. it usually is ResourceManager
type PodGetter interface {
	GetPods() []*v1.Pod
}

// ContainerGroupGetter package dependency: query the Container Group information
type ContainerGroupGetter interface {
	GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*azaci.ContainerGroup, error)
}

/*
there are difference implementation of query Pod's statistics.
this interface is for mocking in unit test
*/
type PodStatsGetter interface {
	GetPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error)
}
