package network

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	aznetworkv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	v1 "k8s.io/api/core/v1"
)

func TestGetDNSConfig(t *testing.T) {
	kubeDNSIP := "10.0.0.10"
	clusterDomain := "fakeClusterDomain"
	podName := "pod-" + uuid.New().String()
	podNamespace := "ns-" + uuid.New().String()

	testCases := []struct {
		desc                    string
		prepPodFunc             func(p *v1.Pod)
		kubeDNSIP               bool
		shouldHaveClusterDomain bool
	}{
		{
			desc: fmt.Sprint("Pod with DNSPolicy == ", v1.DNSClusterFirst, "with DNSConfig"),
			prepPodFunc: func(p *v1.Pod) {
				p.Spec.DNSPolicy = v1.DNSClusterFirst
				p.Spec.DNSConfig = &v1.PodDNSConfig{
					Nameservers: []string{"clusterFirstNS"},
					Searches:    []string{"clusterFirstSearches"},
				}
			},
			kubeDNSIP:               true,
			shouldHaveClusterDomain: true,
		},
		{
			desc: fmt.Sprint("Pod with DNSPolicy == ", v1.DNSClusterFirstWithHostNet, "with DNSConfig"),
			prepPodFunc: func(p *v1.Pod) {
				p.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
				p.Spec.DNSConfig = &v1.PodDNSConfig{
					Nameservers: []string{"clusterFirstWithHostNettNS"},
					Searches:    []string{"clusterFirstWithHostNetSearches"},
				}
			},
			kubeDNSIP:               true,
			shouldHaveClusterDomain: true,
		},
		{
			desc: "Pod with other valid DNSPolicy and DNSConfig",
			prepPodFunc: func(p *v1.Pod) {
				p.Spec.DNSPolicy = v1.DNSDefault
				p.Spec.DNSConfig = &v1.PodDNSConfig{
					Nameservers: []string{"defaultNS"},
					Searches:    []string{"defaultSearches"},
				}
			},
			kubeDNSIP:               false,
			shouldHaveClusterDomain: false,
		},
	}
	for i, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.TODO()
			testPod := testsutil.CreatePodObj(podName, podNamespace)
			tc.prepPodFunc(testPod)
			aciDNSConfig := getDNSConfig(ctx, testPod, kubeDNSIP, clusterDomain)

			if tc.kubeDNSIP {
				assert.Contains(t, aciDNSConfig.NameServers, &kubeDNSIP, "test [%d]", i)
			}
			if tc.shouldHaveClusterDomain {
				assert.Contains(t, *aciDNSConfig.SearchDomains, clusterDomain, "test [%d]", i)
			}
		})
	}
}

func TestFormDNSSearchFitsLimits(t *testing.T) {
	testCases := []struct {
		desc              string
		hostNames         []string
		resultSearch      []string
		expandedDNSConfig bool
	}{
		{
			desc:         "3 search paths",
			hostNames:    []string{"testNS.svc.TEST", "svc.TEST", "TEST"},
			resultSearch: []string{"testNS.svc.TEST", "svc.TEST", "TEST"},
		},
		{
			desc:         fmt.Sprint("5 search paths will get omitted to the max (", maxDNSNameservers, ")"),
			hostNames:    []string{"testNS.svc.TEST", "svc.TEST", "TEST", "AA", "BB"},
			resultSearch: []string{"testNS.svc.TEST", "svc.TEST", "TEST"},
		},
	}

	for i, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := context.TODO()
			dnsSearch := formDNSNameserversFitsLimits(ctx, tc.hostNames)
			assert.EqualValues(t, tc.resultSearch, dnsSearch, "test [%d]", i)
		})
	}
}

// https://github.com/kubernetes/kubernetes/blob/4276ed36282405d026d8072e0ebed4f1da49070d/pkg/kubelet/network/dns/dns_test.go#L246
func TestFormDNSNameserversFitsLimits(t *testing.T) {
	testCases := []struct {
		desc               string
		nameservers        []string
		expectedNameserver []string
	}{
		{
			desc:               "valid: 1 nameserver",
			nameservers:        []string{"127.0.0.1"},
			expectedNameserver: []string{"127.0.0.1"},
		},
		{
			desc:               "valid: 3 nameservers",
			nameservers:        []string{"127.0.0.1", "10.0.0.10", "8.8.8.8"},
			expectedNameserver: []string{"127.0.0.1", "10.0.0.10", "8.8.8.8"},
		},
		{
			desc:               "invalid: 4 nameservers, trimmed to 3",
			nameservers:        []string{"127.0.0.1", "10.0.0.10", "8.8.8.8", "1.2.3.4"},
			expectedNameserver: []string{"127.0.0.1", "10.0.0.10", "8.8.8.8"},
		},
	}

	for _, tc := range testCases {
		ctx := context.TODO()
		appliedNameservers := formDNSNameserversFitsLimits(ctx, tc.nameservers)
		assert.EqualValues(t, tc.expectedNameserver, appliedNameservers, tc.desc)
	}
}

func TestShouldCreateSubnet(t *testing.T) {
	subnetName := "fakeSubnet"
	fakeAddPrefix := "10.00.0/16"
	providerSubnetCIDR := "10.00.0/17"
	subnetDelegationService := "Microsoft.ContainerInstance/containerGroups"
	fakeResourceType := "fakeResourceType"

	fakeServiceAssotiationLinks := []*aznetworkv2.ServiceAssociationLink{
		{
			Properties: &aznetworkv2.ServiceAssociationLinkPropertiesFormat{
				LinkedResourceType: &fakeResourceType,
			},
		}}

	currentSubnet := aznetworkv2.Subnet{
		Name: &subnetName,
	}

	pn := ProviderNetwork{
		SubnetName: subnetName,
	}

	cases := []struct {
		description        string
		providerSubnetCIDR string
		subnetProperties   aznetworkv2.SubnetPropertiesFormat
		expectedError      error
		expectedAssertions func(result bool) bool
	}{
		{
			description:        "can create a subnet because all the checks pass",
			providerSubnetCIDR: "",
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix: &fakeAddPrefix,
			},
			expectedAssertions: func(result bool) bool {
				return assert.Equal(t, result, true, "subnet should be created")
			},
		},
		{
			description:        "doesn't create a subnet because subnet is already linked to Microsoft.ContainerInstance/containerGroups",
			providerSubnetCIDR: "",
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix: &fakeAddPrefix,
				ServiceAssociationLinks: []*aznetworkv2.ServiceAssociationLink{
					{
						Properties: &aznetworkv2.ServiceAssociationLinkPropertiesFormat{
							LinkedResourceType: &subnetDelegationService,
						},
					}},
			},
			expectedAssertions: func(result bool) bool {
				return assert.Equal(t, result, false, "subnet should not be created because subnet already linked to Microsoft.ContainerInstance/containerGroups")
			},
		},
		{
			description:        "doesn't create a subnet because subnet is being delegated to Microsoft.ContainerInstance/containerGroups",
			providerSubnetCIDR: "",
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix: &fakeAddPrefix,
				Delegations: []*aznetworkv2.Delegation{
					{
						Properties: &aznetworkv2.ServiceDelegationPropertiesFormat{
							ServiceName: &subnetDelegationService,
						},
					}},
			},
			expectedAssertions: func(result bool) bool {
				return assert.Equal(t, result, false, "subnet should not be created because subnet is being delegated to Microsoft.ContainerInstance/containerGroups")
			},
		},
		{
			description:        "cannot create a subnet because Microsoft.ContainerInstance/containerGroups can't be delegated to the subnet",
			providerSubnetCIDR: "",
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix:           &fakeAddPrefix,
				ServiceAssociationLinks: fakeServiceAssotiationLinks,
			},
			expectedError: fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance as it is used by other Azure resource: '%v'", pn.SubnetName, fakeServiceAssotiationLinks[0]),
		},
		{
			description:        "cannot create subnet because current subnet references a route table",
			providerSubnetCIDR: "",
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix: &fakeAddPrefix,
				RouteTable: &aznetworkv2.RouteTable{
					ID: &subnetName,
				},
			},
			expectedError: fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance since it references the route table '%s'", pn.SubnetName, subnetName),
		}, {
			description:        "cannot create subnet because provider subnet CIDR does not match with desired subnet",
			providerSubnetCIDR: providerSubnetCIDR,
			subnetProperties: aznetworkv2.SubnetPropertiesFormat{
				AddressPrefix: &fakeAddPrefix,
			},
			expectedError: fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", pn.SubnetName, fakeAddPrefix, providerSubnetCIDR),
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			pn.SubnetCIDR = tc.providerSubnetCIDR
			currentSubnet.Properties = &tc.subnetProperties

			result, err := pn.shouldCreateSubnet(currentSubnet, true)

			if tc.expectedError != nil {
				assert.Equal(t, err.Error(), tc.expectedError.Error(), "Error messages should match")
				assert.Equal(t, result, false, "subnet should not be created")
			} else {
				assert.Equal(t, err, nil, "no error should be returned")
				assert.Equal(t, tc.expectedAssertions(result), true, "Expected assertions should pass")
			}
		})
	}

}

func TestValidateNetworkConfig(t *testing.T) {
	azConfig := auth.Config{}
	err := azConfig.SetAuthConfig(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	pn := &ProviderNetwork{}

	cases := []struct {
		description   string
		setEnvVar     func()
		expectedError error
	}{
		{
			description:   "ACI vnet name env variable is not set",
			setEnvVar:     func() {},
			expectedError: errors.New("vnet name can not be empty please set ACI_VNET_NAME"),
		},
		{
			description: "ACI vnet resource group env variable is not set",
			setEnvVar: func() {
				os.Setenv("ACI_VNET_SUBSCRIPTION_ID", "111111-222-3333-4444-555555")
				os.Setenv("ACI_VNET_NAME", "fakeVnet")
			},
			expectedError: errors.New("vnet resourceGroup can not be empty please set ACI_VNET_RESOURCE_GROUP"),
		},
		{
			description: "ACI subnet CIDR env variable is set but subnet name is missing",
			setEnvVar: func() {
				os.Setenv("ACI_VNET_RESOURCE_GROUP", "fakeRG")
				os.Setenv("ACI_SUBNET_CIDR", "10.00.0/16")
			},
			expectedError: errors.New("subnet CIDR defined but no subnet name, subnet name is required to set a subnet CIDR"),
		},
		{
			description: "ACI subnet CIDR env variable is set but it is malformed",
			setEnvVar: func() {
				os.Setenv("ACI_SUBNET_NAME", "fakeSubnet")
			},
			expectedError: errors.New("error parsing provided subnet CIDR: invalid CIDR address: 10.00.0/16"),
		},
		{
			description: "all environmental variables are set as expected",
			setEnvVar: func() {
				os.Setenv("ACI_SUBNET_CIDR", "127.0.0.1/24")
			},
			expectedError: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			tc.setEnvVar()
			err := pn.validateNetworkConfig(context.Background(), &azConfig)

			if tc.expectedError != nil {
				assert.Equal(t, err.Error(), tc.expectedError.Error(), "Error messages should match")
			} else {
				assert.Equal(t, pn.VnetSubscriptionID, os.Getenv("ACI_VNET_SUBSCRIPTION_ID"))
				assert.Equal(t, pn.VnetName, os.Getenv("ACI_VNET_NAME"))
				assert.Equal(t, pn.VnetResourceGroup, os.Getenv("ACI_VNET_RESOURCE_GROUP"))
				assert.Equal(t, pn.SubnetName, os.Getenv("ACI_SUBNET_NAME"))
				assert.Equal(t, pn.SubnetCIDR, os.Getenv("ACI_SUBNET_CIDR"))
			}
		})
	}
}
