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
	armcontainerservice "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	armmsi "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	v1 "k8s.io/api/core/v1"
)

func SetContainerGroupIdentity(ctx context.Context, identity *armmsi.Identity, identityType azaciv2.ResourceIdentityType, containerGroup *azaciv2.ContainerGroup) {
	if identity == nil || identityType != azaciv2.ResourceIdentityTypeUserAssigned {
		return
	}

	cgIdentity := azaciv2.ContainerGroupIdentity{
		Type: &identityType,
		UserAssignedIdentities: map[string]*azaciv2.UserAssignedIdentities{
			*identity.ID: &azaciv2.UserAssignedIdentities{
				PrincipalID: identity.Properties.PrincipalID,
				ClientID: identity.Properties.ClientID,
			},
		},
	}

	log.G(ctx).Infof("setting managed identity based imageRegistryCredentials\n")
	containerGroup.Identity = &cgIdentity
}

func (p *ACIProvider) GetAgentPoolKubeletIdentity(ctx context.Context, pod *v1.Pod) (*armmsi.Identity, error) {

	// initialize msi  credentials move this to setup
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err := armmsi.NewUserAssignedIdentitiesClient(p.providernetwork.VnetSubscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	// list identities by resource group: covers both default MC_ resource group and user defined node resource group
	pager := client.NewListByResourceGroupPager(p.resourceGroup, nil)
	for pager.More() {
		// pick the agent pool identity
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, v := range nextResult.Value {
			if strings.HasSuffix(*v.ID, "agentpool") {
				return v, nil
			}
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

	aksClient, err := armcontainerservice.NewManagedClustersClient(p.providernetwork.VnetSubscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	// List clusters in RG and filter on fqdn
	clusterResourceGroupPager := aksClient.NewListByResourceGroupPager(rg, nil)
	for clusterResourceGroupPager.More() {
		nextResult, err := clusterResourceGroupPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		// pick the cluster based on fqdn
		for _, cluster := range nextResult.Value {
			if (*cluster.Properties.Fqdn == fqdn) {
				kubeletIdentity, ok:= cluster.Properties.IdentityProfile["kubeletidentity"]
				if !ok || kubeletIdentity == nil {
					return nil, fmt.Errorf("could not get kubelet identity from cluster\n")
				}
				// get armmsi identity object using identity resource name
				identityResourceName := strings.SplitAfter(*kubeletIdentity.ResourceID, "userAssignedIdentities/")[1]
				userAssignedIdentityGetResponse, err := client.Get(ctx, rg, identityResourceName, nil)
				if err != nil {
					return nil, err
				}
				return &userAssignedIdentityGetResponse.Identity, nil
			}
		}
	}

	// if all fails
	// try to find cluster in the subscription and get kubeletidentity
	clusterPager := aksClient.NewListPager(nil)
	for clusterPager.More() {
		nextResult, err := clusterPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		// pick the cluster based on fqdn
		for _, cluster := range nextResult.Value {
			if (*cluster.Properties.Fqdn == fqdn) {
				kubeletIdentity, ok:= cluster.Properties.IdentityProfile["kubeletidentity"]
				if !ok || kubeletIdentity == nil {
					return nil, fmt.Errorf("could not get kubelet identity from cluster\n")
				}
				// get armmsi identity object using identity resource name
				identityResourceName := strings.SplitAfter(*kubeletIdentity.ResourceID, "userAssignedIdentities/")[1]
				userAssignedIdentityGetResponse, err := client.Get(ctx, rg, identityResourceName, nil)
				if err != nil {
					return nil, err
				}
				return &userAssignedIdentityGetResponse.Identity, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find an agent pool identity for cluster under subscription %s\n", p.providernetwork.VnetSubscriptionID)
}
