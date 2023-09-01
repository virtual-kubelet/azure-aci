/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"fmt"
	"strings"
	"os"
	"regexp"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	v1 "k8s.io/api/core/v1"
)

func SetContainerGroupIdentity(ctx context.Context, identityList []string, identityType azaciv2.ResourceIdentityType, containerGroup *azaciv2.ContainerGroup) {
	if len(identityList) == 0 || identityType != azaciv2.ResourceIdentityTypeUserAssigned {
		return
	}

	cgIdentity := azaciv2.ContainerGroupIdentity{
		Type: &identityType,
		UserAssignedIdentities: map[string]*azaciv2.UserAssignedIdentities{},
	}

	for i := range identityList {
		cgIdentity.UserAssignedIdentities[identityList[i]] =  &azaciv2.UserAssignedIdentities{}
	}

	log.G(ctx).Infof("setting managed identity based imageRegistryCredentials\n")
	containerGroup.Identity = &cgIdentity
}

func (p *ACIProvider) GetAgentPoolKubeletIdentity(ctx context.Context, pod *v1.Pod) (*string, error) {

	if kubeletIdentity := os.Getenv("AKS_KUBELET_IDENTITY"); kubeletIdentity != "" {
		return &kubeletIdentity, nil
	}

	// list identities by resource group: covers both default MC_ resource group and user defined node resource group
	idList, err := p.azClientsAPIs.GetIdentitiesListResult(ctx, p.resourceGroup)
	if err != nil {
		log.G(ctx).Errorf("Error while listing identities, %v", err)
	}
	for i := range idList {
		if strings.HasSuffix(*idList[i].ID, "agentpool") {
			return idList[i].ID, nil
		}
	}

	// ACI Resource group provided by user or a user specified kubelet identity is used on the cluster
	// find cluster in the resource group and get kubelet identity
	rg := p.resourceGroup
	if strings.HasPrefix(p.resourceGroup, "MC_") {
		rg = strings.Split(p.resourceGroup, "_")[1]
	}
	masterURI := os.Getenv("MASTER_URI")
	t := regexp.MustCompile(`[:/]`)
	masterURISplit := t.Split(masterURI, -1)
	fqdn := ""
	if len(masterURISplit) > 1 {
		fqdn = masterURISplit[3]
	}

	log.G(ctx).Infof("looking for cluster in resource group: %s \n", rg)

	// List clusters in RG and filter on fqdn
	clusterList, err := p.azClientsAPIs.GetClusterListResult(ctx, p.resourceGroup)
	if err != nil {
		log.G(ctx).Errorf("Error while listing clusters in resource group , %v", err)
	}
	for _, cluster := range clusterList {
		// pick the cluster based on fqdn
		if (*cluster.Properties.Fqdn == fqdn) {
			kubeletIdentity, ok:= cluster.Properties.IdentityProfile["kubeletidentity"]
			if !ok || kubeletIdentity == nil {
				return nil, fmt.Errorf("could not get kubelet identity from cluster\n")
			}
			return kubeletIdentity.ResourceID, nil
		}
	}

	// if all fails
	// try to find cluster in the subscription and get kubeletidentity
	clusterList, err = p.azClientsAPIs.GetClusterListBySubscriptionResult(ctx)
	if err != nil {
		log.G(ctx).Errorf("Error while listing clusters in subscription, %v", err)
	}
	// pick the cluster based on fqdn
	for _, cluster := range clusterList {
		if (*cluster.Properties.Fqdn == fqdn) {
			kubeletIdentity, ok:= cluster.Properties.IdentityProfile["kubeletidentity"]
			if !ok || kubeletIdentity == nil {
				return nil, fmt.Errorf("could not get kubelet identity from cluster\n")
			}
			return kubeletIdentity.ResourceID, nil
		}
	}
	return nil, fmt.Errorf("could not find an agent pool identity for cluster under subscription %s\n", p.providerNetwork.VnetSubscriptionID)
}
