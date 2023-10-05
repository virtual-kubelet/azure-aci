package provider

import (
	"fmt"
	"testing"
	"context"
	"strings"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
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

	mockSecretLister := NewMockSecretLister(mockCtrl)

	aciMocks := createNewACIMock()
	provider, err := createTestProvider(aciMocks, NewMockConfigMapLister(mockCtrl),
		mockSecretLister, NewMockPodLister(mockCtrl), nil)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

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

			serverNames := provider.getImageServerNames(pod)
			assert.Equal(t, tc.expectedLength, len(serverNames))
		})
	}
}

func TestSetContainerGroupIdentity(t *testing.T) {
	fakeIdentityURI	 := "fakeuri"
	fakeIdentityURI2 := "fakeuri2"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cases := []struct {
		description     string
		identityList	[]string
		identityType	azaciv2.ResourceIdentityType
	}{
		{
			description: "identity is nil",
			identityList: []string{},
			identityType: azaciv2.ResourceIdentityTypeUserAssigned,
		},
		{
			description: "identity is not nil",
			identityList: []string{fakeIdentityURI},
			identityType: azaciv2.ResourceIdentityTypeUserAssigned,

		},
		{
			description: "identity is not nil",
			identityList: []string{fakeIdentityURI, fakeIdentityURI2},
			identityType: azaciv2.ResourceIdentityTypeUserAssigned,

		},
		{
			description: "identity type is not user assignted",
			identityList: []string{fakeIdentityURI, fakeIdentityURI2},
			identityType: azaciv2.ResourceIdentityTypeSystemAssigned,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

			testContainerGroup := &azaciv2.ContainerGroup{}
			SetContainerGroupIdentity(context.Background(), tc.identityList, tc.identityType, testContainerGroup)

			if tc.identityType == azaciv2.ResourceIdentityTypeUserAssigned && len(tc.identityList) > 0 {
				// identity uri, clientID, principalID should match
				assert.Check(t, testContainerGroup.Identity != nil, "container group identity should be populated")
				assert.Equal(t, *testContainerGroup.Identity.Type, azaciv2.ResourceIdentityTypeUserAssigned, "identity type should match")
				assert.Check(t, len(testContainerGroup.Identity.UserAssignedIdentities) == len(tc.identityList), "all identities should be populated in UserAssignedIdentities")
				assert.Check(t, testContainerGroup.Identity.UserAssignedIdentities[tc.identityList[0]] != nil , "identity uri should be present in UserAssignedIdentities")
			} else {
				// identity should not be added
				assert.Check(t, testContainerGroup.Identity == nil, "container group identity should not be populated")
			}
		})
	}
}

func TestGetManagedIdentityImageRegistryCredentials(t *testing.T) {
	fakeIdentityURI	:= "fakeuri"
	fakeImageName	:= "fakeregistry.azurecr.io/fakeimage:faketag"
	fakeImageName2	:= "fakeregistry2.azurecr.io/fakeimage:faketag"

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

	mockSecretLister := NewMockSecretLister(mockCtrl)

	aciMocks := createNewACIMock()
	provider, err := createTestProvider(aciMocks, NewMockConfigMapLister(mockCtrl),
		mockSecretLister, NewMockPodLister(mockCtrl), nil)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

	cases := []struct {
		description     string
		identity		*string
	}{
		{
			description: "identity is nil",
			identity: nil,
		},
		{
			description: "identity is not nil",
			identity: &fakeIdentityURI,

		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			testContainerGroup := &azaciv2.ContainerGroup{}
			creds := provider.getManagedIdentityImageRegistryCredentials(pod, tc.identity, testContainerGroup)

			if tc.identity != nil{
				// image registry credentials should have identity
				assert.Check(t, creds != nil, "image registry creds should be populated")
				assert.Equal(t, len(creds), 2, "credentials for all distinct acr should be added")
				assert.Equal(t, *(creds)[0].Identity, *tc.identity, "identity uri should be correct")
				assert.Equal(t, *(creds)[1].Identity, *tc.identity, "identity uri should be correct")
				assert.Equal(t, *(creds)[0].Server, "fakeregistry.azurecr.io", "server should be correct")
				assert.Equal(t, *(creds)[1].Server, "fakeregistry2.azurecr.io", "server should be correct")
			} else {
				// identity should not be added to image registry credentials
				assert.Check(t, len(creds) == 0, "image registry creds should not be populated")

			}
		})
	}
}

func TestSetContainerGroupIdentityFromAnnotation(t *testing.T) {
	fakeIdentityURI	:= "fakeuri;fakeuri2"

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
				},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSecretLister := NewMockSecretLister(mockCtrl)

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *azaciv2.ContainerGroup) error {
		ids := strings.Split(pod.Annotations[containerGroupIdentitiesLabel], ";")
		if cg.Identity != nil {
			assert.Check(t, len(ids) == len(cg.Identity.UserAssignedIdentities), "container group identity should be set from annotation")
		}
		return nil
	}

	provider, err := createTestProvider(aciMocks, NewMockConfigMapLister(mockCtrl),
		mockSecretLister, NewMockPodLister(mockCtrl), nil)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

	cases := []struct {
		description     string
		annotations		map[string]string
	}{
		{
			description: "container group identity annotation  is nil ",
			annotations: map[string]string{},
		},
		{
			description: "container group identity annotation is not nil",
			annotations: map[string]string{
				containerGroupIdentitiesLabel: fakeIdentityURI,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			pod.Annotations = tc.annotations
			if err := provider.CreatePod(context.Background(), pod); err != nil {
				t.Fatal("failed to create pod", err)
			}
		})
	}
}

