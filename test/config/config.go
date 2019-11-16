package config

import "os"

type Config struct {
	Location         string
	ResourceGroup    string
	NetworkProfileID string
}

const (
	defaultLocation       = "westus"
	defaultResourceGroup  = "virtual-kubelet-tests"
	defaultNetworkProfile = "/subscriptions/ae43b1e3-c35d-4c8c-bc0d-f148b4c52b78/resourceGroups/aci-connector/providers/Microsoft.Network/networkprofiles/aci-connector-network-profile-westus"
)

// New creates a default test config
func New() Config {
	return Config{
		Location:      defaultLocation,
		ResourceGroup: defaultResourceGroup,
	}
}

// FromEnv creates a test config from env vars using defaults when env vars are not set.
//
// Supported env vars are:
//    ACI_TEST_REGION
//    ACI_TEST_RG
//    ACI_TEST_NETWORK_PROFILE
func FromEnv() Config {
	cfg := New()

	if l := os.Getenv("ACI_TEST_REGION"); l != "" {
		cfg.Location = l
	}

	if rg := os.Getenv("ACI_TEST_RG"); rg != "" {
		cfg.ResourceGroup = rg
	}

	if np := os.Getenv("ACI_TEST_NETWORK_PROFILE"); np != "" {
		cfg.NetworkProfileID = np
	}

	return cfg
}
