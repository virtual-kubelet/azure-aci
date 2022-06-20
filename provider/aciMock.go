package provider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gorilla/mux"
	"github.com/virtual-kubelet/azure-aci/client/aci"
)

// ACIMock implements a Azure Container Instance mock server.
type ACIMock struct {
	server               *httptest.Server
	OnCreate             func(string, string, string, *aci.ContainerGroup) (int, interface{})
	OnGetContainerGroups func(string, string) (int, interface{})
	OnGetContainerGroup  func(string, string, string) (int, interface{})
	OnGetRPManifest      func() (int, interface{})
}

const (
	containerGroupsRoute  = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroup}/providers/Microsoft.ContainerInstance/containerGroups"
	containerGroupRoute   = containerGroupsRoute + "/{containerGroup}"
	resourceProviderRoute = "/providers/Microsoft.ContainerInstance"
	aksClustersListURLRoute = "/subscriptions/{{.subscriptionId}}/resourceGroups/{{.resourceGroup}}/providers/Microsoft.ContainerService/managedClusters"
)

// NewACIMock creates a new Azure Container Instance mock server.
func NewACIMock() *ACIMock {
	mock := new(ACIMock)
	mock.start()

	return mock
}

// Start the Azure Container Instance mock service.
func (mock *ACIMock) start() {
	if mock.server != nil {
		return
	}

	router := mux.NewRouter()
	router.HandleFunc(
		containerGroupRoute,
		func(w http.ResponseWriter, r *http.Request) {
			subscription := mux.Vars(r)["subscriptionId"]
			resourceGroup := mux.Vars(r)["resourceGroup"]
			containerGroup := mux.Vars(r)["containerGroup"]

			var cg aci.ContainerGroup
			if err := json.NewDecoder(r.Body).Decode(&cg); err != nil {
				panic(err)
			}

			if mock.OnCreate != nil {
				statusCode, response := mock.OnCreate(subscription, resourceGroup, containerGroup, &cg)
				w.WriteHeader(statusCode)
				b := new(bytes.Buffer)
				if err := json.NewEncoder(b).Encode(response); err != nil {
					panic(err)
				}
				if _, err := w.Write(b.Bytes()); err != nil {
					panic(err)
				}
				return
			}

			w.WriteHeader(http.StatusNotImplemented)
		}).Methods("PUT")

	router.HandleFunc(
		containerGroupRoute,
		func(w http.ResponseWriter, r *http.Request) {
			subscription := mux.Vars(r)["subscriptionId"]
			resourceGroup := mux.Vars(r)["resourceGroup"]
			containerGroup := mux.Vars(r)["containerGroup"]

			if mock.OnGetContainerGroup != nil {
				statusCode, response := mock.OnGetContainerGroup(subscription, resourceGroup, containerGroup)
				w.WriteHeader(statusCode)
				b := new(bytes.Buffer)
				if err := json.NewEncoder(b).Encode(response); err != nil {
					panic(err)
				}

				if _, err := w.Write(b.Bytes()); err != nil {
					panic(err)
				}
				return
			}

			w.WriteHeader(http.StatusNotImplemented)
		}).Methods("GET")

	router.HandleFunc(
		containerGroupsRoute,
		func(w http.ResponseWriter, r *http.Request) {
			subscription := mux.Vars(r)["subscriptionId"]
			resourceGroup := mux.Vars(r)["resourceGroup"]

			if mock.OnGetContainerGroups != nil {
				statusCode, response := mock.OnGetContainerGroups(subscription, resourceGroup)
				w.WriteHeader(statusCode)
				b := new(bytes.Buffer)
				if err := json.NewEncoder(b).Encode(response); err != nil {
					panic(err)
				}
				if _, err := w.Write(b.Bytes()); err != nil {
					panic(err)
				}

				return
			}

			w.WriteHeader(http.StatusNotImplemented)
		}).Methods("GET")

	router.HandleFunc(
		resourceProviderRoute,
		func(w http.ResponseWriter, r *http.Request) {
			if mock.OnGetRPManifest != nil {
				statusCode, response := mock.OnGetRPManifest()
				w.WriteHeader(statusCode)
				b := new(bytes.Buffer)
				if err := json.NewEncoder(b).Encode(response); err != nil {
					panic(err)
				}
				if _, err := w.Write(b.Bytes()); err != nil {
					panic(err)
				}

				return
			}

			w.WriteHeader(http.StatusNotImplemented)
		}).Methods("GET")

	router.HandleFunc(
		aksClustersListURLRoute,
		func(w http.ResponseWriter, r *http.Request) {
			statusCode := 200
			response := &aci.AKSClusterListResult{
				Value: []aci.AKSCluster{
					aci.AKSCluster{
						Properties: aci.AKSClusterPropertiesTruncated{
							Fqdn: "fake.cluster.uri",
						},
					},
				},
			}
			w.WriteHeader(statusCode)
			b := new(bytes.Buffer)
			if err := json.NewEncoder(b).Encode(response); err != nil {
				panic(err)
			}
			if _, err := w.Write(b.Bytes()); err != nil {
				panic(err)
			}

		}).Methods("GET")
	mock.server = httptest.NewServer(router)
}

// GetServerURL returns the mock server URL.
func (mock *ACIMock) GetServerURL() string {
	if mock.server != nil {
		return mock.server.URL
	}

	panic("Mock server is not initialized.")
}

// Close terminates the Azure Container Instance mock server.
func (mock *ACIMock) Close() {
	if mock.server != nil {
		mock.server.Close()
		mock.server = nil
	}
}
