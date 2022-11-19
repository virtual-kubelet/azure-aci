/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"testing"
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	testutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	cgCreationTime, _ = time.Parse(testutil.TimeLayout, time.Now().String())
	cgName            = "testCG"
)

func TestContainerGroupToPodStatus(t *testing.T) {
	startTime := cgCreationTime.Add(time.Second * 3)
	finishTime := startTime.Add(time.Second * 3)

	provider, err := createTestProvider(createNewACIMock(), nil)
	if err != nil {
		t.Fatal("failed to create the test provider", err)
	}
	cases := []struct {
		description           string
		containerGroup        *azaci.ContainerGroup
		expectedPodPhase      v1.PodPhase
		expectedPodConditions []v1.PodCondition
	}{
		{
			description:           "Container is Running/Succeeded",
			containerGroup:        testutil.CreateContainerGroupObj(cgName, cgName, "Succeeded", testutil.CreateACIContainersListObj("Running", "Initializing", startTime, finishTime, false, false, false), "Succeeded"),
			expectedPodPhase:      getPodPhaseFromACIState("Succeeded"),
			expectedPodConditions: testutil.GetPodConditions(metav1.NewTime(cgCreationTime), metav1.NewTime(finishTime), v1.ConditionTrue),
		},
		{
			description:           "Container Failed",
			containerGroup:        testutil.CreateContainerGroupObj(cgName, cgName, "Failed", testutil.CreateACIContainersListObj("Failed", "Running", startTime, finishTime, false, false, false), "Succeeded"),
			expectedPodPhase:      getPodPhaseFromACIState("Failed"),
			expectedPodConditions: []v1.PodCondition{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			expectedStatus, err := provider.getPodStatusFromContainerGroup(tc.containerGroup)
			assert.NilError(t, err, "no errors should be returned")
			assert.Equal(t, tc.expectedPodPhase, expectedStatus.Phase, "Pod phase is not as expected as current container group phase")
			assert.Equal(t, len(tc.expectedPodConditions), len(expectedStatus.Conditions), "Pod conditions are not as expected")
		})
	}
}
