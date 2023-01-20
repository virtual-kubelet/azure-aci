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

	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	v1 "k8s.io/api/core/v1"
)

var (
	// WindowsType to prevent indirect pointer access
	WindowsType = azaci.OperatingSystemTypesWindows
	// LinuxType to prevent indirect pointer access
	LinuxType = azaci.OperatingSystemTypesLinux
	// LogTypeContainerInsights to prevent indirect pointer access
	LogTypeContainerInsights = azaci.LogAnalyticsLogTypeContainerInsights
	// ContainerNetworkProtocolTCP to prevent indirect pointer access
	ContainerNetworkProtocolTCP = azaci.ContainerNetworkProtocolTCP
	//ContainerNetworkProtocolUDP to prevent indirect pointer access
	ContainerNetworkProtocolUDP = azaci.ContainerNetworkProtocolUDP
	// ContainerGroupIPAddressTypePublic to prevent indirect pointer access
	ContainerGroupIPAddressTypePublic = azaci.ContainerGroupIPAddressTypePublic
	// ContainerGroupNetworkProtocolTCP to prevent indirect pointer access
	ContainerGroupNetworkProtocolTCP = azaci.ContainerGroupNetworkProtocolTCP
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

func GetProtocol(pro v1.Protocol) *azaci.ContainerNetworkProtocol {
	switch pro {
	case v1.ProtocolUDP:
		return &ContainerNetworkProtocolUDP
	default:
		return &ContainerNetworkProtocolTCP
	}
}
