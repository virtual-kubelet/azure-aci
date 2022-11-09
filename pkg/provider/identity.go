package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"regexp"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	client2 "github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	armcontainerservice "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	armmsi "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func SetContainerGroupIdentity(identity *armmsi.Identity, identityType azaci.ResourceIdentityType, containerGroup *client2.ContainerGroupWrapper) {
	if identity == nil || identityType != azaci.ResourceIdentityTypeUserAssigned {
		return
	}

	cgIdentity := azaci.ContainerGroupIdentity{
		Type: identityType,
		UserAssignedIdentities: map[string]*azaci.ContainerGroupIdentityUserAssignedIdentitiesValue{
			*identity.ID: &azaci.ContainerGroupIdentityUserAssignedIdentitiesValue{
				PrincipalID: identity.Properties.PrincipalID,
				ClientID: identity.Properties.ClientID,
			},
		},
	}
	containerGroup.Identity = &cgIdentity
}

func GetAgentPoolKubeletIdentity(ctx context.Context, providerResourceGroup string, providerVnetSubscriptionID string) (*armmsi.Identity, error) {

	// initialize msi  credentials move this to setup
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err := armmsi.NewUserAssignedIdentitiesClient(providerVnetSubscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	// default MC_ resource group
	if strings.HasPrefix(providerResourceGroup, "MC_") {
		// use sdk to list identities by RG
		pager := client.NewListByResourceGroupPager(providerResourceGroup, nil)
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
	}

	// Resource group provided by user or a non default kubelet identity is used on the cluster
	// list AKS clusters by resource group, and filter on fqdn to get fkubelet identity
	rg := providerResourceGroup
	if strings.HasPrefix(providerResourceGroup, "MC_") {
		rg = strings.Split(providerResourceGroup, "_")[1]
	}
	masterURI := os.Getenv("MASTER_URI")
	t := regexp.MustCompile(`[:/]`)
	masterURISplit := t.Split(masterURI, -1)
	clusterFqdn := ""
	if len(masterURISplit) > 1 {
		clusterFqdn = masterURISplit[3]
	}

	log.G(ctx).Infof("looking for cluster in resource group: %s \n", rg)

	aksClient, err := armcontainerservice.NewManagedClustersClient(providerVnetSubscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}
	clusterPager := aksClient.NewListByResourceGroupPager(rg, nil)
	for clusterPager.More() {
		nextResult, err := clusterPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		// pick the cluster based on fqdn
		for _, cluster := range nextResult.Value {
			if (*cluster.Properties.Fqdn == clusterFqdn) {
				kubeletIdentity, ok:= cluster.Properties.IdentityProfile["kubeletidentity"]
				if !ok || kubeletIdentity == nil {
					return nil, fmt.Errorf("could not get kubelet identity from cluster")
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

	return nil, fmt.Errorf("could not find an agent pool identity for cluster under resource group %s", providerResourceGroup)
}
