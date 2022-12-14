package main

import (
	"context"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/network"
	"github.com/virtual-kubelet/azure-aci/pkg/provider"
	cli "github.com/virtual-kubelet/node-cli"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
)

func main() {
	logger := logrus.StandardLogger()
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))

	ctx := cli.ContextWithCancelOnSignal(context.Background())

	vkVersion, err := strconv.ParseBool(os.Getenv("USE_VK_VERSION_2"))
	if err != nil {
		log.G(ctx).Warn("cannot get USE_VK_VERSION_2 environment variable, the provider will use VK version 1. Skipping init container checks")
		return
	}

	azConfig := auth.Config{}

	if vkVersion {
		//Setup config
		err = azConfig.SetAuthConfig()
		if err != nil {
			log.G(ctx).Fatal(err)
		}
	}
	p := provider.ACIProvider{
		ProviderNetwork: network.ProviderNetwork{},
	}
	// Check or set up a network for VK
	err = p.ProviderNetwork.SetVNETConfig(ctx, &azConfig)
	if err != nil {

	}
}
