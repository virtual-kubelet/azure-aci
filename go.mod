module github.com/virtual-kubelet/azure-aci

require (
	github.com/Azure/azure-sdk-for-go v26.0.0+incompatible
	github.com/Azure/go-autorest v11.5.0+incompatible
	github.com/BurntSushi/toml v0.3.1
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/dimchansky/utfbom v1.1.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/uuid v1.1.0
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.0
	github.com/pkg/errors v0.8.1
	github.com/sirupsen/logrus v1.4.2
	github.com/virtual-kubelet/node-cli v0.1.2-0.20190808213126-cd8af9b9bc8c
	github.com/virtual-kubelet/virtual-kubelet v1.1.0
	go.opencensus.io v0.21.0
	golang.org/x/sync v0.0.0-20190227155943-e225da77a7e6
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/apiserver v0.0.0-20190805142138-368b2058237c // indirect
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/kubernetes v1.15.2
	k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a // indirect
)

replace k8s.io/api => k8s.io/api v0.0.0-20190805141119-fdd30b57c827

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190805141520-2fe0317bcee0

replace k8s.io/kubernetes => k8s.io/kubernetes v1.14.3
