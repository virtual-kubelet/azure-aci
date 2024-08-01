/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	aznetworkv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
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

var (
	delegationName = "aciDelegation"
	serviceName    = "Microsoft.ContainerInstance/containerGroups"
	subnetAction   = "Microsoft.Network/virtualNetworks/subnets/action"
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

	err := pn.validateNetworkConfig(ctx, azConfig)
	if err != nil {
		return err
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

func (pn *ProviderNetwork) validateNetworkConfig(ctx context.Context, azConfig *auth.Config) error {
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
	return nil
}

func (pn *ProviderNetwork) setupNetwork(ctx context.Context, azConfig *auth.Config) error {
	logger := log.G(ctx).WithField("method", "setupNetwork")
	ctx, span := trace.StartSpan(ctx, "network.setupNetwork")
	defer span.End()

	subnetsClient, err := pn.GetSubnetClient(ctx, azConfig)
	if err != nil {
		return err
	}

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	updateSubnet := true
	currentSubnet, err := pn.GetACISubnet(ctxWithResp, subnetsClient)
	if err != nil {
		return err
	}

	createNewSubnet := false
	if currentSubnet == (aznetworkv2.Subnet{}) {
		createNewSubnet = true
	} else {
		// check if the current subnet is valid or if we need to create a new subnet
		updateSubnet, err = pn.shouldCreateSubnet(currentSubnet, updateSubnet)
		if err != nil {
			return err
		}
	}

	if updateSubnet {
		// decide whether to create a new subnet or update the existing one based on createNewSubnet bool
		err2 := pn.CreateOrUpdateACISubnet(ctx, subnetsClient, createNewSubnet)
		if err2 != nil {
			return err2
		}
	}

	logger.Debug("setup network is successful")
	return nil
}

func (pn *ProviderNetwork) shouldCreateSubnet(currentSubnet aznetworkv2.Subnet, createSubnet bool) (bool, error) {
	//check if addressPrefix has been set
	if currentSubnet.Properties.AddressPrefix != nil && len(*currentSubnet.Properties.AddressPrefix) > 0 {
		if pn.SubnetCIDR == "" {
			pn.SubnetCIDR = *currentSubnet.Properties.AddressPrefix
		}
		if pn.SubnetCIDR != *currentSubnet.Properties.AddressPrefix {
			return false, fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", pn.SubnetName, *currentSubnet.Properties.AddressPrefix, pn.SubnetCIDR)
		}
	} else if len(currentSubnet.Properties.AddressPrefixes) > 0 { // else check if addressPrefixes array has been set
		firstPrefix := currentSubnet.Properties.AddressPrefixes[0]
		if pn.SubnetCIDR == "" {
			pn.SubnetCIDR = *firstPrefix
		}
		if pn.SubnetCIDR != *firstPrefix {
			return false, fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", pn.SubnetName, *firstPrefix, pn.SubnetCIDR)
		}
	} else {
		return false, fmt.Errorf("both AddressPrefix and AddressPrefixes for subnet '%s' are not set", pn.SubnetName)
	}

	if currentSubnet.Properties.RouteTable != nil {
		return false, fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance since it references the route table '%s'", pn.SubnetName, *currentSubnet.Properties.RouteTable.ID)
	}
	if currentSubnet.Properties.ServiceAssociationLinks != nil {
		for _, l := range currentSubnet.Properties.ServiceAssociationLinks {
			if l.Properties != nil && l.Properties.LinkedResourceType != nil {
				if *l.Properties.LinkedResourceType == subnetDelegationService {
					createSubnet = false
					break
				} else {
					return false, fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance as it is used by other Azure resource: '%v'", pn.SubnetName, l)
				}
			}
		}
	} else {
		for _, d := range currentSubnet.Properties.Delegations {
			if d.Properties != nil && d.Properties.ServiceName != nil &&
				*d.Properties.ServiceName == subnetDelegationService {
				createSubnet = false
				break
			}
		}
	}
	return createSubnet, nil
}

func (pn *ProviderNetwork) GetACISubnet(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient) (aznetworkv2.Subnet, error) {
	response, err := subnetsClient.Get(ctx, pn.VnetResourceGroup, pn.VnetName, pn.SubnetName, nil)
	var respErr *azcore.ResponseError
	if err != nil {
		if errors.As(err, &respErr) && !(respErr.RawResponse.StatusCode == http.StatusNotFound) {
			return aznetworkv2.Subnet{}, fmt.Errorf("error while looking up subnet: %v", err)
		}

		if respErr.RawResponse.StatusCode == http.StatusNotFound && pn.SubnetCIDR == "" {
			return aznetworkv2.Subnet{}, fmt.Errorf("subnet '%s' is not found in vnet '%s' in resource group '%s' and subscription '%s' and subnet CIDR is not specified", pn.SubnetName, pn.VnetName, pn.VnetResourceGroup, pn.VnetSubscriptionID)
		}
	}
	return response.Subnet, nil
}

func (pn *ProviderNetwork) GetSubnetClient(ctx context.Context, azConfig *auth.Config) (*aznetworkv2.SubnetsClient, error) {
	logger := log.G(ctx).WithField("method", "GetSubnetClient")
	ctx, span := trace.StartSpan(ctx, "network.GetSubnetClient")
	defer span.End()

	logger.Debug("getting azure credential")

	var err error
	var credential azcore.TokenCredential
	isUserIdentity := len(azConfig.AuthConfig.ClientID) == 0

	if isUserIdentity {
		credential, err = azConfig.GetMSICredential(ctx)
	} else {
		credential, err = azConfig.GetSPCredential(ctx)
	}
	if err != nil {
		return nil, errors.Wrap(err, "an error has occurred while creating getting credential ")
	}

	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: azConfig.Cloud,
		},
	}

	subnetsClient, err := aznetworkv2.NewSubnetsClient(azConfig.AuthConfig.SubscriptionID, credential, &options)
	if err != nil {
		return nil, errors.Wrap(err, "an error has occurred while creating subnet client")
	}
	return subnetsClient, nil
}

func (pn *ProviderNetwork) CreateOrUpdateACISubnet(ctx context.Context, subnetsClient *aznetworkv2.SubnetsClient, isCreate bool) error {
	logger := log.G(ctx).WithField("method", "CreateOrUpdateACISubnet")
	ctx, span := trace.StartSpan(ctx, "network.CreateOrUpdateACISubnet")
	defer span.End()

	action := "updating"

	subnet := aznetworkv2.Subnet{
		Name: &pn.SubnetName,
		Properties: &aznetworkv2.SubnetPropertiesFormat{
			Delegations: []*aznetworkv2.Delegation{
				{
					Name: &delegationName,
					Properties: &aznetworkv2.ServiceDelegationPropertiesFormat{
						ServiceName: &serviceName,
						Actions:     []*string{&subnetAction},
					},
				},
			},
		},
	}

	// only set the address prefix and prefixes if we are creating a new subnet
	if isCreate {
		subnet.Properties.AddressPrefix = &pn.SubnetCIDR
		subnet.Properties.AddressPrefixes = []*string{
			&pn.SubnetCIDR,
		}
		action = "creating"
	}

	logger.Debugf("%s subnet %s", action, *subnet.Name)

	var rawResponse *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &rawResponse)

	poller, err := subnetsClient.BeginCreateOrUpdate(ctxWithResp, pn.VnetResourceGroup, pn.VnetName, pn.SubnetName, subnet, nil)
	if err != nil {
		return fmt.Errorf("error %s subnet: %v", action, err)
	}
	_, err = poller.PollUntilDone(context.TODO(), nil)
	if err != nil {
		return fmt.Errorf("error %s subnet: %v", action, err)
	}

	updatedAction := action[:len(action)-3] + "ed"
	logger.Debugf("new subnet %s has been %s successfully. vnet %s, response code %d", pn.SubnetName, updatedAction, pn.VnetName, rawResponse.StatusCode)
	logger.Infof("new subnet %s has been %s successfully", pn.SubnetName, updatedAction)
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
