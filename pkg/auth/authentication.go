/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package auth

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"unicode/utf16"

	"github.com/dimchansky/utfbom"
	"github.com/virtual-kubelet/virtual-kubelet/log"
)

// Authentication represents the Authentication file for Azure.
type Authentication struct {
	ClientID             string `json:"clientId,omitempty"`
	ClientSecret         string `json:"clientSecret,omitempty"`
	SubscriptionID       string `json:"subscriptionId,omitempty"`
	TenantID             string `json:"tenantId,omitempty"`
	UserIdentityClientId string `json:"userIdentityClientId,omitempty"`
}

// NewAuthentication returns an Authentication struct from user provided credentials.
func NewAuthentication(clientID, clientSecret, subscriptionID, tenantID, userAssignedIdentityID string) *Authentication {
	return &Authentication{
		ClientID:             clientID,
		ClientSecret:         clientSecret,
		SubscriptionID:       subscriptionID,
		TenantID:             tenantID,
		UserIdentityClientId: userAssignedIdentityID,
	}
}

// newAuthenticationFromFile returns an Authentication struct from file path.
func (a *Authentication) newAuthenticationFromFile(filepath string) error {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("reading Authentication file %q failed: %v", filepath, err)
	}

	// Authentication file might be encoded.
	decoded, err := a.decode(b)
	if err != nil {
		return fmt.Errorf("decoding Authentication file %q failed: %v", filepath, err)
	}

	// Unmarshal the Authentication file.
	if err := json.Unmarshal(decoded, &a); err != nil {
		return err
	}
	return nil
}

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

// newAKSCredential returns an aksCredential struct from file path.
func newAKSCredential(ctx context.Context, filePath string) (*aksCredential, error) {
	logger := log.G(ctx).WithField("method", "newAKSCredential").WithField("file", filePath)
	logger.Debug("Reading AKS credential file")

	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading AKS credential file %q failed: %v", filePath, err)
	}

	// Unmarshal the Authentication file.
	var cred aksCredential
	if err := json.Unmarshal(b, &cred); err != nil {
		return nil, err
	}
	logger.Debug("load AKS credential file successfully")
	return &cred, nil
}

func (a *Authentication) decode(b []byte) ([]byte, error) {
	reader, enc := utfbom.Skip(bytes.NewReader(b))

	switch enc {
	case utfbom.UTF16LittleEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.LittleEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	case utfbom.UTF16BigEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.BigEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	}
	return io.ReadAll(reader)
}
