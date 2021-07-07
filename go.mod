module github.com/virtual-kubelet/azure-aci

go 1.13

require (
	contrib.go.opencensus.io/exporter/ocagent v0.7.0
	github.com/Azure/azure-sdk-for-go v35.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.0
	github.com/Azure/go-autorest/autorest/adal v0.9.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.0
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/cpuguy83/dockercfg v0.3.0
	github.com/dimchansky/utfbom v1.1.0
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/virtual-kubelet/virtual-kubelet v0.0.0-00010101000000-000000000000
	go.opencensus.io v0.22.3
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.19.10
	k8s.io/apimachinery v0.19.10
	k8s.io/client-go v0.19.10
	k8s.io/klog v1.0.0
)

replace github.com/virtual-kubelet/virtual-kubelet => github.com/cpuguy83/virtual-kubelet v1.6.0-dev-nodemanager-0
