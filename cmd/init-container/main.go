package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/network"
	"github.com/virtual-kubelet/azure-aci/pkg/provider"
	cli "github.com/virtual-kubelet/node-cli"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
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

	setupBackoff := wait.Backoff{
		Steps:    10,
		Duration: 10 * time.Millisecond,
		Factor:   0,
		Jitter:   0.1,
	}

	retry.OnError(setupBackoff,
		func(err error) bool { return true },
		func() error {
			azConfig := auth.Config{}

			if vkVersion {
				//Setup config
				err = azConfig.SetAuthConfig(ctx)
				if err != nil {
					log.G(ctx).Fatalf("init container: cannot setup the auth configuration ", err)
				}
			}
			p := provider.ACIProvider{
				ProviderNetwork: network.ProviderNetwork{},
			}

			// Check or set up a network for VK
			log.G(ctx).Info("init container: setting up the network configuration")
			err = p.ProviderNetwork.SetVNETConfig(ctx, &azConfig)
			if err != nil {
				log.G(ctx).Fatalf("init container: cannot setup the VNet configuration ", err)
			}

			return nil
		})
	log.G(ctx).Info("initial setup for virtual kubelet Azure ACI is successful")
}