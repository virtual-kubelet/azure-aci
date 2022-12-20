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

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	v1 "k8s.io/api/core/v1"
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

func GetProtocol(pro v1.Protocol) containerinstance.ContainerNetworkProtocol {
	switch pro {
	case v1.ProtocolUDP:
		return containerinstance.ContainerNetworkProtocolUDP
	default:
		return containerinstance.ContainerNetworkProtocolTCP
	}
}
