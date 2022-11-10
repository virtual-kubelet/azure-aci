// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	azproviderv2 "github.com/virtual-kubelet/azure-aci/pkg/provider"
	azproviderv1 "github.com/virtual-kubelet/azure-aci/provider"
	cli "github.com/virtual-kubelet/node-cli"
	logruscli "github.com/virtual-kubelet/node-cli/logrus"
	opencensuscli "github.com/virtual-kubelet/node-cli/opencensus"
	"github.com/virtual-kubelet/node-cli/opts"
	"github.com/virtual-kubelet/node-cli/provider"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"github.com/virtual-kubelet/virtual-kubelet/trace/opencensus"
)

var (
	buildVersion    = "N/A"
	buildTime       = "N/A"
	k8sVersion      = "v1.19.10" // This should follow the version of k8s.io/client-go we are importing
	numberOfWorkers = 50
)

func main() {
	ctx := cli.ContextWithCancelOnSignal(context.Background())

	logger := logrus.StandardLogger()
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))
	logConfig := &logruscli.Config{LogLevel: "info"}

	trace.T = opencensus.Adapter{}
	traceConfig := opencensuscli.Config{
		AvailableExporters: map[string]opencensuscli.ExporterInitFunc{
			"ocagent": initOCAgent,
		},
	}

	o, err := opts.FromEnv()
	if err != nil {
		log.G(ctx).Fatal(err)
	}
	o.Provider = "azure"
	o.Version = strings.Join([]string{k8sVersion, "vk-azure-aci", buildVersion}, "-")
	o.PodSyncWorkers = numberOfWorkers

	vkVersion, err := strconv.ParseBool(os.Getenv("USE_VK_VERSION_2"))
	if err != nil {
		log.G(ctx).Warn("cannot get USE_VK_VERSION_2 environment variable, the provider will use VK version 1")
		vkVersion = false
	}

	var azACIAPIs *client.AzClientsAPIs
	azConfig := auth.Config{}

	if vkVersion {
		//Setup config
		err = azConfig.SetAuthConfig()
		if err != nil {
			log.G(ctx).Fatal(err)
		}

		azACIAPIs = client.NewAzClientsAPIs(ctx, azConfig)
	}
	run := func(ctx context.Context) error {
		node, err := cli.New(ctx,
			cli.WithBaseOpts(o),
			cli.WithCLIVersion(buildVersion, buildTime),
			cli.WithProvider("azure", func(cfg provider.InitConfig) (provider.Provider, error) {
				if vkVersion {
					return azproviderv2.NewACIProvider(ctx, cfg.ConfigPath, azConfig, azACIAPIs, cfg.ResourceManager, cfg.NodeName, cfg.OperatingSystem, cfg.InternalIP, cfg.DaemonPort, cfg.KubeClusterDomain)
				} else {
					return azproviderv1.NewACIProvider(cfg.ConfigPath, cfg.ResourceManager, cfg.NodeName, cfg.OperatingSystem, cfg.InternalIP, cfg.DaemonPort, cfg.KubeClusterDomain)
				}
			}),
			cli.WithPersistentFlags(logConfig.FlagSet()),
			cli.WithPersistentPreRunCallback(func() error {
				return logruscli.Configure(logConfig, logger)
			}),
			cli.WithPersistentFlags(traceConfig.FlagSet()),
			cli.WithPersistentPreRunCallback(func() error {
				return opencensuscli.Configure(ctx, &traceConfig, o)
			}),
		)
		if err != nil {
			return err
		}

		err = node.Run(ctx)
		if err != nil {
			return err
		}
		return nil
	}

	if err := run(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.G(ctx).Fatal(err)
		}
		log.G(ctx).Debug(err)
	}
}
