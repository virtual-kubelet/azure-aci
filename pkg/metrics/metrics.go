package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
)

const (
	ContainerGroupCacheTTLSeconds = 60 * 5
)

type ACIPodMetricsProvider struct {
	nodeName           string
	metricsSync        sync.Mutex
	podGetter          client.PodGetter
	aciCGGetter        client.ContainerGroupGetter
	aciCGMetricsGetter client.MetricsGetter
	podStatsGetter     client.PodStatsGetter
}

func NewACIPodMetricsProvider(nodeName, aciResourcegroup string, podGetter client.PodGetter, aciCGGetter client.ContainerGroupGetter, aciCGMetricsGetter client.MetricsGetter) *ACIPodMetricsProvider {
	provider := ACIPodMetricsProvider{
		nodeName:           nodeName,
		podGetter:          podGetter,
		aciCGGetter:        aciCGGetter,
		aciCGMetricsGetter: aciCGMetricsGetter,
	}

	containerInsightGetter := WrapCachedPodStatsGetter(
		30,
		NewContainerInsightsMetricsProvider(aciCGMetricsGetter, aciResourcegroup))
	realTimeGetter := WrapCachedPodStatsGetter(
		5,
		NewRealTimeMetrics())
	provider.podStatsGetter = NewPodStatsGetterDecider(containerInsightGetter, realTimeGetter, aciResourcegroup, aciCGGetter)
	return &provider
}

// GetStatsSummary returns the stats summary for pods running on ACI
func (p *ACIPodMetricsProvider) GetStatsSummary(ctx context.Context) (summary *stats.Summary, err error) {
	ctx, span := trace.StartSpan(ctx, "GetSummaryStats")
	defer span.End()

	p.metricsSync.Lock()
	defer p.metricsSync.Unlock()

	log.G(ctx).Debug("acquired metrics mutex")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	pods := p.podGetter.GetPods()

	var errGroup errgroup.Group
	chResult := make(chan stats.PodStats, len(pods))

	sema := make(chan struct{}, 10)
	for _, pod := range pods {
		if pod.Status.Phase != v1.PodRunning {
			continue
		}
		pod := pod
		errGroup.Go(func() error {
			ctx, span := trace.StartSpan(ctx, "getPodMetrics")
			defer span.End()
			logger := log.G(ctx).WithFields(log.Fields{
				"UID":       string(pod.UID),
				"Name":      pod.Name,
				"Namespace": pod.Namespace,
			})

			select {
			case <-ctx.Done():
				return ctx.Err()
			case sema <- struct{}{}:
			}
			defer func() {
				<-sema
			}()

			logger.Debug("Acquired semaphore")

			podMetrics, err := p.podStatsGetter.GetPodStats(ctx, pod)
			if err != nil {
				span.SetStatus(err)
				return errors.Wrapf(err, "error fetching metrics for pods '%s'", pod.Name)
			}

			chResult <- *podMetrics
			return nil
		})
	}

	if err := errGroup.Wait(); err != nil {
		span.SetStatus(err)
		return nil, errors.Wrap(err, "error in request to fetch container group metrics")
	}
	close(chResult)
	log.G(ctx).Debugf("Collected status from azure for %d pods", len(pods))

	var s stats.Summary
	s.Node = stats.NodeStats{
		NodeName: p.nodeName,
	}
	s.Pods = make([]stats.PodStats, 0, len(chResult))

	for stat := range chResult {
		s.Pods = append(s.Pods, stat)
	}

	return &s, nil
}

type podStatsGetterDecider struct {
	containerInsightsGetter client.PodStatsGetter
	realTimeGetter          client.PodStatsGetter
	rgName                  string
	aciCGGetter             client.ContainerGroupGetter
	cache                   *cache.Cache
}

func NewPodStatsGetterDecider(containerInsightsGetter client.PodStatsGetter, realTimeGetter client.PodStatsGetter, rgName string, aciCGGetter client.ContainerGroupGetter) *podStatsGetterDecider {
	decider := &podStatsGetterDecider{
		containerInsightsGetter: containerInsightsGetter,
		realTimeGetter:          realTimeGetter,
		rgName:                  rgName,
		aciCGGetter:             aciCGGetter,
		cache:                   cache.New(ContainerGroupCacheTTLSeconds*time.Second, 10*time.Minute),
	}
	return decider
}

func (decider *podStatsGetterDecider) GetPodStats(ctx context.Context, pod *v1.Pod) (*stats.PodStats, error) {
	logger := log.G(ctx).WithFields(log.Fields{
		"UID":       string(pod.UID),
		"Name":      pod.Name,
		"Namespace": pod.Namespace,
	})

	aciCG, err := decider.getContainerGroupFromPod(ctx, pod)
	if err != nil {
		logger.Errorf("failed to query Container Group %s", err)
		return nil, errors.Wrapf(err, "failed to query Container Group")
	}

	useRealTime := false
	for _, extension := range aciCG.ContainerGroupPropertiesWrapper.Extensions {
		if extension.Properties.Type == client.ExtensionTypeRealtimeMetrics {
			useRealTime = true
		}
	}

	if useRealTime {
		logger.Infof("use Real-Time Metrics Extension for pod '%s'", pod.Name)
		return decider.realTimeGetter.GetPodStats(ctx, pod)
	} else {
		logger.Infof("use Container Insights metrics for pod '%s'", pod.Name)
		return decider.containerInsightsGetter.GetPodStats(ctx, pod)
	}
}

func (decider *podStatsGetterDecider) getContainerGroupFromPod(ctx context.Context, pod *v1.Pod) (*client.ContainerGroupWrapper, error) {
	cgName := containerGroupName(pod.Namespace, pod.Name)
	cacheKey := string(pod.UID)
	aciContainerGroup, found := decider.cache.Get(cacheKey)
	if found {
		return aciContainerGroup.(*client.ContainerGroupWrapper), nil
	}
	aciCG, err := decider.aciCGGetter.GetContainerGroup(ctx, decider.rgName, cgName)
	if err != nil {
		if aciCG.StatusCode == http.StatusNotFound {
			return nil, errors.Wrapf(err, "failed to query Container Group %s, not found it", cgName)
		}
		return nil, errors.Wrapf(err, "failed to query Container Group %s", cgName)
	}
	decider.cache.Set(cacheKey, aciCG, cache.DefaultExpiration)
	return aciCG, nil
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}

func newUInt64Pointer(value int) *uint64 {
	var u = uint64(value)
	return &u
}
