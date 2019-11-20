package provider

import (
	"github.com/virtual-kubelet/node-cli/manager"
	v1 "k8s.io/api/core/v1"
)

type podsTracker struct {
	rm            *manager.ResourceManager
	podFetcher    func(string, string) *v1.Pod
	updateHandler func(*v1.Pod)
}

func (pt *podsTracker) updatePods() {
	k8sPods := pt.rm.GetPods()
	for _, pod := range k8sPods {
		aciPod := pt.podFetcher(pod.Namespace, pod.Name)

	}
}
