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
	ccePolicyString	   := "cGFja2FnZSBwb2xpY3kKCmFwaV9zdm4gOj0gIjAuOS4wIgoKaW1wb3J0IGZ1dHVyZS5rZXl3b3Jkcy5ldmVyeQppbXBvcnQgZnV0dXJlLmtleXdvcmRzLmluCgpmcmFnbWVudHMgOj0gWwpdCgpjb250YWluZXJzIDo9IFsKICAgIHsKICAgICAgICAiY29tbWFuZCI6IFsiL3BhdXNlIl0sCiAgICAgICAgImVudl9ydWxlcyI6IFt7InBhdHRlcm4iOiAiUEFUSD0vdXNyL2xvY2FsL3NiaW46L3Vzci9sb2NhbC9iaW46L3Vzci9zYmluOi91c3IvYmluOi9zYmluOi9iaW4iLCAic3RyYXRlZ3kiOiAic3RyaW5nIiwgInJlcXVpcmVkIjogdHJ1ZX0seyJwYXR0ZXJuIjogIlRFUk09eHRlcm0iLCAic3RyYXRlZ3kiOiAic3RyaW5nIiwgInJlcXVpcmVkIjogZmFsc2V9XSwKICAgICAgICAibGF5ZXJzIjogWyIxNmI1MTQwNTdhMDZhZDY2NWY5MmMwMjg2M2FjYTA3NGZkNTk3NmM3NTVkMjZiZmYxNjM2NTI5OTE2OWU4NDE1Il0sCiAgICAgICAgIm1vdW50cyI6IFtdLAogICAgICAgICJleGVjX3Byb2Nlc3NlcyI6IFtdLAogICAgICAgICJzaWduYWxzIjogW10sCiAgICAgICAgImFsbG93X2VsZXZhdGVkIjogZmFsc2UsCiAgICAgICAgIndvcmtpbmdfZGlyIjogIi8iCiAgICB9LApdCmFsbG93X3Byb3BlcnRpZXNfYWNjZXNzIDo9IHRydWUKYWxsb3dfZHVtcF9zdGFja3MgOj0gdHJ1ZQphbGxvd19ydW50aW1lX2xvZ2dpbmcgOj0gdHJ1ZQphbGxvd19lbnZpcm9ubWVudF92YXJpYWJsZV9kcm9wcGluZyA6PSB0cnVlCmFsbG93X3VuZW5jcnlwdGVkX3NjcmF0Y2ggOj0gdHJ1ZQoKCm1vdW50X2RldmljZSA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQp1bm1vdW50X2RldmljZSA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQptb3VudF9vdmVybGF5IDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CnVubW91bnRfb3ZlcmxheSA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQpjcmVhdGVfY29udGFpbmVyIDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CmV4ZWNfaW5fY29udGFpbmVyIDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CmV4ZWNfZXh0ZXJuYWwgOj0geyAiYWxsb3dlZCIgOiB0cnVlIH0Kc2h1dGRvd25fY29udGFpbmVyIDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CnNpZ25hbF9jb250YWluZXJfcHJvY2VzcyA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQpwbGFuOV9tb3VudCA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQpwbGFuOV91bm1vdW50IDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CmdldF9wcm9wZXJ0aWVzIDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CmR1bXBfc3RhY2tzIDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CnJ1bnRpbWVfbG9nZ2luZyA6PSB7ICJhbGxvd2VkIiA6IHRydWUgfQpsb2FkX2ZyYWdtZW50IDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CnNjcmF0Y2hfbW91bnQgOj0geyAiYWxsb3dlZCIgOiB0cnVlIH0Kc2NyYXRjaF91bm1vdW50IDo9IHsgImFsbG93ZWQiIDogdHJ1ZSB9CnJlYXNvbiA6PSB7ImVycm9ycyI6IGRhdGEuZnJhbWV3b3JrLmVycm9yc30K"
	mockCtrl           := gomock.NewController(t)
	defer mockCtrl.Finish()
	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *azaciv2.ContainerGroup) error {
		containers := cg.Properties.Containers
		initContainers := cg.Properties.InitContainers
		confidentialComputeProperties := cg.Properties.ConfidentialComputeProperties
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, initContainers != nil, "Container group is nil")
		if (len(initContainers) > 0) {
			assert.Check(t, is.Equal(len(containers), 2), "2 Containers are expected")
			assert.Check(t, is.Equal(len(initContainers), 1), "2 init containers are expected")
			assert.Check(t, initContainers[0].Properties.VolumeMounts != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].Properties.EnvironmentVariables != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].Properties.Command != nil, "Command mount should be present")
			assert.Check(t, initContainers[0].Properties.Image != nil, "Image should be present")
			assert.Check(t, *initContainers[0].Name == initContainerName1, "Name should be correct")
		}
		if (confidentialComputeProperties != nil) {
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
