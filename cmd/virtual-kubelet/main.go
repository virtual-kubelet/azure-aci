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
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	azproviderv2 "github.com/virtual-kubelet/azure-aci/pkg/provider"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"
)

var (
	buildVersion = "N/A"
	k8sVersion   = "v1.25.0" // This should follow the version of k8s.io we are importing

	taintKey    = envOrDefault("VKUBELET_TAINT_KEY", "virtual-kubelet.io/provider")
	taintEffect = envOrDefault("VKUBELET_TAINT_EFFECT", string(v1.TaintEffectNoSchedule))
	taintValue  = envOrDefault("VKUBELET_TAINT_VALUE", "azure")

	logLevel        = "info"
	traceSampleRate string

	// for aci
	kubeConfigPath  = os.Getenv("KUBECONFIG")
	cfgPath         string
	clusterDomain   = "cluster.local"
	startupTimeout  time.Duration
	disableTaint    bool
	operatingSystem = "Linux"
	numberOfWorkers = 50
	resync          time.Duration

	certPath       = os.Getenv("APISERVER_CERT_LOCATION")
	keyPath        = os.Getenv("APISERVER_KEY_LOCATION")
	clientCACert   string
	clientNoVerify bool

	webhookAuth                  bool
	webhookAuthnCacheTTL         time.Duration
	webhookAuthzUnauthedCacheTTL time.Duration
	webhookAuthzAuthedCacheTTL   time.Duration
	nodeName                     = "vk-aci-test-aks"
	listenPort                   = 10250

	// deprecated
	namespace   string
	metricsAddr string
	leases      bool
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	binaryName := filepath.Base(os.Args[0])
	desc := binaryName + " implements a node on a Kubernetes cluster using Azure Container Instances to run pods."

	var azACIAPIs *client.AzClientsAPIs
	azConfig := auth.Config{}

	var provider string
	//Setup config
	err := azConfig.SetAuthConfig(ctx)
	if err != nil {
		log.G(ctx).Fatal(err)
	}

	azACIAPIs, err = client.NewAzClientsAPIs(ctx, azConfig)
	if err != nil {
		log.G(ctx).Fatal(err)
	}

	if kubeConfigPath == "" {
		home, _ := homedir.Dir()
		if home != "" {
			kubeConfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	withTaint := func(cfg *nodeutil.NodeConfig) error {
		if disableTaint {
			return nil
		}

		taint := v1.Taint{
			Key:   taintKey,
			Value: taintValue,
		}
		switch taintEffect {
		case "NoSchedule":
			taint.Effect = v1.TaintEffectNoSchedule
		case "NoExecute":
			taint.Effect = v1.TaintEffectNoExecute
		case "PreferNoSchedule":
			taint.Effect = v1.TaintEffectPreferNoSchedule
		default:
			return errdefs.InvalidInputf("taint effect %q is not supported", taintEffect)
		}
		cfg.NodeSpec.Spec.Taints = append(cfg.NodeSpec.Spec.Taints, taint)
		return nil
	}
	withVersion := func(cfg *nodeutil.NodeConfig) error {
		cfg.NodeSpec.Status.NodeInfo.KubeletVersion = strings.Join([]string{k8sVersion, "vk-azure-aci", buildVersion}, "-")
		return nil
	}
	configureRoutes := func(cfg *nodeutil.NodeConfig) error {
		mux := http.NewServeMux()
		cfg.Handler = mux
		return nodeutil.AttachProviderRoutes(mux)(cfg)
	}
	withWebhookAuth := func(cfg *nodeutil.NodeConfig) error {
		if !webhookAuth {
			cfg.Handler = api.InstrumentHandler(nodeutil.WithAuth(nodeutil.NoAuth(), cfg.Handler))
			return nil
		}

		auth, err := nodeutil.WebhookAuth(cfg.Client, nodeName,
			func(cfg *nodeutil.WebhookAuthConfig) error {
				var err error

				cfg.AuthzConfig.WebhookRetryBackoff = options.DefaultAuthWebhookRetryBackoff()

				if webhookAuthnCacheTTL > 0 {
					cfg.AuthnConfig.CacheTTL = webhookAuthnCacheTTL
				}
				if webhookAuthzAuthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzAuthedCacheTTL
				}
				if webhookAuthzUnauthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzUnauthedCacheTTL
				}
				if clientCACert != "" {
					ca, err := dynamiccertificates.NewDynamicCAContentFromFile("client-ca", clientCACert)
					if err != nil {
						return err
					}
					cfg.AuthnConfig.ClientCertificateCAContentProvider = ca
					go ca.Run(ctx, 1)
				}
				return err
			})

		if err != nil {
			return err
		}
		cfg.TLSConfig.ClientAuth = tls.RequestClientCert
		cfg.Handler = api.InstrumentHandler(nodeutil.WithAuth(auth, cfg.Handler))
		return nil
	}

	withCA := func(cfg *tls.Config) error {
		if clientCACert == "" {
			return nil
		}
		if err := nodeutil.WithCAFromPath(clientCACert)(cfg); err != nil {
			return fmt.Errorf("error getting CA from path: %w", err)
		}
		if clientNoVerify {
			cfg.ClientAuth = tls.NoClientCert
		}
		return nil
	}
	k8sClient, err := nodeutil.ClientsetFromEnv(kubeConfigPath)
	if err != nil {
		log.G(ctx).Fatal(err)
	}
	withClient := func(cfg *nodeutil.NodeConfig) error {
		return nodeutil.WithClient(k8sClient)(cfg)
	}

	run := func(ctx context.Context) error {
		if err := configureTracing(nodeName, traceSampleRate); err != nil {
			return err
		}

		node, err := nodeutil.NewNode(nodeName,
			func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
				if port := os.Getenv("KUBELET_PORT"); port != "" {
					kubeletPort, err := strconv.ParseInt(port, 10, 32)
					if err != nil {
						return nil, nil, err
					}
					listenPort = int(kubeletPort)
				}
				p, err := azproviderv2.NewACIProvider(ctx, cfgPath, azConfig, azACIAPIs, cfg,
					nodeName, operatingSystem, os.Getenv("VKUBELET_POD_IP"),
					int32(listenPort), clusterDomain, k8sClient)
				if err != nil {
					return nil, nil, err
				}
				p.ConfigureNode(ctx, cfg.Node)
				return p, nil, err
			},
			withClient,
			withTaint,
			withVersion,
			nodeutil.WithTLSConfig(nodeutil.WithKeyPairFromPath(certPath, keyPath), withCA),
			withWebhookAuth,
			configureRoutes,
			func(cfg *nodeutil.NodeConfig) error {
				cfg.InformerResyncPeriod = resync
				cfg.NumWorkers = numberOfWorkers
				cfg.HTTPListenAddr = fmt.Sprintf(":%d", listenPort)
				return nil
			},
		)
		if err != nil {
			return err
		}

		go func() error {
			err = node.Run(ctx)
			if err != nil {
				return fmt.Errorf("error running the node: %w", err)
			}
			return nil
		}()

		if err := node.WaitReady(ctx, startupTimeout); err != nil {
			return fmt.Errorf("error waiting for node to be ready: %w", err)
		}

		<-node.Done()
		return node.Err()
	}

	cmd := &cobra.Command{
		Use:   binaryName,
		Short: desc,
		Long:  desc,
		Run: func(cmd *cobra.Command, args []string) {
			logger := logrus.StandardLogger()
			lvl, err := logrus.ParseLevel(logLevel)
			if err != nil {
				logrus.WithError(err).Fatal("Error parsing log level")
			}
			logger.SetLevel(lvl)

			ctx := log.WithLogger(cmd.Context(), logruslogger.FromLogrus(logrus.NewEntry(logger)))

			if err := run(ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.G(ctx).Fatal(err)
				}
				log.G(ctx).Debug(err)
			}
		},
	}
	flags := cmd.Flags()

	klogFlags := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlags)
	klogFlags.VisitAll(func(f *flag.Flag) {
		f.Name = "klog." + f.Name
		flags.AddGoFlag(f)
	})

	flags.StringVar(&nodeName, "nodename", nodeName, "kubernetes node name")
	flags.StringVar(&cfgPath, "provider-config", cfgPath, "cloud provider configuration file")
	flags.StringVar(&clusterDomain, "cluster-domain", clusterDomain, "kubernetes cluster-domain")
	flags.DurationVar(&startupTimeout, "startup-timeout", startupTimeout, "How long to wait for the virtual-kubelet to start")
	flags.BoolVar(&disableTaint, "disable-taint", disableTaint, "disable the node taint")
	flags.StringVar(&operatingSystem, "os", operatingSystem, "Operating System (Linux/Windows)")
	flags.StringVar(&logLevel, "log-level", logLevel, "log level.")
	flags.IntVar(&numberOfWorkers, "pod-sync-workers", numberOfWorkers, `set the number of pod synchronization workers`)
	flags.DurationVar(&resync, "full-resync-period", resync, "how often to perform a full resync of pods between kubernetes and the provider")

	flags.StringVar(&clientCACert, "client-verify-ca", os.Getenv("APISERVER_CA_CERT_LOCATION"), "CA cert to use to verify client requests")
	flags.BoolVar(&clientNoVerify, "no-verify-clients", clientNoVerify, "Do not require client certificate validation")
	flags.BoolVar(&webhookAuth, "authentication-token-webhook", webhookAuth, ""+
		"Use the TokenReview API to determine authentication for bearer tokens.")
	flags.DurationVar(&webhookAuthnCacheTTL, "authentication-token-webhook-cache-ttl", webhookAuthnCacheTTL,
		"The duration to cache responses from the webhook token authenticator.")
	flags.DurationVar(&webhookAuthzAuthedCacheTTL, "authorization-webhook-cache-authorized-ttl", webhookAuthzAuthedCacheTTL,
		"The duration to cache 'authorized' responses from the webhook authorizer.")
	flags.DurationVar(&webhookAuthzUnauthedCacheTTL, "authorization-webhook-cache-unauthorized-ttl", webhookAuthzUnauthedCacheTTL,
		"The duration to cache 'unauthorized' responses from the webhook authorizer.")

	flags.StringVar(&traceSampleRate, "trace-sample-rate", traceSampleRate, "set probability of tracing samples")

	// deprecated flags
	flags.StringVar(&namespace, "namespace", namespace, "set namespace to watch for pods")
	flags.MarkDeprecated("namespace", "cannot set namespace, all namespaces watched")
	flags.MarkHidden("namespace")
	flags.StringVar(&metricsAddr, "metrics-addr", metricsAddr, "address to listen for metrics/stats requests")
	flags.MarkDeprecated("metrics-addr", "metrics are only available on the main api port")
	flags.MarkHidden("metrics-addr")
	flags.StringVar(&provider, "provider", provider, "cloud provider")
	flags.MarkDeprecated("provider", "only one provider is supported")
	flags.MarkHidden("provider")
	flags.BoolVar(&leases, "enable-node-lease", leases, "use node leases for heartbeats")
	flags.MarkDeprecated("leases", "Leases are always enabled")
	flags.MarkHidden("leases")
	flags.StringVar(&taintKey, "taint", taintKey, "Set node taint key")
	flags.MarkDeprecated("taint", "Taint key should now be configured using the VKUBELET_TAINT_KEY environment variable")

	if err := cmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Fatal("Error running command")
		}
	}
}

func envOrDefault(key string, defaultValue string) string {
	v, set := os.LookupEnv(key)
	if set {
		return v
	}
	return defaultValue
}
