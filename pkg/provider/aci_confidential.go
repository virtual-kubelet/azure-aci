/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"strings"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	v1 "k8s.io/api/core/v1"
)

const (
	confidentialComputeSkuLabel       = "virtual-kubelet.io/container-sku"
	confidentialComputeCcePolicyLabel = "virtual-kubelet.io/confidential-compute-cce-policy"
)

func (p *ACIProvider) setConfidentialComputeProperties(ctx context.Context, pod *v1.Pod, cg *azaciv2.ContainerGroup) {
	containerGroupSku := pod.Annotations[confidentialComputeSkuLabel]
	ccePolicy := pod.Annotations[confidentialComputeCcePolicyLabel]
	confidentialSku := azaciv2.ContainerGroupSKUConfidential

	l := log.G(ctx).WithField("containerGroup", cg.Name)

	if ccePolicy != "" {
		cg.Properties.SKU = &confidentialSku
		confidentialComputeProperties := azaciv2.ConfidentialComputeProperties{
			CcePolicy: &ccePolicy,
		}
		cg.Properties.ConfidentialComputeProperties = &confidentialComputeProperties
		l.Infof("setting confidential compute properties with CCE Policy")

	} else if strings.ToLower(containerGroupSku) == "confidential" {
		cg.Properties.SKU = &confidentialSku
		l.Infof("setting confidential container group SKU")
	} else {
		l.Infof("no annotations for confidential SKU")
	}

}

func getCapabilityStringPtr(capability v1.Capability) *string {
	capString := string(capability)
	return &capString
}

// sets the security context for each container from the pod spec
func (p *ACIProvider) getSecurityContext(ctx context.Context, podSecurityContext *v1.PodSecurityContext, containerSecurityContext *v1.SecurityContext) *azaciv2.SecurityContextDefinition {

	aciSecurityContext := azaciv2.SecurityContextDefinition{}
	if podSecurityContext != nil {
		if podSecurityContext.RunAsUser != nil {
			user := int32(*podSecurityContext.RunAsUser)
			aciSecurityContext.RunAsUser = &user
		}
		if podSecurityContext.RunAsGroup != nil {
			group :=  int32(*podSecurityContext.RunAsGroup)
			aciSecurityContext.RunAsGroup = &group
		}
		aciSecurityContext.SeccompProfile = nil
		if podSecurityContext.SeccompProfile != nil {
			log.G(ctx).Warnf("SeccompProfile is currently not supported. Skipping seccomp profile")
		}
	}

	if containerSecurityContext != nil {
		if containerSecurityContext.RunAsUser != nil {
			user := int32(*containerSecurityContext.RunAsUser)
			aciSecurityContext.RunAsUser = &user
		}
		if containerSecurityContext.RunAsGroup != nil {
			group := int32(*containerSecurityContext.RunAsGroup)
			aciSecurityContext.RunAsGroup = &group
		}
		aciSecurityContext.Privileged = containerSecurityContext.Privileged
		aciSecurityContext.AllowPrivilegeEscalation = containerSecurityContext.AllowPrivilegeEscalation
		if containerSecurityContext.Capabilities != nil {
			aciSecurityContext.Capabilities = &azaciv2.SecurityContextCapabilitiesDefinition{}
			add := make([]*string, 0)
			drop := make([]*string, 0)
			for i := range containerSecurityContext.Capabilities.Add {
				add = append(add, getCapabilityStringPtr(containerSecurityContext.Capabilities.Add[i]))
			}
			for i := range containerSecurityContext.Capabilities.Drop {
				drop = append(drop, getCapabilityStringPtr(containerSecurityContext.Capabilities.Drop[i]))
			}
			aciSecurityContext.Capabilities.Add = add
			aciSecurityContext.Capabilities.Drop = drop
		}
		if containerSecurityContext.SeccompProfile != nil {
			log.G(ctx).Warnf("SeccompProfile is currently not supported. Skipping seccomp profile")
		}
	}

	return &aciSecurityContext
}

func isConfidentialSku(pod *v1.Pod) bool {
	ccePolicy := pod.Annotations[confidentialComputeCcePolicyLabel]
	containerGroupSku := pod.Annotations[confidentialComputeSkuLabel]
	if ccePolicy != "" || strings.ToLower(containerGroupSku) == "confidential" {
		return true
	}
	return false
}
