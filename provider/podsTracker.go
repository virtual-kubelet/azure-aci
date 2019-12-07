package provider

import (
	"context"
	"time"

	"github.com/docker/docker/errdefs"
	"github.com/virtual-kubelet/node-cli/manager"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type podsTracker struct {
	rm            *manager.ResourceManager
	podFetcher    func(string, string) (*v1.Pod, error)
	updateHandler func(*v1.Pod)
}

func (pt *podsTracker) StartTracking(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "podsTracker.StartTracking")
	defer span.End()

	interval := 5 * time.Second

	timer := time.NewTimer(interval)
	defer timer.Stop()

	if !timer.Stop() {
		<-timer.C
	}

	for {
		log.G(ctx).Debug("Pod status update loop start")
		timer.Reset(interval)

		select {
		case <-ctx.Done():
			log.G(ctx).WithError(ctx.Err()).Debug("Pod status update loop exiting")
			return
		case <-timer.C:
		}
		pt.updatePods(ctx)
	}
}

func (pt *podsTracker) updatePods(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "podsTracker.updatePods")
	defer span.End()

	k8sPods := pt.rm.GetPods()
	// TODO check pods returned

	for _, pod := range k8sPods {
		if pt.shouldSkipPodStatusUpdate(pod) {
			continue
		}

		log.G(ctx).WithField("podName", pod.Name).Infof("processing pod updates...")

		podStatus, err := pt.getPodStatusFromProvider(pod)
		// TODO: check failures

		updatedPod := pod.DeepCopy()
		podStatus.DeepCopyInto(&updatedPod.Status)
		pt.updateHandler(updatedPod)
	}
}

func (pt *podsTracker) shouldSkipPodStatusUpdate(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded ||
		pod.Status.Phase == v1.PodFailed ||
		pod.Status.Reason == podStatusReasonProviderFailed // Pending because of failure
}

func (pt *podsTracker) getPodStatusFromProvider(podFromKubernetes *v1.Pod) (*v1.PodStatus, error) {
	var podStatus *v1.PodStatus
	pod, err := pt.podFetcher(podFromKubernetes.Namespace, podFromKubernetes.Name)
	if pod != nil {
		podStatus = &pod.Status
	}

	if errdefs.IsNotFound(err) || (err == nil && podStatus == nil) {
		// Only change the status when the pod was already up
		if podFromKubernetes.Status.Phase == v1.PodRunning {
			// Set the pod to failed, this makes sure if the underlying container implementation is gone that a new pod will be created.
			podStatus = podFromKubernetes.Status.DeepCopy()
			podStatus.Phase = v1.PodFailed
			podStatus.Reason = podStatusReasonNotFound
			podStatus.Message = podStatusMessageNotFound
			now := metav1.NewTime(time.Now())
			for i, c := range podStatus.ContainerStatuses {
				if c.State.Running == nil {
					continue
				}

				podStatus.ContainerStatuses[i].State.Terminated = &v1.ContainerStateTerminated{
					ExitCode:    containerStatusExitCodeNotFound,
					Reason:      containerStatusReasonNotFound,
					Message:     containerStatusMessageNotFound,
					FinishedAt:  now,
					StartedAt:   c.State.Running.StartedAt,
					ContainerID: c.ContainerID,
				}
				podStatus.ContainerStatuses[i].State.Running = nil
			}
		} else {
			return nil, nil
		}
	} else if err != nil {
		return nil, err
	}

	return podStatus, nil
}
