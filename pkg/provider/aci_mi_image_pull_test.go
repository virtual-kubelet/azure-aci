package provider

import (
	//"context"
	//"encoding/base64"
	"fmt"
	"testing"

	//azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	//"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	//is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/util/intstr"
)

// TODO:
// func TestGetAgentPoolMI
// func TestSetContainerGroupId
// func TestGetImageRegistryCreds
// func TestCreatePodWithACRImage

// method:
// for each test set variables
// create test cases
// define mock server, and mock create assertions
// for each test case define a mockListener, resourceManager, and Provider
// use provider to create pods
// validate any expected errors
func TestGetImageServerNames(t *testing.T) {
	podName         := "pod-" + uuid.New().String()
	podNamespace    := "ns-" + uuid.New().String()
	containerName	:= "mi-image-pull-container"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cases := []struct {
		description     string
		imageNames		[]string
		expectedLength	int
	}{
		{
			description: "string image name with azurecr io",
			imageNames: []string{
				"fakename.azurecr.io/fakeimage:faketag",
				"fakename2.azurecr.io/fakeimage:faketag",
			},
			expectedLength: 2,

		},
		{
			description: "alphanumeric image name with azurecr.io",
			imageNames: []string{"123fakename456.azurecr.io/fakerepo/fakeimage:faketag"},
			expectedLength: 1,
		},
		{
			description: "image name without azurecr.io",
			imageNames: []string{
				"fakerepo/fakeimage:faketag",
				"fakerepo2/fakeimage2:faketag",
			},
			expectedLength: 0,
		},
		{
			description: "image name with and without azurecr.io",
			imageNames: []string{
				"fakerepo.azurecr.io/fakeimage:faketag",
				"fakerepo2/fakeimage2:faketag",
			},
			expectedLength: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			// pod spec definition with container image names
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{},
				},
			}
			for i, imageName := range tc.imageNames {
				pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{
					Image: imageName,
					Name: fmt.Sprintf("%s-%d", containerName, i),
				})
			}

			// create new provider
			resourceManager, err := manager.NewResourceManager(
				NewMockPodLister(mockCtrl),
			    NewMockSecretLister(mockCtrl),
				NewMockConfigMapLister(mockCtrl),
				NewMockServiceLister(mockCtrl),
				NewMockPersistentVolumeClaimLister(mockCtrl),
				NewMockPersistentVolumeLister(mockCtrl))
			if err != nil {
				t.Fatal("Unable to prepare mocks for resourceManager", err)
			}

			aciMocks := createNewACIMock()
			provider, err := createTestProvider(aciMocks, resourceManager)
			if err != nil {
				t.Fatal("Unable to create test provider", err)
			}

			serverNames := provider.getImageServerNames(pod)
			assert.Equal(t, tc.expectedLength, len(serverNames))
		})
	}
}
