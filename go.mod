module github.com/virtual-kubelet/azure-aci

go 1.16

require (
	contrib.go.opencensus.io/exporter/ocagent v0.4.12
	github.com/Azure/azure-sdk-for-go v43.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.0
	github.com/Azure/go-autorest/autorest/adal v0.9.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.0
	github.com/BurntSushi/toml v0.3.1
	github.com/dimchansky/utfbom v1.1.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/virtual-kubelet/node-cli v0.6.1
	github.com/virtual-kubelet/virtual-kubelet v1.5.1-0.20210903190255-5fe8a7d0008d
	go.opencensus.io v0.22.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.19.10
	k8s.io/apimachinery v0.19.10
	k8s.io/client-go v0.19.10
	k8s.io/kubernetes v1.19.10
)

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.10

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.10

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.10

replace k8s.io/apiserver => k8s.io/apiserver v0.19.10

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.10

replace k8s.io/cri-api => k8s.io/cri-api v0.19.10

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.10

replace k8s.io/kubelet => k8s.io/kubelet v0.19.10

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.10

replace k8s.io/apimachinery => k8s.io/apimachinery v0.19.10

replace k8s.io/api => k8s.io/api v0.19.10

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.10

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.10

replace k8s.io/component-base => k8s.io/component-base v0.19.10

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.10

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.10

replace k8s.io/metrics => k8s.io/metrics v0.19.10

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.10

replace k8s.io/code-generator => k8s.io/code-generator v0.19.10

replace k8s.io/client-go => k8s.io/client-go v0.19.10

replace k8s.io/kubectl => k8s.io/kubectl v0.19.10
