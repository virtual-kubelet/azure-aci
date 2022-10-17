package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/virtual-kubelet/virtual-kubelet/log"
)

// aksCredential represents the credential file for AKS
type aksCredential struct {
	Cloud                  string `json:"cloud"`
	TenantID               string `json:"tenantId"`
	SubscriptionID         string `json:"subscriptionId"`
	ClientID               string `json:"aadClientId"`
	ClientSecret           string `json:"aadClientSecret"`
	ResourceGroup          string `json:"resourceGroup"`
	Region                 string `json:"location"`
	VNetName               string `json:"vnetName"`
	VNetResourceGroup      string `json:"vnetResourceGroup"`
	UserAssignedIdentityID string `json:"userAssignedIdentityID"`
}

// NewAKSCredential returns an aksCredential struct from file path
func NewAKSCredential(p string) (*aksCredential, error) {
	logger := log.G(context.TODO()).WithField("method", "NewAKSCredential").WithField("file", p)
	logger.Debug("Reading AKS credential file")

	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("reading AKS credential file %q failed: %v", p, err)
	}

	// Unmarshal the authentication file.
	var cred aksCredential
	if err := json.Unmarshal(b, &cred); err != nil {
		return nil, err
	}

	logger.Debug("Load AKS credential file successfully")
	return &cred, nil
}
