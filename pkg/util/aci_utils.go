/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	v1 "k8s.io/api/core/v1"
)

var (
	// LogTypeContainerInsights to prevent indirect pointer access
	LogTypeContainerInsights = azaciv2.LogAnalyticsLogTypeContainerInsights
	// ContainerNetworkProtocolTCP to prevent indirect pointer access
	ContainerNetworkProtocolTCP = azaciv2.ContainerNetworkProtocolTCP
	//ContainerNetworkProtocolUDP to prevent indirect pointer access
	ContainerNetworkProtocolUDP = azaciv2.ContainerNetworkProtocolUDP
	// ContainerGroupIPAddressTypePublic to prevent indirect pointer access
	ContainerGroupIPAddressTypePublic = azaciv2.ContainerGroupIPAddressTypePublic
	// ContainerGroupNetworkProtocolTCP to prevent indirect pointer access
	ContainerGroupNetworkProtocolTCP = azaciv2.ContainerGroupNetworkProtocolTCP
)

func GetContainerID(cgID, containerName *string) string {
	if cgID == nil {
		return ""
	}

	containerResourceID := fmt.Sprintf("%s/containers/%s", *cgID, *containerName)

	h := sha256.New()
	if _, err := h.Write([]byte(strings.ToUpper(containerResourceID))); err != nil {
		panic(err)
	}
	hashBytes := h.Sum(nil)
	return fmt.Sprintf("aci://%s", hex.EncodeToString(hashBytes))
}

func OmitDuplicates(strs []string) []string {
	uniqueStrs := make(map[string]bool)

	var ret []string
	for _, str := range strs {
		if !uniqueStrs[str] {
			ret = append(ret, str)
			uniqueStrs[str] = true
		}
	}
	return ret
}

func GetProtocol(pro v1.Protocol) *azaciv2.ContainerNetworkProtocol {
	switch pro {
	case v1.ProtocolUDP:
		return &ContainerNetworkProtocolUDP
	default:
		return &ContainerNetworkProtocolTCP
	}
}
