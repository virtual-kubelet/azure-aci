/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"testing"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/golang/mock/gomock"
	"github.com/virtual-kubelet/azure-aci/pkg/featureflag"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreatePodWithConfidentialComputeProperties(t *testing.T) {

	initContainerName1 := "init-container-1"
	ccePolicyString := "eyJhbGxvd19hbGwiOiB0cnVlLCAiY29udGFpbmVycyI6IHsibGVuZ3RoIjogMCwgImVsZW1lbnRzIjogbnVsbH19"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *azaciv2.ContainerGroup) error {
		containers := cg.Properties.Containers
		initContainers := cg.Properties.InitContainers
		confidentialComputeProperties := cg.Properties.ConfidentialComputeProperties
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, initContainers != nil, "Container group is nil")
		if len(initContainers) > 0 {
			assert.Check(t, is.Equal(len(containers), 2), "2 Containers are expected")
			assert.Check(t, is.Equal(len(initContainers), 1), "2 init containers are expected")
			assert.Check(t, initContainers[0].Properties.VolumeMounts != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].Properties.EnvironmentVariables != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].Properties.Command != nil, "Command mount should be present")
			assert.Check(t, initContainers[0].Properties.Image != nil, "Image should be present")
			assert.Check(t, *initContainers[0].Name == initContainerName1, "Name should be correct")
		}
		if confidentialComputeProperties != nil {
			assert.Check(t, confidentialComputeProperties.CcePolicy != nil, "CCE policy should not be nil")
			assert.Check(t, *confidentialComputeProperties.CcePolicy == ccePolicyString, "CCE policy should match")
		}
		assert.Check(t, *cg.Properties.SKU == azaciv2.ContainerGroupSKUConfidential, "Container group sku should be confidential")

		return nil
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "container-name-01",
					Image: "alpine",
				},
				{
					Name:  "container-name-02",
					Image: "alpine",
				},
			},
		},
	}
	cases := []struct {
		description    string
		initContainers []v1.Container
		annotations    map[string]string
		expectedError  error
	}{
		{
			description:   "create confidential container group with wildcard policy",
			expectedError: nil,
			annotations: map[string]string{
				confidentialComputeSkuLabel: "Confidential",
			},
			initContainers: nil,
		},
		{
			description:   "create confidential container group with specified cce policy",
			expectedError: nil,
			annotations: map[string]string{
				confidentialComputeCcePolicyLabel: ccePolicyString,
			},
			initContainers: nil,
		},
		{
			description:   "create confidential container group with init container",
			expectedError: nil,
			annotations: map[string]string{
				confidentialComputeSkuLabel: "Confidential",
			},
			initContainers: []v1.Container{
				v1.Container{
					Name:  initContainerName1,
					Image: "alpine",
					VolumeMounts: []v1.VolumeMount{
						v1.VolumeMount{
							Name:      "fakeVolumeName",
							MountPath: "/mnt/azure",
						},
					},
					Command: []string{"/bin/bash"},
					Args:    []string{"-c echo test"},
					Env: []v1.EnvVar{
						v1.EnvVar{
							Name:  "TEST_ENV",
							Value: "testvalue",
						},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

			ctx := context.TODO()

			resourceManager, err := manager.NewResourceManager(
				NewMockPodLister(mockCtrl),
				NewMockSecretLister(mockCtrl),
				NewMockConfigMapLister(mockCtrl),
				NewMockServiceLister(mockCtrl),
				NewMockPersistentVolumeClaimLister(mockCtrl),
				NewMockPersistentVolumeLister(mockCtrl))
			if err != nil {
				t.Fatal("Unable to prepare the mocks for resourceManager", err)
			}

			provider, err := createTestProvider(aciMocks, resourceManager)
			if err != nil {
				t.Fatal("Unable to create test provider", err)
			}

			if !provider.enabledFeatures.IsEnabled(ctx, featureflag.InitContainerFeature) {
				t.Skipf("%s feature is not enabled", featureflag.InitContainerFeature)
			}

			pod.Annotations = tc.annotations
			pod.Spec.InitContainers = tc.initContainers
			err = provider.CreatePod(context.Background(), pod)

			// check that the correct error is returned
			if tc.expectedError != nil && err != tc.expectedError {
				assert.Equal(t, tc.expectedError.Error(), err.Error(), "expected error and actual error don't match")
			}
		})
	}
}
