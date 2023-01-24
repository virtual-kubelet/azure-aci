/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	aznetwork "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-05-01/network"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	v1 "k8s.io/api/core/v1"
)

// DNS configuration settings
const (
	maxDNSNameservers       = 3
	maxDNSSearchPaths       = 6
	maxDNSSearchListChars   = 256
	subnetDelegationService = "Microsoft.ContainerInstance/containerGroups"
)

type ProviderNetwork struct {
	VnetSubscriptionID string
	VnetName           string
	VnetResourceGroup  string
	SubnetName         string
	SubnetCIDR         string
	KubeDNSIP          string
}

func (pn *ProviderNetwork) SetVNETConfig(ctx context.Context, azConfig *auth.Config) error {
	ctx, span := trace.StartSpan(ctx, "network.SetVNETConfig")
	defer span.End()

	// the VNET subscription ID by default is authentication subscription ID.
	// We need to override when using cross subscription virtual network resource
	pn.VnetSubscriptionID = azConfig.AuthConfig.SubscriptionID
	if vnetSubscriptionID := os.Getenv("ACI_VNET_SUBSCRIPTION_ID"); vnetSubscriptionID != "" {
		log.G(ctx).Debug("ACI VNet subscription ID env variable ACI_VNET_SUBSCRIPTION_ID is set")
		pn.VnetSubscriptionID = vnetSubscriptionID
	}

	if vnetName := os.Getenv("ACI_VNET_NAME"); vnetName != "" {
		log.G(ctx).Debug("ACI VNet name env variable ACI_VNET_NAME is set")
		pn.VnetName = vnetName
	} else if pn.VnetName == "" {
		return errors.New("vnet name can not be empty please set ACI_VNET_NAME")
	}

	if vnetResourceGroup := os.Getenv("ACI_VNET_RESOURCE_GROUP"); vnetResourceGroup != "" {
		log.G(ctx).Debug("ACI VNet resource group env variable ACI_VNET_RESOURCE_GROUP is set")

		pn.VnetResourceGroup = vnetResourceGroup
	} else if pn.VnetResourceGroup == "" {
		return errors.New("vnet resourceGroup can not be empty please set ACI_VNET_RESOURCE_GROUP")
	}

	// Set subnet properties.
	if subnetName := os.Getenv("ACI_SUBNET_NAME"); pn.VnetName != "" && subnetName != "" {
		log.G(ctx).Debug("ACI subnet name env variable ACI_SUBNET_NAME is set")
		pn.SubnetName = subnetName
	}

	if subnetCIDR := os.Getenv("ACI_SUBNET_CIDR"); subnetCIDR != "" {
		log.G(ctx).Debug("ACI subnet CIDR env variable ACI_SUBNET_CIDR is set")
		if pn.SubnetName == "" {
			return fmt.Errorf("subnet CIDR defined but no subnet name, subnet name is required to set a subnet CIDR")
		}
		if _, _, err := net.ParseCIDR(subnetCIDR); err != nil {
			return fmt.Errorf("error parsing provided subnet CIDR: %v", err)
		}
		pn.SubnetCIDR = subnetCIDR
	}

	if pn.SubnetName != "" {
		if err := pn.setupNetwork(ctx, azConfig); err != nil {
			return fmt.Errorf("error setting up network: %v", err)
		}

		if kubeDNSIP := os.Getenv("KUBE_DNS_IP"); kubeDNSIP != "" {
			log.G(ctx).Debug("kube DNS IP env variable KUBE_DNS_IP is set")
			pn.KubeDNSIP = kubeDNSIP
		}
	}
	return nil
}

func (pn *ProviderNetwork) setupNetwork(ctx context.Context, azConfig *auth.Config) error {
	logger := log.G(ctx).WithField("method", "setupNetwork")
	logger.Debug("setting up network")

	c := aznetwork.NewSubnetsClient(azConfig.AuthConfig.SubscriptionID)
	c.Authorizer = azConfig.Authorizer

	createSubnet := true
	subnet, err := c.Get(ctx, pn.VnetResourceGroup, pn.VnetName, pn.SubnetName, "")
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("error while looking up subnet: %v", err)
	}
	if errdefs.IsNotFound(err) && pn.SubnetCIDR == "" {
		return fmt.Errorf("subnet '%s' is not found in vnet '%s' in resource group '%s' and subscription '%s' and subnet CIDR is not specified", pn.SubnetName, pn.VnetName, pn.VnetResourceGroup, pn.VnetSubscriptionID)
	}
	if err == nil {
		if pn.SubnetCIDR == "" {
			pn.SubnetCIDR = *subnet.SubnetPropertiesFormat.AddressPrefix
		}
		if pn.SubnetCIDR != *subnet.SubnetPropertiesFormat.AddressPrefix {
			return fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", pn.SubnetName, *subnet.SubnetPropertiesFormat.AddressPrefix, pn.SubnetCIDR)
		}
		if subnet.SubnetPropertiesFormat.RouteTable != nil {
			return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance since it references the route table '%s'", pn.SubnetName, *subnet.SubnetPropertiesFormat.RouteTable.ID)
		}
		if subnet.SubnetPropertiesFormat.ServiceAssociationLinks != nil {
			for _, l := range *subnet.SubnetPropertiesFormat.ServiceAssociationLinks {
				if l.ServiceAssociationLinkPropertiesFormat != nil {
					if *l.ServiceAssociationLinkPropertiesFormat.LinkedResourceType == subnetDelegationService {
						createSubnet = false
						break
					} else {
						return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance as it is used by other Azure resource: '%v'", pn.SubnetName, l)
					}
				}
			}
		} else {
			for _, d := range *subnet.SubnetPropertiesFormat.Delegations {
				if d.ServiceDelegationPropertiesFormat != nil && *d.ServiceDelegationPropertiesFormat.ServiceName == subnetDelegationService {
					createSubnet = false
					break
				}
			}
		}
	}

	if createSubnet {
		logger.Debug("creating a subnet")

		var (
			delegationName = "aciDelegation"
			serviceName    = "Microsoft.ContainerInstance/containerGroups"
			subnetAction   = "Microsoft.Network/virtualNetworks/subnets/action"
		)

		subnet = aznetwork.Subnet{
			Name: &pn.SubnetName,
			SubnetPropertiesFormat: &aznetwork.SubnetPropertiesFormat{
				AddressPrefix: &pn.SubnetCIDR,
				Delegations: &[]aznetwork.Delegation{
					{
						Name: &delegationName,
						ServiceDelegationPropertiesFormat: &aznetwork.ServiceDelegationPropertiesFormat{
							ServiceName: &serviceName,
							Actions:     &[]string{subnetAction},
						},
					},
				},
			},
		}
		_, err = c.CreateOrUpdate(ctx, pn.VnetResourceGroup, pn.VnetName, pn.SubnetName, subnet)
		if err != nil {
			return fmt.Errorf("error creating subnet: %v", err)
		}
	}
	return nil
}

func (pn *ProviderNetwork) AmendVnetResources(ctx context.Context, cg azaciv2.ContainerGroup, pod *v1.Pod, clusterDomain string) {
	if pn.SubnetName == "" {
		return
	}

	subnetID := "/subscriptions/" + pn.VnetSubscriptionID + "/resourceGroups/" + pn.VnetResourceGroup + "/providers/Microsoft.Network/virtualNetworks/" + pn.VnetName + "/subnets/" + pn.SubnetName
	cgIDList := []*azaciv2.ContainerGroupSubnetID{{ID: &subnetID}}
	cg.Properties.SubnetIDs = cgIDList
	// windows containers don't support DNS config
	if cg.Properties.OSType != nil &&
		*cg.Properties.OSType != azaciv2.OperatingSystemTypesWindows {
		cg.Properties.DNSConfig = getDNSConfig(ctx, pod, pn.KubeDNSIP, clusterDomain)
	}
}

func getDNSConfig(ctx context.Context, pod *v1.Pod, kubeDNSIP, clusterDomain string) *azaciv2.DNSConfiguration {
	servers := make([]string, 0)
	searchDomains := make([]string, 0)

	if pod.Spec.DNSPolicy == v1.DNSClusterFirst || pod.Spec.DNSPolicy == v1.DNSClusterFirstWithHostNet {
		servers = append(servers, kubeDNSIP)
		searchDomains = generateSearchesForDNSClusterFirst(pod.Spec.DNSConfig, pod, clusterDomain)
	}

	options := make([]string, 0)

	if pod.Spec.DNSConfig != nil {
		servers = util.OmitDuplicates(append(servers, pod.Spec.DNSConfig.Nameservers...))
		searchDomains = util.OmitDuplicates(append(searchDomains, pod.Spec.DNSConfig.Searches...))

		for _, option := range pod.Spec.DNSConfig.Options {
			op := option.Name
			if option.Value != nil && *(option.Value) != "" {
				op = op + ":" + *(option.Value)
			}
			options = append(options, op)
		}
	}

	if len(servers) == 0 {
		return nil
	}
	servers = formDNSNameserversFitsLimits(ctx, servers)
	domain := formDNSSearchFitsLimits(ctx, searchDomains)
	nameServers := make([]*string, 0)
	for s := range servers {
		nameServers = append(nameServers, &servers[s])
	}
	opt := strings.Join(options, " ")
	result := azaciv2.DNSConfiguration{
		NameServers:   nameServers,
		SearchDomains: &domain,
		Options:       &opt,
	}

	return &result
}

// This is taken from the kubelet equivalent -  https://github.com/kubernetes/kubernetes/blob/d24fe8a801748953a5c34fd34faa8005c6ad1770/pkg/kubelet/network/dns/dns.go#L141-L151
func generateSearchesForDNSClusterFirst(dnsConfig *v1.PodDNSConfig, pod *v1.Pod, clusterDomain string) []string {
	hostSearch := make([]string, 0)

	if dnsConfig != nil {
		hostSearch = dnsConfig.Searches
	}
	if clusterDomain == "" {
		return hostSearch
	}

	nsSvcDomain := fmt.Sprintf("%s.svc.%s", pod.Namespace, clusterDomain)
	svcDomain := fmt.Sprintf("svc.%s", clusterDomain)
	clusterSearch := []string{nsSvcDomain, svcDomain, clusterDomain}

	return util.OmitDuplicates(append(clusterSearch, hostSearch...))
}

// https://github.com/kubernetes/kubernetes/blob/4276ed36282405d026d8072e0ebed4f1da49070d/pkg/kubelet/network/dns/dns.go#L101-L149
func formDNSNameserversFitsLimits(ctx context.Context, nameservers []string) []string {
	if len(nameservers) > maxDNSNameservers {
		nameservers = nameservers[:maxDNSNameservers]
		msg := fmt.Sprintf("Nameserver limits were exceeded, some nameservers have been omitted, the applied nameserver line is: %s", strings.Join(nameservers, ";"))
		log.G(ctx).WithField("method", "formDNSNameserversFitsLimits").Warn(msg)
	}
	return nameservers
}

func formDNSSearchFitsLimits(ctx context.Context, searches []string) string {
	limitsExceeded := false

	if len(searches) > maxDNSSearchPaths {
		searches = searches[:maxDNSSearchPaths]
		limitsExceeded = true
	}

	// In some DNS resolvers(e.g. glibc 2.28), DNS resolving causes abort() if there is a
	// search path exceeding 255 characters. We have to filter them out.
	l := 0
	for _, search := range searches {
		if len(search) > utilvalidation.DNS1123SubdomainMaxLength {
			limitsExceeded = true
			continue
		}
		searches[l] = search
		l++
	}
	searches = searches[:l]

	if resolveSearchLineStrLen := len(strings.Join(searches, " ")); resolveSearchLineStrLen > maxDNSSearchListChars {
		cutDomainsNum := 0
		cutDomainsLen := 0
		for i := len(searches) - 1; i >= 0; i-- {
			cutDomainsLen += len(searches[i]) + 1
			cutDomainsNum++

			if (resolveSearchLineStrLen - cutDomainsLen) <= maxDNSSearchListChars {
				break
			}
		}

		searches = searches[:(len(searches) - cutDomainsNum)]
		limitsExceeded = true
	}

	if limitsExceeded {
		msg := fmt.Sprintf("Search Line limits were exceeded, some search paths have been omitted, the applied search line is: %s", strings.Join(searches, ";"))
		log.G(ctx).WithField("method", "formDNSSearchFitsLimits").Warn(msg)
	}

	return strings.Join(searches, " ")
}
