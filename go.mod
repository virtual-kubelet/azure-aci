module github.com/virtual-kubelet/azure-aci

go 1.16

require (
	contrib.go.opencensus.io/exporter/ocagent v0.4.12
	github.com/Azure/azure-sdk-for-go v63.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.20
	github.com/Azure/go-autorest/autorest/adal v0.9.20
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.0
	github.com/Azure/go-autorest/autorest/mocks v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/dimchansky/utfbom v1.1.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/thoas/go-funk v0.9.1
	github.com/virtual-kubelet/node-cli v0.7.0
	github.com/virtual-kubelet/virtual-kubelet v1.6.0
	go.opencensus.io v0.22.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.19.10
	k8s.io/apimachinery v0.19.10
	k8s.io/client-go v0.19.10
)

// fix cve
replace github.com/dgrijalva/jwt-go v3.2.0+incompatible => github.com/golang-jwt/jwt/v4 v4.1.0
