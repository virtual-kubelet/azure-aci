/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getACIResourceMetaFromContainerGroup(cg *azaci.ContainerGroup) (*string, metav1.Time) {
	// Use the Provisioning State if it's not Succeeded,
	// otherwise use the state of the instance.
	aciState := cg.ContainerGroupProperties.ProvisioningState
	if *aciState == "Succeeded" {
		aciState = cg.ContainerGroupProperties.InstanceView.State
	}

	var creationTime metav1.Time
	ts := *cg.Tags["CreationTimestamp"]

	if ts != "" {
		t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", ts)
		if err == nil {
			creationTime = metav1.NewTime(t)
		}
	}

	return aciState, creationTime
}

func getContainerID(cgID, containerName string) string {
	if cgID == "" {
		return ""
	}

	containerResourceID := fmt.Sprintf("%s/containers/%s", cgID, containerName)

	h := sha256.New()
	if _, err := h.Write([]byte(strings.ToUpper(containerResourceID))); err != nil {
		panic(err)
	}
	hashBytes := h.Sum(nil)
	return fmt.Sprintf("aci://%s", hex.EncodeToString(hashBytes))
}
