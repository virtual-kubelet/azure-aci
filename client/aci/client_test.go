package aci

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/google/uuid"
	azure "github.com/virtual-kubelet/azure-aci/client"
	"github.com/virtual-kubelet/azure-aci/client/api"
	"github.com/virtual-kubelet/azure-aci/client/resourcegroups"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/b3"
)

var (
	client                   *Client
	location                 = "westus"
	resourceGroup            = "virtual-kubelet-tests"
	containerGroup           = "virtual-kubelet-test-container-group"
	subscriptionID           string
	testUserIdentityClientId = "97c70c2a-fa56-4b70-95b5-1c67ca26f383"
)

func init() {
	//Create a resource group name with uuid.
	uid := uuid.New()
	resourceGroup += "-" + uid.String()[0:6]
}

// The TestMain function creates a resource group for testing
// and deletes in when it's done.
func TestMain(m *testing.M) {
	auth, err := azure.NewAuthenticationFromFile(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		log.Fatalf("Failed to load Azure authentication file: %v", err)
	}

	subscriptionID = auth.SubscriptionID

	// Check if the resource group exists and create it if not.
	rgCli, err := resourcegroups.NewClient(auth, "unit-test")
	if err != nil {
		log.Fatalf("creating new resourcegroups client failed: %v", err)
	}

	// Check if the resource group exists.
	exists, err := rgCli.ResourceGroupExists(resourceGroup)
	if err != nil {
		log.Fatalf("checking if resource group exists failed: %v", err)
	}

	if !exists {
		// Create the resource group.
		_, err := rgCli.CreateResourceGroup(resourceGroup, resourcegroups.Group{
			Location: location,
		})
		if err != nil {
			log.Fatalf("creating resource group failed: %v", err)
		}
	}

	// initialize client in Main
	c, err := NewClient(auth, "unit-test")
	if err != nil {
		log.Fatal(err)
	}
	client = c

	// Run the tests.
	merr := m.Run()

	// Delete the resource group.
	if err := rgCli.DeleteResourceGroup(resourceGroup); err != nil {
		log.Printf("Couldn't delete resource group %q: %v", resourceGroup, err)

	}

	if merr != 0 {
		os.Exit(merr)
	}

	os.Exit(0)
}

func TestNewClient(t *testing.T) {
	auth, err := azure.NewAuthenticationFromFile(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		log.Fatalf("Failed to load Azure authentication file: %v", err)
	}

	c, err := NewClient(auth, "unit-test")
	if err != nil {
		t.Fatal(err)
	}

	client = c
}

func TestNewMsiClient(t *testing.T) {
	auth, err := azure.NewAuthenticationFromFile(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		log.Fatalf("Failed to load Azure authentication file: %v", err)
	}

	auth.UserIdentityClientId = testUserIdentityClientId
	auth.UseUserIdentity = true

	c, err := azure.NewClient(auth, []string{"test-client"})
	if err != nil {
		t.Fatal(err)
	}

	hc := c.HTTPClient
	hc.Transport = &ochttp.Transport{
		Base:           hc.Transport,
		Propagation:    &b3.HTTPFormat{},
		NewClientTrace: ochttp.NewSpanAnnotatingClientTrace,
	}

	restClient := &Client{hc: hc, auth: auth}

	s := mocks.NewSender()
	ds := adal.DecorateSender(s,
		(func() adal.SendDecorator {
			return func(s adal.Sender) adal.Sender {
				return adal.SenderFunc(func(r *http.Request) (*http.Response, error) {
					expectedRefreshQuery := fmt.Sprintf(
						"http://169.254.169.254/metadata/identity/oauth2/token?api-version=%v&client_id=%v",
						"2018-02-01",
						testUserIdentityClientId)

					if !strings.HasPrefix(r.URL.String(), expectedRefreshQuery) {
						t.Fatal("token not requested through msi endpoint or client id is not matching")
					}

					resp := mocks.NewResponseWithBodyAndStatus(mocks.NewBody("{}"), http.StatusOK, "OK")
					return resp, nil
				})
			}
		})())

	c.SetTokenProviderTestSender(ds)

	err = restClient.DeleteContainerGroup(context.Background(), resourceGroup, containerGroup)
	if err != nil {
		// Expected as no proper response is sent back.
		return
	}
}

func TestCreateContainerGroupFails(t *testing.T) {
	_, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroup, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected create container group to fail with ResourceRequestsNotSpecified, but returned nil")
	}

	if !strings.Contains(err.Error(), "ResourceRequestsNotSpecified") {
		t.Fatalf("expected ResourceRequestsNotSpecified to be in the error message but got: %v", err)
	}
}

func TestCreateContainerGroupWithoutResourceLimit(t *testing.T) {
	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroup, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroup {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroup)
	}

	if err := client.DeleteContainerGroup(context.Background(), resourceGroup, containerGroup); err != nil {
		t.Fatal(err)
	}
}

func TestCreateContainerGroup(t *testing.T) {
	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroup, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroup {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroup)
	}
}

func TestCreateContainerGroupWithBadVNetFails(t *testing.T) {
	_, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroup, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
			NetworkProfile: &containerinstance.ContainerGroupNetworkProfile{
				ID: to.StringPtr(fmt.Sprintf(
					"/subscriptions/%s/resourceGroups/%s/providers"+
						"/Microsoft.Network/networkProfiles/%s",
					subscriptionID,
					resourceGroup,
					"badNetworkProfile",
				)),
			},
		},
	})
	if err == nil {
		t.Fatal("expected create container group to fail with  NetworkProfileNotFound, but returned nil")
	}
	if !strings.Contains(err.Error(), "NetworkProfileNotFound") {
		t.Fatalf("expected NetworkProfileNotFound to be in the error message but got: %v", err)
	}
}

func TestGetContainerGroup(t *testing.T) {
	cg, err := client.GetContainerGroup(context.Background(), resourceGroup, containerGroup)
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroup {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroup)
	}
}

func TestListContainerGroup(t *testing.T) {
	list, err := client.ListContainerGroups(context.Background(), resourceGroup)
	if err != nil {
		t.Fatal(err)
	}
	for _, cg := range *list.Value {
		if *cg.Name != containerGroup {
			t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroup)
		}
	}
}

func TestCreateContainerGroupWithLivenessProbe(t *testing.T) {
	uid := uuid.New()
	containerGroupName := containerGroup + "-" + uid.String()[0:6]
	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroupName, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
						LivenessProbe: &containerinstance.ContainerProbe{
							HTTPGet: &containerinstance.ContainerHTTPGet{
								Port: to.Int32Ptr(80),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroupName {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroupName)
	}
}

func TestCreateContainerGroupFailsWithLivenessProbeMissingPort(t *testing.T) {
	uid := uuid.New()
	containerGroupName := containerGroup + "-" + uid.String()[0:6]
	_, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroupName, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
						LivenessProbe: &containerinstance.ContainerProbe{
							HTTPGet: &containerinstance.ContainerHTTPGet{
								Path: to.StringPtr("/"),
							},
						},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected failure")
	}
}

func TestCreateContainerGroupWithReadinessProbe(t *testing.T) {
	uid := uuid.New()
	containerGroupName := containerGroup + "-" + uid.String()[0:6]
	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroupName, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
						ReadinessProbe: &containerinstance.ContainerProbe{
							HTTPGet: &containerinstance.ContainerHTTPGet{
								Port: to.Int32Ptr(80),
								Path: to.StringPtr("/"),
							},
							InitialDelaySeconds: to.Int32Ptr(5),
							SuccessThreshold:    to.Int32Ptr(3),
							FailureThreshold:    to.Int32Ptr(5),
							TimeoutSeconds:      to.Int32Ptr(120),
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroupName {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroupName)
	}
}

func TestCreateContainerGroupWithLogAnalytics(t *testing.T) {
	diagnostics, err := NewContainerGroupDiagnosticsFromFile(os.Getenv("LOG_ANALYTICS_AUTH_LOCATION"))
	if err != nil {
		t.Fatal(err)
	}
	cgname := "cgla"
	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, cgname, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
			Diagnostics: diagnostics,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != cgname {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, cgname)
	}
	if err := client.DeleteContainerGroup(context.Background(), resourceGroup, cgname); err != nil {
		t.Fatalf("Delete Container Group failed: %s", err.Error())
	}
}

func TestCreateContainerGroupWithInvalidLogAnalytics(t *testing.T) {
	law := &containerinstance.LogAnalytics{}
	_, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroup, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
			Diagnostics: &containerinstance.ContainerGroupDiagnostics{
				LogAnalytics: law,
			},
		},
	})
	if err == nil {
		t.Fatal("TestCreateContainerGroupWithInvalidLogAnalytics should fail but encountered no errors")
	}
}

func TestCreateContainerGroupWithVNet(t *testing.T) {
	uid := uuid.New()
	containerGroupName := containerGroup + "-" + uid.String()[0:6]
	//fakeKubeConfig := base64.StdEncoding.EncodeToString([]byte(uid.String()))
	networkProfileID := "/subscriptions/da28f5e5-aa45-46fe-90c8-053ca49ab4b5/resourceGroups/virtual-kubelet-tests/providers/Microsoft.Network/networkProfiles/aci-network-profile-virtual-kubelet-tests-vnet-aci-connector"
	diagnostics, err := NewContainerGroupDiagnosticsFromFile(os.Getenv("LOG_ANALYTICS_AUTH_LOCATION"))
	if err != nil {
		t.Fatal(err)
	}

	diagnostics.LogAnalytics.LogType = containerinstance.ContainerInsights

	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroupName, containerinstance.ContainerGroup{
		Location: &location,
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
							},
						},
					},
				},
			},
			NetworkProfile: &containerinstance.ContainerGroupNetworkProfile{
				ID: &networkProfileID,
			},
			// TODO(gossion): what is this extension?
			// Extensions: []*Extension{
			// 	&Extension{
			// 		Name: "kube-proxy",
			// 		Properties: &ExtensionProperties{
			// 			Type:    ExtensionTypeKubeProxy,
			// 			Version: ExtensionVersion1_0,
			// 			Settings: map[string]string{
			// 				KubeProxyExtensionSettingClusterCIDR: "10.240.0.0/16",
			// 				KubeProxyExtensionSettingKubeVersion: KubeProxyExtensionKubeVersion,
			// 			},
			// 			ProtectedSettings: map[string]string{
			// 				KubeProxyExtensionSettingKubeConfig: fakeKubeConfig,
			// 			},
			// 		},
			// 	},
			// },
			DNSConfig: &containerinstance.DNSConfiguration{
				NameServers: &[]string{"1.1.1.1"},
			},
			Diagnostics: diagnostics,
		},
	})

	if err != nil {
		t.Fatal(err)
	}
	if *cg.Name != containerGroupName {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroupName)
	}
	if err := client.DeleteContainerGroup(context.Background(), resourceGroup, containerGroupName); err != nil {
		t.Fatalf("Delete Container Group failed: %s", err.Error())
	}
}

func TestCreateContainerGroupWithGPU(t *testing.T) {
	uid := uuid.New()
	containerGroupName := containerGroup + "-" + uid.String()[0:6]

	cg, err := client.CreateContainerGroup(context.Background(), resourceGroup, containerGroupName, containerinstance.ContainerGroup{
		Location: to.StringPtr("eastus"),
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			OsType: containerinstance.Linux,
			Containers: &[]containerinstance.Container{
				{
					Name: to.StringPtr("nginx"),
					ContainerProperties: &containerinstance.ContainerProperties{
						Image:   to.StringPtr("nginx"),
						Command: &[]string{"nginx", "-g", "daemon off;"},
						Ports: &[]containerinstance.ContainerPort{
							{
								Protocol: containerinstance.ContainerNetworkProtocolTCP,
								Port:     to.Int32Ptr(80),
							},
						},
						Resources: &containerinstance.ResourceRequirements{
							Requests: &containerinstance.ResourceRequests{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
								Gpu: &containerinstance.GpuResource{
									Count: to.Int32Ptr(1),
									Sku:   containerinstance.K80,
								},
							},
							Limits: &containerinstance.ResourceLimits{
								CPU:        to.Float64Ptr(1),
								MemoryInGB: to.Float64Ptr(1),
								Gpu: &containerinstance.GpuResource{
									Count: to.Int32Ptr(1),
									Sku:   containerinstance.K80,
								},
							},
						},
					},
				},
			},
		},
	})

	if err != nil {
		if apierr, ok := err.(*api.Error); ok && apierr.StatusCode == 409 {
			t.Skip("Skip GPU test case since it often failed for ACI's GPU capacity")
		}
		t.Fatal(err)
	}
	if *cg.Name != containerGroupName {
		t.Fatalf("resource group name is %s, expected %s", *cg.Name, containerGroupName)
	}
	if err := client.DeleteContainerGroup(context.Background(), resourceGroup, containerGroupName); err != nil {
		t.Fatalf("Delete Container Group failed: %s", err.Error())
	}
}

func TestDeleteContainerGroup(t *testing.T) {
	err := client.DeleteContainerGroup(context.Background(), resourceGroup, containerGroup)
	if err != nil {
		t.Fatal(err)
	}
}
