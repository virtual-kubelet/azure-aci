package provider

import (
	"fmt"
	"testing"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	armmsi "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestSetContainerGroupIdentity(t *testing.T) {
	fakeIdentityURI	:= "fakeuri"
	fakePrincipalID	:= "fakeprincipalid"
	fakeClientID	:= "fakeClientid"
	armmsiIdentity	:= &armmsi.Identity{
		ID: &fakeIdentityURI,
		Properties: &armmsi.UserAssignedIdentityProperties{
			ClientID: &fakeClientID,
			PrincipalID: &fakePrincipalID,
		},
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cases := []struct {
		description     string
		identity		*armmsi.Identity
		identityType	azaci.ResourceIdentityType
	}{
		{
			description: "identity is nil",
			identity: nil,
			identityType: azaci.ResourceIdentityTypeUserAssigned,
		},
		{
			description: "identity is not nil",
			identity: armmsiIdentity,
			identityType: azaci.ResourceIdentityTypeUserAssigned,

		},
		{
			description: "identity type is not user assignted",
			identity: armmsiIdentity,
			identityType: azaci.ResourceIdentityTypeSystemAssigned,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
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

			testContainerGroup := &client.ContainerGroupWrapper{
				ContainerGroupPropertiesWrapper: &client.ContainerGroupPropertiesWrapper{
					ContainerGroupProperties: &azaci.ContainerGroupProperties{},
				},
			}

			provider.setContainerGroupIdentity(tc.identity, tc.identityType, testContainerGroup)
			if tc.identityType == azaci.ResourceIdentityTypeUserAssigned && tc.identity != nil{
				// identity uri, clientID, principalID should match
				assert.Check(t, testContainerGroup.Identity != nil, "container group identity should be populated")
				assert.Equal(t, testContainerGroup.Identity.Type, azaci.ResourceIdentityTypeUserAssigned, "identity type should match")
				assert.Check(t, testContainerGroup.Identity.UserAssignedIdentities[*tc.identity.ID] != nil , "identity uri should be present in UserAssignedIdenttities")
				assert.Equal(t, testContainerGroup.Identity.UserAssignedIdentities[*tc.identity.ID].PrincipalID, tc.identity.Properties.PrincipalID, "principal id should matc")
				assert.Equal(t, testContainerGroup.Identity.UserAssignedIdentities[*tc.identity.ID].ClientID, tc.identity.Properties.ClientID , "client id should matc")
			} else {
				// identity should not be added
				assert.Check(t, testContainerGroup.Identity == nil, "container group identity should not be populated")
			}
		})
	}
}

func TestGetManagedIdentityImageRegistryCredentials(t *testing.T) {
	fakeIdentityURI	:= "fakeuri"
	fakePrincipalID	:= "fakeprincipalid"
	fakeClientID	:= "fakeClientid"
	fakeImageName	:= "fakeregistry.azurecr.io/fakeimage:faketag"
	fakeImageName2	:= "fakeregistry2.azurecr.io/fakeimage:faketag"
	armmsiIdentity	:= &armmsi.Identity{
		ID: &fakeIdentityURI,
		Properties: &armmsi.UserAssignedIdentityProperties{
			ClientID: &fakeClientID,
			PrincipalID: &fakePrincipalID,
		},
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Image: fakeImageName,
				},
				v1.Container{
					Image: fakeImageName, // duplicate image server
				},
				v1.Container{
					Image: fakeImageName2,
				},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cases := []struct {
		description     string
		identity		*armmsi.Identity
	}{
		{
			description: "identity is nil",
			identity: nil,
		},
		{
			description: "identity is not nil",
			identity: armmsiIdentity,

		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
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

			testContainerGroup := &client.ContainerGroupWrapper{
				ContainerGroupPropertiesWrapper: &client.ContainerGroupPropertiesWrapper{
					ContainerGroupProperties: &azaci.ContainerGroupProperties{},
				},
			}

			creds := provider.getManagedIdentityImageRegistryCredentials(pod, tc.identity, testContainerGroup)
			if tc.identity != nil{
				// image registry credentials should have identity
				assert.Check(t, creds != nil, "image registry creds should be populated")
				assert.Equal(t, len(*creds), 2, "credentials for all distinct acr should be added")
				assert.Equal(t, *(*creds)[0].Identity, *tc.identity.ID, "identity uri should be correct")
				assert.Equal(t, *(*creds)[1].Identity, *tc.identity.ID, "identity uri should be correct")
			} else {
				// identity should not be added to image registry credentials
				assert.Check(t, len(*creds) == 0, "image registry creds should not be populated")

			}
		})
	}
}

// TODO:
// func TestCreatePodWithACRImage
// func TestGetAgentPoolMI