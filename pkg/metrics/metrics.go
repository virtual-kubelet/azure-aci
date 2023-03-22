package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/azure-aci/pkg/metrics/collectors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	stats "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
	compbasemetrics "k8s.io/component-base/metrics"
	dto "github.com/prometheus/client_model/go"
)

const (
	ContainerGroupCacheTTLSeconds = 60 * 5
)

type ACIPodMetricsProvider struct {
	nodeName       string
	metricsSync    sync.Mutex
	podGetter      corev1listers.PodLister
	aciCGGetter    client.ContainerGroupGetter
	podStatsGetter client.PodStatsGetter
}

func NewACIPodMetricsProvider(nodeName, aciResourcegroup string, podLister corev1listers.PodLister, aciCGGetter client.ContainerGroupGetter) *ACIPodMetricsProvider {
	provider := ACIPodMetricsProvider{
		nodeName:    nodeName,
		podGetter:   podLister,
		aciCGGetter: aciCGGetter,
	}

	realTimeGetter := WrapCachedPodStatsGetter(
		5,
		NewRealTimeMetrics())
	provider.podStatsGetter = NewPodStatsGetterDecider(realTimeGetter, aciResourcegroup, aciCGGetter)
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

	pods, err := p.podGetter.List(labels.Everything())
	if err != nil {
		log.L.WithError(err).Errorf("failed to retrieve pods list")
	}

	var errGroup errgroup.Group
	chResult := make(chan *stats.PodStats, len(pods))

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
			if podMetrics == nil {
				err := fmt.Errorf("error fetching metrics for pods '%s'. cannot retrieve the pod status", pod.Name)
				span.SetStatus(err)
				return err
			}

			chResult <- podMetrics
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
		s.Pods = append(s.Pods, *stat)
	}

	return &s, nil
}


// GetMetrics Resource returns the metrics for pods running on ACI
func (p *ACIPodMetricsProvider) GetMetricsResource(ctx context.Context) ([]*dto.MetricFamily, error) {
	ctx, span := trace.StartSpan(ctx, "GetMetricsResource")
	defer span.End()

	statsSummary, err := p.GetStatsSummary(ctx)
	if err != nil {
		span.SetStatus(err)
		return nil, errors.Wrapf(err, "error fetching MetricsResource")
	}
	if statsSummary == nil {
		err := fmt.Errorf("no stats were returned !")
		span.SetStatus(err)
		return nil, err
	}

	registry := compbasemetrics.NewKubeRegistry()
	registry.CustomMustRegister(collectors.NewKubeletResourceMetricsCollector(statsSummary))

	metricFamily, err := registry.Gather()
	if err != nil {
		span.SetStatus(err)
		return nil, errors.Wrapf(err, "error gathering metrics from collector")
	}
	return metricFamily, nil
}


type podStatsGetterDecider struct {
	realTimeGetter client.PodStatsGetter
	rgName         string
	aciCGGetter    client.ContainerGroupGetter
	cache          *cache.Cache
}

func NewPodStatsGetterDecider(realTimeGetter client.PodStatsGetter, rgName string, aciCGGetter client.ContainerGroupGetter) *podStatsGetterDecider {
	decider := &podStatsGetterDecider{
		realTimeGetter: realTimeGetter,
		rgName:         rgName,
		aciCGGetter:    aciCGGetter,
		cache:          cache.New(ContainerGroupCacheTTLSeconds*time.Second, 10*time.Minute),
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
	for _, extension := range aciCG.Properties.Extensions {
		if extension.Properties.ExtensionType != nil &&
			*extension.Properties.ExtensionType == client.ExtensionTypeRealtimeMetrics {
			useRealTime = true
		}
	}

	if useRealTime {
		logger.Infof("use Real-Time Metrics Extension for pod '%s'", pod.Name)
		return decider.realTimeGetter.GetPodStats(ctx, pod)
	} else {
		logger.Infof("no metrics has been setup for pod '%s'", pod.Name)
		return nil, nil
	}
}

func (decider *podStatsGetterDecider) getContainerGroupFromPod(ctx context.Context, pod *v1.Pod) (*azaciv2.ContainerGroup, error) {
	cgName := containerGroupName(pod.Namespace, pod.Name)
	cacheKey := string(pod.UID)
	aciContainerGroup, found := decider.cache.Get(cacheKey)
	if found {
		return aciContainerGroup.(*azaciv2.ContainerGroup), nil
	}
	aciCG, err := decider.aciCGGetter.GetContainerGroup(ctx, decider.rgName, cgName)
	if err != nil {
		return nil, err
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
