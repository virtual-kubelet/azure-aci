package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
				assert.Contains(t, *aciDNSConfig.NameServers, kubeDNSIP, "test [%d]", i)
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
