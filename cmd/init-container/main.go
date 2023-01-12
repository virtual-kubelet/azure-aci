package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/network"
	cli "github.com/virtual-kubelet/node-cli"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

func main() {
	logger := logrus.StandardLogger()
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))

	ctx := cli.ContextWithCancelOnSignal(context.Background())

	vkVersion, err := strconv.ParseBool(os.Getenv("USE_VK_VERSION_2"))
	if err != nil {
		log.G(ctx).Warn("init container: cannot get USE_VK_VERSION_2 environment variable, the provider will use VK version 1. Skipping init container checks")
		return
	}

	wait.Until(func() {
		azConfig := auth.Config{}

		if vkVersion {
			//Setup config
			err = azConfig.SetAuthConfig(ctx)
			if err != nil {
				log.G(ctx).Fatalf("init container: cannot setup the auth configuration ", err)
			}
		}

		var providerNetwork network.ProviderNetwork
		// Check or set up a network for VK
		log.G(ctx).Info("init container: setting up the network configuration")
		err = providerNetwork.SetVNETConfig(ctx, &azConfig)
		if err != nil {
			log.G(ctx).Fatalf("init container: cannot setup the VNet configuration ", err)
		}
	},
		1*time.Minute, ctx.Done())

	log.G(ctx).Info("initial setup for virtual kubelet Azure ACI is successful")
}
