package main

import (
	"github.com/virtual-kubelet/azure-aci"
	"github.com/virtual-kubelet/virtual-kubelet/providers"
)

func registerACI(s *providers.Store) error {
	return s.Register("azure", func(cfg providers.InitConfig) (providers.Provider, error) {
		return azure.NewACIProvider(cfg.ConfigPath, cfg.ResourceManager, cfg.NodeName, cfg.OperatingSystem, cfg.InternalIP, cfg.DaemonPort)
	})
}
