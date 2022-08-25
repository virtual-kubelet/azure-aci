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
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	azprovider "github.com/virtual-kubelet/azure-aci/provider"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

var (
	buildVersion = "N/A"
	k8sVersion   = "v1.19.10" // This should follow the version of k8s.io/client-go we are importing
)

func main() {
	klogFlags := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlags)

	prog := filepath.Base(os.Args[0])
	desc := prog + " implements a node on a Kubernetes cluster using Azure Container Instances to run pods."

	var (
		taintKey    = envOrDefault("VKUBELET_TAINT_KEY", "virtual-kubelet.io/provider")
		taintEffect = envOrDefault("VKUBELET_TAINT_EFFECT", string(v1.TaintEffectNoSchedule))
		taintValue  = envOrDefault("VKUBELET_TAINT_VALUE", "azure")

		logLevel        = "info"
		traceSampleRate string

		// for aci
		kubeconfigPath     = os.Getenv("KUBECONFIG")
		ProviderConfigPath string
		clusterDomain      = "cluster.local"
		startupTimeout     time.Duration
		disableTaint       bool
		operatingSystem    = "Linux"
		numberOfWorkers    = 50
		resync             time.Duration

		certPath       = os.Getenv("APISERVER_CERT_LOCATION")
		keyPath        = os.Getenv("APISERVER_KEY_LOCATION")
		clientCACert   = os.Getenv("APISERVER_CA_CERT_LOCATION")
		clientNoVerify bool

		webhookAuth                  bool
		webhookAuthnCacheTTL         time.Duration
		webhookAuthzUnauthedCacheTTL time.Duration
		webhookAuthzAuthedCacheTTL   time.Duration
		nodeName                     string
		listenPort                   = 10250

		// deprecated
		namespace   string
		metricsAddr string
		provider    string
		leases      bool
	)

	if kubeconfigPath == "" {
		home, err := homedir.Dir()
		if err != nil || home == "" {
			return
		}
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	if kPort := os.Getenv("KUBELET_PORT"); kPort != "" {
		var err error
		listenPort, err = strconv.Atoi(kPort)
		if err != nil {
			return
		}
	}

	mux := http.NewServeMux()

	withProvider := func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
		p, err := azprovider.NewACIProvider(ProviderConfigPath, cfg, operatingSystem, nodeName, os.Getenv("VKUBELET_POD_IP"), clusterDomain, int32(listenPort))
		if err != nil {
			return nil, nil, err
		}

		cfg.Node.Status.Capacity = p.Capacity()
		cfg.Node.Status.Allocatable = p.Capacity()
		cfg.Node.Status.Conditions = p.NodeConditions()
		cfg.Node.Status.Addresses = p.NodeAddresses()
		cfg.Node.Status.DaemonEndpoints = p.NodeDaemonEndpoints()
		cfg.Node.Status.NodeInfo.KubeletVersion = strings.Join([]string{k8sVersion, "vk-azure-aci", buildVersion}, "-")

		return p, p, err
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

	withWebhookAuth := func(cfg *nodeutil.NodeConfig) error {
		if !webhookAuth {
			return nil
		}
		auth, err := nodeutil.WebhookAuth(cfg.Client, nodeName,
			func(cfg *nodeutil.WebhookAuthConfig) error {
				if webhookAuthnCacheTTL > 0 {
					cfg.AuthnConfig.CacheTTL = webhookAuthnCacheTTL
				}
				if webhookAuthzAuthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzAuthedCacheTTL
				}
				if webhookAuthzUnauthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzUnauthedCacheTTL
				}
				return nil
			})
		if err != nil {
			return err
		}
		cfg.Handler = nodeutil.WithAuth(auth, mux)

		return nil
	}

	withClient := func(cfg *nodeutil.NodeConfig) error {
		cfg.Handler = mux
		client, err := nodeutil.ClientsetFromEnv(kubeconfigPath)
		if err != nil {
			return err
		}
		return nodeutil.WithClient(client)(cfg)
	}

	withAzNodeConfig := func(cfg *nodeutil.NodeConfig) error {
		cfg.KubeconfigPath = kubeconfigPath
		cfg.InformerResyncPeriod = resync
		cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH

		cfg.NodeSpec.Status.NodeInfo.OperatingSystem = operatingSystem
		cfg.NodeSpec.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
		cfg.NodeSpec.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"

		// Virtual node would be skipped for cloud provider operations (e.g. CP should not add route).
		cfg.NodeSpec.ObjectMeta.Labels["kubernetes.azure.com/managed"] = "false"
		cfg.NumWorkers = numberOfWorkers
		cfg.HTTPListenAddr = fmt.Sprintf(":%d", listenPort)
		cfg.DebugHTTP = true

		return nil
	}

	withTLSConfig := func(cfg *nodeutil.NodeConfig) error {
		var (
			caPool     *x509.CertPool
			clientAuth = tls.RequestClientCert
		)
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("error loading tls certs : %w", err)
		}

		if clientCACert != "" {
			caPool = x509.NewCertPool()
			pem, err := ioutil.ReadFile(clientCACert)
			if err != nil {
				return fmt.Errorf("error reading ca cert pem: %w", err)
			}

			if !caPool.AppendCertsFromPEM(pem) {
				return fmt.Errorf("error appending ca cert to certificate pool")
			}
		}

		cfg.TLSConfig = &tls.Config{
			Certificates:             []tls.Certificate{cert},
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
			CipherSuites:             nodeutil.DefaultServerCiphers(),
			ClientAuth:               clientAuth,
			ClientCAs:                caPool,
		}
		return nil
	}
	run := func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		node, err := nodeutil.NewNode(nodeName,
			withProvider,
			withClient,
			withTaint,
			withAzNodeConfig,
			withTLSConfig,
			withWebhookAuth,
			nodeutil.AttachProviderRoutes(mux),
		)
		if err != nil {
			return err
		}

		if err := configureTracing(nodeName, traceSampleRate); err != nil {
			return err
		}
		ctx = log.WithLogger(ctx, log.G(ctx).WithFields(log.Fields{
			"provider":        provider,
			"operatingSystem": operatingSystem,
			"node":            nodeName,
		}))

		go node.Run(ctx)

		defer func() {
			log.G(ctx).Debug("Waiting for node to be done")
			cancel()
			<-node.Done()
		}()

		if err := node.WaitReady(ctx, startupTimeout); err != nil {
			return fmt.Errorf("error waiting for node to be ready: %w", err)
		}

		log.G(ctx).Info("Node is ready")

		select {
		case <-ctx.Done():
		case <-node.Done():
			return node.Err()
		}
		return nil
	}

	cmd := &cobra.Command{
		Use:   prog,
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
	klogFlags.VisitAll(func(f *flag.Flag) {
		f.Name = "klog." + f.Name
		flags.AddGoFlag(f)
	})

	flags.StringVar(&nodeName, "nodename", nodeName, "kubernetes node name")
	flags.StringVar(&ProviderConfigPath, "provider-config", ProviderConfigPath, "cloud provider configuration file")
	flags.StringVar(&clusterDomain, "cluster-domain", clusterDomain, "kubernetes cluster-domain")
	flags.BoolVar(&disableTaint, "disable-taint", disableTaint, "disable the node taint")
	flags.StringVar(&operatingSystem, "os", operatingSystem, "Operating System (Linux/Windows)")
	flags.IntVar(&numberOfWorkers, "pod-sync-workers", numberOfWorkers, `set the number of pod synchronization workers`)
	flags.DurationVar(&resync, "full-resync-period", resync, "how often to perform a full resync of pods between kubernetes and the provider")
	flags.StringVar(&logLevel, "log-level", logLevel, "log level")

	flags.StringVar(&clientCACert, "client-verify-ca", clientCACert, "CA cert to use to verify client requests")
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

	ctx, cancel := BaseContext(context.Background())
	defer cancel()

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
