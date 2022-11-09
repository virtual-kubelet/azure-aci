/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"os"

	"github.com/virtual-kubelet/virtual-kubelet/trace"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigureNode enables a provider to configure the node object that
// will be used for Kubernetes.
func (p *ACIProvider) ConfigureNode(ctx context.Context, node *v1.Node) {
	node.Status.Capacity = p.capacity()
	node.Status.Allocatable = p.capacity()
	node.Status.Conditions = p.nodeConditions()
	node.Status.Addresses = p.nodeAddresses()
	node.Status.DaemonEndpoints = p.nodeDaemonEndpoints()
	node.Status.NodeInfo.OperatingSystem = p.operatingSystem
	node.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	node.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"

	// Virtual node would be skipped for cloud provider operations (e.g. CP should not add route).
	node.ObjectMeta.Labels["kubernetes.azure.com/managed"] = "false"
}

// capacity returns a resource list containing the capacity limits set for ACI.
func (p *ACIProvider) capacity() v1.ResourceList {
	resourceList := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse(p.cpu),
		v1.ResourceMemory: resource.MustParse(p.memory),
		v1.ResourcePods:   resource.MustParse(p.pods),
	}

	if p.gpu != "" {
		resourceList[gpuResourceName] = resource.MustParse(p.gpu)
	}

	return resourceList
}

// nodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *ACIProvider) nodeConditions() []v1.NodeCondition {
	// TODO: Make these dynamic and augment with custom ACI specific conditions of interest
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionTrue,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is ready.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}
}

// nodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *ACIProvider) nodeAddresses() []v1.NodeAddress {
	// TODO: Make these dynamic and augment with custom ACI specific conditions of interest
	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: p.internalIP,
		},
	}
}

// nodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *ACIProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}

func (p *ACIProvider) setupNodeCapacity(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "aci.setupNodeCapacity")
	defer span.End()
	_ = addAzureAttributes(ctx, span, p)

	// Set sane defaults for Capacity in case config is not supplied
	p.cpu = "10000"
	p.memory = "4Ti"
	p.pods = "5000"

	if cpuQuota := os.Getenv("ACI_QUOTA_CPU"); cpuQuota != "" {
		p.cpu = cpuQuota
	}

	if memoryQuota := os.Getenv("ACI_QUOTA_MEMORY"); memoryQuota != "" {
		p.memory = memoryQuota
	}

	if podsQuota := os.Getenv("ACI_QUOTA_POD"); podsQuota != "" {
		p.pods = podsQuota
	}

	//TODO To be uncommented after Location API fix
	//capabilities, err := p.azClientsAPIs.ListCapabilities(ctx, p.region)
	//if err != nil {
	//	return errors.Wrapf(err, "Unable to fetch the ACI capabilities for the location %s, skipping GPU availability check. GPU capacity will be disabled", p.region)
	//}
	//
	//for _, capability := range *capabilities {
	//	if strings.EqualFold(*capability.Location, p.region) && *capability.Gpu != "" {
	//		p.gpu = "100"
	//		if gpu := os.Getenv("ACI_QUOTA_GPU"); gpu != "" {
	//			p.gpu = gpu
	//		}
	//		p.gpuSKUs = append(p.gpuSKUs, azaci.GpuSku(*capability.Gpu))
	//	}
	//}

	return nil
}
