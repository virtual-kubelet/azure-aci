/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/network"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	logger := logrus.StandardLogger()
	log.L = logruslogger.FromLogrus(logrus.NewEntry(logger))

	log.G(ctx).Debug("Init container started")

	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("NAMESPACE")

	if podName == "" || podNamespace == "" {
		log.G(ctx).Fatal("an error has occurred while retrieve the pod info ")
	}

	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.G(ctx).Fatal("an error has occurred while creating client ", err)
	}

	kubeClient := kubernetes.NewForConfigOrDie(config)
	eventBroadcast := util.NewRecorder(ctx, kubeClient)
	defer eventBroadcast.Shutdown()

	recorder := eventBroadcast.NewRecorder(scheme.Scheme, v1.EventSource{Component: "virtual kubelet"})

	setupBackoff := wait.Backoff{
		Steps:    50,
		Duration: time.Minute,
		Factor:   0,
		Jitter:   0.01,
	}
	azConfig := auth.Config{}

	//Setup config
	err = azConfig.SetAuthConfig(ctx)
	if err != nil {
		log.G(ctx).Fatalf("cannot setup the auth configuration. Retrying, ", err)
	}

	err = retry.OnError(setupBackoff,
		func(err error) bool {
			return true
		}, func() error {
			var providerNetwork network.ProviderNetwork
			providerNetwork.Client = network.ProviderNetworkImpl{ProviderNetwork: &providerNetwork}
			if azConfig.AKSCredential != nil {
				providerNetwork.VnetName = azConfig.AKSCredential.VNetName
				if azConfig.AKSCredential.VNetResourceGroup != "" {
					providerNetwork.VnetResourceGroup = azConfig.AKSCredential.VNetResourceGroup
				} else {
					providerNetwork.VnetResourceGroup = azConfig.AKSCredential.ResourceGroup
				}
			}
			// Check or set up a network for VK
			log.G(ctx).Debug("setting up the network configuration")
			err = providerNetwork.SetVNETConfig(ctx, &azConfig)
			if err != nil {
				log.G(ctx).Errorf("cannot setup the VNet configuration. Retrying", err)
				return err
			}
			return nil
		})

	if err != nil {
		recorder.Eventf(&v1.ObjectReference{
			Kind:      "Pod",
			Name:      podName,
			Namespace: podNamespace,
		}, v1.EventTypeWarning, "InitFailed", "VNet config setup failed")
		log.G(ctx).Fatal("cannot setup the VNet configuration ", err)
	}
	recorder.Eventf(&v1.ObjectReference{
		Kind:      "Pod",
		Name:      podName,
		Namespace: podNamespace,
	}, v1.EventTypeNormal, "InitSuccess", "initial setup for virtual kubelet Azure ACI is successful")
	log.G(ctx).Info("initial setup for virtual kubelet Azure ACI is successful")
}
