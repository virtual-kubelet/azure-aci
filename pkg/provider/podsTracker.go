package provider

import (
	"context"
	"time"

	"github.com/virtual-kubelet/node-cli/manager"
	errdef "github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	podStatusReasonProviderFailed       = "ProviderFailed"
	statusReasonNotFound                = "NotFound"
	statusMessageNotFound               = "The pod may have been deleted from the provider"
	containerExitCodeNotFound     int32 = -137

	statusUpdatesInterval = 5 * time.Second
	cleanupInterval       = 5 * time.Minute
)

type PodIdentifier struct {
	namespace string
	name      string
}

type PodsTrackerHandler interface {
	ListActivePods(ctx context.Context) ([]PodIdentifier, error)
	FetchPodStatus(ctx context.Context, ns, name string) (*v1.PodStatus, error)
	CleanupPod(ctx context.Context, ns, name string) error
}

type PodsTracker struct {
	rm       *manager.ResourceManager
	updateCb func(*v1.Pod)
	handler  PodsTrackerHandler
}

// StartTracking starts the background tracking for created pods.
func (pt *PodsTracker) StartTracking(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "PodsTracker.StartTracking")
	defer span.End()

	statusUpdatesTimer := time.NewTimer(statusUpdatesInterval)
	cleanupTimer := time.NewTimer(cleanupInterval)
	defer statusUpdatesTimer.Stop()
	defer cleanupTimer.Stop()

	for {
		log.G(ctx).Debug("Pod status updates & cleanup loop start")

		select {
		case <-ctx.Done():
			log.G(ctx).WithError(ctx.Err()).Debug("Pod status update loop exiting")
			return
		case <-statusUpdatesTimer.C:
			pt.updatePodsLoop(ctx)
			statusUpdatesTimer.Reset(statusUpdatesInterval)
		case <-cleanupTimer.C:
			pt.cleanupDanglingPods(ctx)
			cleanupTimer.Reset(cleanupInterval)
		}
	}
}

// UpdatePodStatus updates the status of a pod, by posting to update callback.
func (pt *PodsTracker) UpdatePodStatus(ns, name string, updateHandler func(*v1.PodStatus), forceUpdate bool) error {
	k8sPods := pt.rm.GetPods()
	pod := getPodFromList(k8sPods, ns, name)

	if pod == nil {
		return errdef.NotFound("pod not found")
	}

	updatedPod := pod.DeepCopy()
	if forceUpdate {
		updatedPod.ResourceVersion = ""
	}

	updateHandler(&updatedPod.Status)
	pt.updateCb(updatedPod)
	return nil
}

func (pt *PodsTracker) updatePodsLoop(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "PodsTracker.updatePods")
	defer span.End()

	k8sPods := pt.rm.GetPods()
	for _, pod := range k8sPods {
		updatedPod := pod.DeepCopy()
		ok := pt.processPodUpdates(ctx, updatedPod)
		if ok {
			pt.updateCb(updatedPod)
		}
	}
}

func (pt *PodsTracker) cleanupDanglingPods(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "PodsTracker.cleanupDanglingPods")
	defer span.End()

	k8sPods := pt.rm.GetPods()
	activePods, err := pt.handler.ListActivePods(ctx)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to retrieve active container groups list")
		return
	}

	if len(activePods) > 0 {
		for i := range activePods {
			pod := getPodFromList(k8sPods, activePods[i].namespace, activePods[i].name)
			if pod != nil {
				continue
			}

			log.G(ctx).Errorf("cleaning up dangling pod %v", activePods[i].name)

			err := pt.handler.CleanupPod(ctx, activePods[i].namespace, activePods[i].name)
			if err != nil && !errdef.IsNotFound(err) {
				log.G(ctx).WithError(err).Errorf("failed to cleanup pod %v", activePods[i].name)
			}
		}
	}
}

func (pt *PodsTracker) processPodUpdates(ctx context.Context, pod *v1.Pod) bool {
	ctx, span := trace.StartSpan(ctx, "PodsTracker.processPodUpdates")
	defer span.End()

	if pt.shouldSkipPodStatusUpdate(pod) {
		return false
	}

	podStatusFromProvider, err := pt.handler.FetchPodStatus(ctx, pod.Namespace, pod.Name)
	if err == nil && podStatusFromProvider != nil {
		podStatusFromProvider.DeepCopyInto(&pod.Status)
		return true
	}

	if errdef.IsNotFound(err) || (err == nil && podStatusFromProvider == nil) {
		// Only change the status when the pod was already up
		if pod.Status.Phase == v1.PodRunning {
			// Set the pod to failed, this makes sure if the underlying container implementation is gone that a new pod will be created.
			pod.Status.Phase = v1.PodFailed
			pod.Status.Reason = statusReasonNotFound
			pod.Status.Message = statusMessageNotFound
			now := metav1.NewTime(time.Now())
			for i := range pod.Status.ContainerStatuses {
				if pod.Status.ContainerStatuses[i].State.Running == nil {
					continue
				}

				pod.Status.ContainerStatuses[i].State.Terminated = &v1.ContainerStateTerminated{
					ExitCode:    containerExitCodeNotFound,
					Reason:      statusReasonNotFound,
					Message:     statusMessageNotFound,
					FinishedAt:  now,
					StartedAt:   pod.Status.ContainerStatuses[i].State.Running.StartedAt,
					ContainerID: pod.Status.ContainerStatuses[i].ContainerID,
				}
				pod.Status.ContainerStatuses[i].State.Running = nil
			}

			return true
		}

		return false
	}

	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to retrieve pod %v status from provider", pod.Name)
	}

	return false
}

func (pt *PodsTracker) shouldSkipPodStatusUpdate(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || // Pod completed its execution
		pod.Status.Phase == v1.PodFailed ||
		pod.Status.Reason == podStatusReasonProviderFailed || // Pending phase because of failure
		pod.DeletionTimestamp != nil // Terminating
}

func getPodFromList(list []*v1.Pod, ns, name string) *v1.Pod {
	for _, pod := range list {
		if pod.Namespace == ns && pod.Name == name {
			return pod
		}
	}

	return nil
}
