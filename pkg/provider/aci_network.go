/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pkg/errors"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	aznetwork "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-05-01/network"
	"github.com/virtual-kubelet/azure-aci/client/network"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	client2 "github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	v1 "k8s.io/api/core/v1"
)

// DNS configuration settings
const (
	maxDNSNameservers     = 3
	maxDNSSearchPaths     = 6
	maxDNSSearchListChars = 256
)

func (p *ACIProvider) setVNETConfig(ctx context.Context, azConfig *auth.Config) error {
	// the VNET subscription ID by default is authentication subscription ID.
	// We need to override when using cross subscription virtual network resource
	p.vnetSubscriptionID = azConfig.AuthConfig.SubscriptionID
	if vnetSubscriptionID := os.Getenv("ACI_VNET_SUBSCRIPTION_ID"); vnetSubscriptionID != "" {
		p.vnetSubscriptionID = vnetSubscriptionID
	}

	if vnetName := os.Getenv("ACI_VNET_NAME"); vnetName != "" {
		p.vnetName = vnetName
	} else if p.vnetName == "" {
		return errors.New("vnet name can not be empty please set ACI_VNET_NAME")
	}

	if vnetResourceGroup := os.Getenv("ACI_VNET_RESOURCE_GROUP"); vnetResourceGroup != "" {
		p.vnetResourceGroup = vnetResourceGroup
	} else if p.vnetResourceGroup == "" {
		return errors.New("vnet resourceGroup can not be empty please set ACI_VNET_RESOURCE_GROUP")
	}

	// Set subnet properties.
	if subnetName := os.Getenv("ACI_SUBNET_NAME"); p.vnetName != "" && subnetName != "" {
		p.subnetName = subnetName
	}

	if subnetCIDR := os.Getenv("ACI_SUBNET_CIDR"); subnetCIDR != "" {
		if p.subnetName == "" {
			return fmt.Errorf("subnet CIDR defined but no subnet name, subnet name is required to set a subnet CIDR")
		}
		if _, _, err := net.ParseCIDR(subnetCIDR); err != nil {
			return fmt.Errorf("error parsing provided subnet range: %v", err)
		}
		p.subnetCIDR = subnetCIDR
	}

	if p.subnetName != "" {
		if err := p.setupNetwork(ctx, azConfig); err != nil {
			return fmt.Errorf("error setting up network: %v", err)
		}

		masterURI := os.Getenv("MASTER_URI")
		if masterURI == "" {
			masterURI = "10.0.0.1"
		}

		clusterCIDR := os.Getenv("CLUSTER_CIDR")
		if clusterCIDR == "" {
			clusterCIDR = "10.240.0.0/16"
		}

		// setup aci extensions
		kubeExtensions, err := client2.GetKubeProxyExtension(serviceAccountSecretMountPath, masterURI, clusterCIDR)
		if err != nil {
			return fmt.Errorf("error creating kube proxy extension: %v", err)
		}

		p.containerGroupExtensions = append(p.containerGroupExtensions, kubeExtensions)

		enableRealTimeMetricsExtension := os.Getenv("ENABLE_REAL_TIME_METRICS")
		if enableRealTimeMetricsExtension == "true" {
			realtimeExtension := client2.GetRealtimeMetricsExtension()
			p.containerGroupExtensions = append(p.containerGroupExtensions, realtimeExtension)
		}

		if kubeDNSIP := os.Getenv("KUBE_DNS_IP"); kubeDNSIP != "" {
			p.kubeDNSIP = kubeDNSIP
		}
	}
	return nil
}

func (p *ACIProvider) setupNetwork(ctx context.Context, azConfig *auth.Config) error {
	c := aznetwork.NewSubnetsClient(azConfig.AuthConfig.SubscriptionID)
	c.Authorizer = azConfig.Authorizer

	createSubnet := true
	subnet, err := c.Get(ctx, p.vnetResourceGroup, p.vnetName, p.subnetName, "")
	if err != nil && !network.IsNotFound(err) {
		return fmt.Errorf("error while looking up subnet: %v", err)
	}
	if network.IsNotFound(err) && p.subnetCIDR == "" {
		return fmt.Errorf("subnet '%s' is not found in vnet '%s' in resource group '%s' and subscription '%s' and subnet CIDR is not specified", p.subnetName, p.vnetName, p.vnetResourceGroup, p.vnetSubscriptionID)
	}
	if err == nil {
		if p.subnetCIDR == "" {
			p.subnetCIDR = *subnet.SubnetPropertiesFormat.AddressPrefix
		}
		if p.subnetCIDR != *subnet.SubnetPropertiesFormat.AddressPrefix {
			return fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", p.subnetName, *subnet.SubnetPropertiesFormat.AddressPrefix, p.subnetCIDR)
		}
		if subnet.SubnetPropertiesFormat.RouteTable != nil {
			return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance since it references the route table '%s'", p.subnetName, *subnet.SubnetPropertiesFormat.RouteTable.ID)
		}
		if subnet.SubnetPropertiesFormat.ServiceAssociationLinks != nil {
			for _, l := range *subnet.SubnetPropertiesFormat.ServiceAssociationLinks {
				if l.ServiceAssociationLinkPropertiesFormat != nil {
					if *l.ServiceAssociationLinkPropertiesFormat.LinkedResourceType == subnetDelegationService {
						createSubnet = false
						break
					} else {
						return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance as it is used by other Azure resource: '%v'", p.subnetName, l)
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
		var (
			delegationName = "aciDelegation"
			serviceName    = "Microsoft.ContainerInstance/containerGroups"
			subnetAction   = "Microsoft.Network/virtualNetworks/subnets/action"
		)

		subnet = aznetwork.Subnet{
			Name: &p.subnetName,
			SubnetPropertiesFormat: &aznetwork.SubnetPropertiesFormat{
				AddressPrefix: &p.subnetCIDR,
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
		_, err = c.CreateOrUpdate(ctx, p.vnetResourceGroup, p.vnetName, p.subnetName, subnet)
		if err != nil {
			return fmt.Errorf("error creating subnet: %v", err)
		}
	}
	return nil
}

func (p *ACIProvider) amendVnetResources(ctx context.Context, cg client2.ContainerGroupWrapper, pod *v1.Pod) {
	if p.subnetName == "" {
		return
	}

	subnetID := "/subscriptions/" + p.vnetSubscriptionID + "/resourceGroups/" + p.vnetResourceGroup + "/providers/Microsoft.Network/virtualNetworks/" + p.vnetName + "/subnets/" + p.subnetName
	cgIDList := []azaci.ContainerGroupSubnetID{{ID: &subnetID}}
	cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.SubnetIds = &cgIDList
	cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.DNSConfig = p.getDNSConfig(ctx, pod)
	cg.ContainerGroupPropertiesWrapper.Extensions = p.containerGroupExtensions
}

func (p *ACIProvider) getDNSConfig(ctx context.Context, pod *v1.Pod) *azaci.DNSConfiguration {
	nameServers := make([]string, 0)
	searchDomains := make([]string, 0)

	if pod.Spec.DNSPolicy == v1.DNSClusterFirst || pod.Spec.DNSPolicy == v1.DNSClusterFirstWithHostNet {
		nameServers = append(nameServers, p.kubeDNSIP)
		searchDomains = p.generateSearchesForDNSClusterFirst(pod.Spec.DNSConfig, pod)
	}

	options := make([]string, 0)

	if pod.Spec.DNSConfig != nil {
		nameServers = omitDuplicates(append(nameServers, pod.Spec.DNSConfig.Nameservers...))
		searchDomains = omitDuplicates(append(searchDomains, pod.Spec.DNSConfig.Searches...))

		for _, option := range pod.Spec.DNSConfig.Options {
			op := option.Name
			if option.Value != nil && *(option.Value) != "" {
				op = op + ":" + *(option.Value)
			}
			options = append(options, op)
		}
	}

	if len(nameServers) == 0 {
		return nil
	}
	nameServers = formDNSNameserversFitsLimits(ctx, nameServers)
	domain := formDNSSearchFitsLimits(ctx, searchDomains)
	opt := strings.Join(options, " ")
	result := azaci.DNSConfiguration{
		NameServers:   &nameServers,
		SearchDomains: &domain,
		Options:       &opt,
	}

	return &result
}

// This is taken from the kubelet equivalent -  https://github.com/kubernetes/kubernetes/blob/d24fe8a801748953a5c34fd34faa8005c6ad1770/pkg/kubelet/network/dns/dns.go#L141-L151
func (p *ACIProvider) generateSearchesForDNSClusterFirst(dnsConfig *v1.PodDNSConfig, pod *v1.Pod) []string {

	hostSearch := make([]string, 0)

	if dnsConfig != nil {
		hostSearch = dnsConfig.Searches
	}
	if p.clusterDomain == "" {
		return hostSearch
	}

	nsSvcDomain := fmt.Sprintf("%s.svc.%s", pod.Namespace, p.clusterDomain)
	svcDomain := fmt.Sprintf("svc.%s", p.clusterDomain)
	clusterSearch := []string{nsSvcDomain, svcDomain, p.clusterDomain}

	return omitDuplicates(append(clusterSearch, hostSearch...))
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

func getProtocol(pro v1.Protocol) azaci.ContainerNetworkProtocol {
	switch pro {
	case v1.ProtocolUDP:
		return azaci.ContainerNetworkProtocolUDP
	default:
		return azaci.ContainerNetworkProtocolTCP
	}
}
