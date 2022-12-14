/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/azure-aci/pkg/featureflag"
	testsutil "github.com/virtual-kubelet/azure-aci/pkg/tests"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	podName         = "pod-" + uuid.New().String()
	podNamespace    = "ns-" + uuid.New().String()
	emptyVolumeName = "emptyVolumeName"
	fakeShareName1  = "aksshare1"
	fakeShareName2  = "aksshare2"
)

func TestCreatedPodWithAzureFilesVolume(t *testing.T) {
	azureFileVolumeName1 := "azurefile1"
	azureFileVolumeName2 := "azurefile2"
	fakeSecretName := "fake-secret"
	initContainerName := "init-container"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockSecretLister := NewMockSecretLister(mockCtrl)
	resourceManager, err := manager.NewResourceManager(
		NewMockPodLister(mockCtrl),
		mockSecretLister,
		NewMockConfigMapLister(mockCtrl),
		NewMockServiceLister(mockCtrl),
		NewMockPersistentVolumeClaimLister(mockCtrl),
		NewMockPersistentVolumeLister(mockCtrl))
	if err != nil {
		t.Fatal("Unable to prepare the mocks for resourceManager", err)
	}
	aciMocks := createNewACIMock()

	provider, err := createTestProvider(aciMocks, resourceManager)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

	initEnabled := provider.enabledFeatures.IsEnabled(context.TODO(), featureflag.InitContainerFeature)

	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Containers
		// Check only if init container feature is enabled
		if initEnabled {
			initContainers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.InitContainers
			assert.Check(t, initContainers[0].VolumeMounts != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].EnvironmentVariables != nil, "Volume mount should be present")
			assert.Check(t, initContainers[0].Command != nil, "Command mount should be present")
			assert.Check(t, initContainers[0].Image != nil, "Image should be present")
			assert.Check(t, *initContainers[0].Name == initContainerName, "Name should be correct")
		}

		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers)[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(3, len(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)), "volume count not match")
		assert.Check(t, is.Equal(azureFileVolumeName1, *(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)[1].Name), "volume name is not matched")
		assert.Check(t, is.Equal(fakeShareName1, *(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)[1].AzureFile.ShareName), "volume share name is not matched")
		assert.Check(t, is.Equal(azureFileVolumeName2, *(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)[2].Name), "volume name is not matched")
		assert.Check(t, is.Equal(fakeShareName2, *(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)[2].AzureFile.ShareName), "volume share name is not matched")

		return nil
	}

	fakeSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeSecretName,
			Namespace: podNamespace,
		},
		Data: map[string][]byte{
			azureFileStorageAccountName: []byte("azure storage account name"),
			azureFileStorageAccountKey:  []byte("azure storage account key")},
	}

	fakeVolumeMount := []v1.VolumeMount{
		{
			Name:      azureFileVolumeName1,
			MountPath: "/mnt/azure1",
		}, {
			Name:      azureFileVolumeName2,
			MountPath: "/mnt/azure2",
		}}

	fakeVolumes := []v1.Volume{
		{
			Name: emptyVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: azureFileVolumeName1,
			VolumeSource: v1.VolumeSource{
				AzureFile: &v1.AzureFileVolumeSource{
					ShareName:  fakeShareName1,
					SecretName: fakeSecretName,
					ReadOnly:   true,
				},
			},
		}, {
			Name: azureFileVolumeName2,
			VolumeSource: v1.VolumeSource{
				AzureFile: &v1.AzureFileVolumeSource{
					ShareName:  fakeShareName2,
					SecretName: fakeSecretName,
					ReadOnly:   true,
				},
			},
		},
	}

	cases := []struct {
		description     string
		secretVolume    *v1.Secret
		volumes         []v1.Volume
		callSecretMocks func(secretMock *MockSecretLister)
		expectedError   error
	}{
		{
			description:  "Secret is nil",
			secretVolume: nil,
			volumes:      fakeVolumes,
			callSecretMocks: func(secretMock *MockSecretLister) {
				for _, volume := range fakeVolumes {
					if volume.Name == azureFileVolumeName1 {
						mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
						secretMock.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
						mockSecretNamespaceLister.EXPECT().Get(volume.AzureFile.SecretName).Return(nil, nil)
					}
				}
			},
			expectedError: fmt.Errorf("getting secret for AzureFile volume returned an empty secret"),
		},
		{
			description:  "Volume has a secret with a valid value",
			secretVolume: &fakeSecret,
			volumes:      fakeVolumes,
			callSecretMocks: func(secretMock *MockSecretLister) {
				for _, volume := range fakeVolumes {
					if volume.Name == azureFileVolumeName1 || volume.Name == azureFileVolumeName2 {
						mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
						secretMock.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
						mockSecretNamespaceLister.EXPECT().Get(volume.AzureFile.SecretName).Return(&fakeSecret, nil)
					}
				}
			},
			expectedError: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

			pod := testsutil.CreatePodObj(podName, podNamespace)
			pod.Spec.Containers[0].VolumeMounts = fakeVolumeMount
			pod.Spec.InitContainers = []v1.Container{
				v1.Container{
					Name:  initContainerName,
					Image: "alpine",
					VolumeMounts: []v1.VolumeMount{
						v1.VolumeMount{
							Name:      "fakeVolume",
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
			}

			tc.callSecretMocks(mockSecretLister)
			pod.Spec.Volumes = tc.volumes

			err = provider.CreatePod(context.Background(), pod)

			if tc.expectedError == nil {
				assert.NilError(t, tc.expectedError, err)
			} else {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			}

		})
	}
}

func TestCreatePodWithProjectedVolume(t *testing.T) {
	projectedVolumeName := "projectedvolume"
	fakeSecretName := "fake-secret"
	azureFileVolumeName := "azurefile"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	secretLister := NewMockSecretLister(mockCtrl)
	configMapLister := NewMockConfigMapLister(mockCtrl)
	mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
	configMapNamespaceLister := NewMockConfigMapNamespaceLister(mockCtrl)

	configMapLister.EXPECT().ConfigMaps(podNamespace).Return(configMapNamespaceLister)
	configMapNamespaceLister.EXPECT().Get("kube-root-ca.crt").Return(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-root-ca.crt",
		},
		Data: map[string]string{
			"ca.crt": "fake-ca-data",
			"foo":    "bar",
		},
	}, nil)

	resourceManager, err := manager.NewResourceManager(
		NewMockPodLister(mockCtrl),
		secretLister,
		configMapLister,
		NewMockServiceLister(mockCtrl),
		NewMockPersistentVolumeClaimLister(mockCtrl),
		NewMockPersistentVolumeLister(mockCtrl))
	if err != nil {
		t.Fatal("Unable to prepare the mocks for resourceManager", err)
	}

	aciMocks := createNewACIMock()

	encodedSecretVal := base64.StdEncoding.EncodeToString([]byte("fake-ca-data"))
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Containers
		volumes := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes
		certVal := (*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)[2].Secret["ca.crt"]
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, is.Equal(3, len(volumes)), "volume count not match")
		assert.Check(t, is.Equal(projectedVolumeName, *volumes[2].Name), "volume name doesn't match")
		assert.Check(t, is.Equal(encodedSecretVal, *certVal), "configmap data doesn't match")

		return nil
	}

	aciMocks.MockGetContainerGroupInfo = func(ctx context.Context, resourceGroup, namespace, name, nodeName string) (*azaci.ContainerGroup, error) {
		caStr := "ca.crt"
		node := fakeNodeName
		cgName := "nginx"
		provisioningState := "Creating"
		return &azaci.ContainerGroup{
			Tags: map[string]*string{
				"CreationTimestamp": &creationTime,
				"PodName":           &podName,
				"Namespace":         &podNamespace,
				"ClusterName":       &node,
				"NodeName":          &node,
				"UID":               &podName,
			},
			Name: &cgName,
			ContainerGroupProperties: &azaci.ContainerGroupProperties{
				ProvisioningState: &provisioningState,
				Volumes: &[]azaci.Volume{
					{
						Name: &emptyVolumeName,
					}, {
						Name: &azureFileVolumeName,
						AzureFile: &azaci.AzureFileVolume{
							ShareName: &fakeShareName1,
						},
					}, {
						Name:   &projectedVolumeName,
						Secret: map[string]*string{"Key": &caStr, "Path": &caStr},
					},
				},
			},
		}, nil
	}

	fakeSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeSecretName,
			Namespace: podNamespace,
		},
		Data: map[string][]byte{
			azureFileStorageAccountName: []byte("azure storage account name"),
			azureFileStorageAccountKey:  []byte("azure storage account key")},
	}

	fakeVolumeMount := v1.VolumeMount{
		Name:      azureFileVolumeName,
		MountPath: "/mnt/azure1",
	}

	fakeVolumes := testsutil.CreatePodVolumeObj(azureFileVolumeName, fakeSecretName, projectedVolumeName)

	pod := testsutil.CreatePodObj(podName, podNamespace)
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, fakeVolumeMount)

	for _, volume := range fakeVolumes {
		if volume.AzureFile != nil {
			secretLister.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
			mockSecretNamespaceLister.EXPECT().Get(volume.AzureFile.SecretName).Return(fakeSecret, nil)
		}
	}

	pod.Spec.Volumes = fakeVolumes

	provider, err := createTestProvider(aciMocks, resourceManager)
	if err != nil {
		t.Fatal("Unable to create test provider", err)
	}

	if err := provider.CreatePod(context.Background(), pod); err != nil {
		t.Fatal("Failed to create pod", err)
	}
}

func TestCreatePodWithCSIVolume(t *testing.T) {
	fakeVolumeSecret := "fake-volume-secret"
	azureFileVolumeName := "azure"

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Containers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", *(containers[0]).Name), "Container nginx is expected")
		assert.Check(t, is.Equal(2, len(*cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Volumes)), "volume count not match")

		return nil
	}

	fakeSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeVolumeSecret,
			Namespace: podNamespace,
		},
		Data: map[string][]byte{
			azureFileStorageAccountName: []byte("azure storage account name"),
			azureFileStorageAccountKey:  []byte("azure storage account key")},
	}

	fakeVolumeMount := v1.VolumeMount{
		Name:      azureFileVolumeName,
		MountPath: "/mnt/azure",
	}

	fakePodVolumes := []v1.Volume{
		{
			Name: emptyVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: azureFileVolumeName,
			VolumeSource: v1.VolumeSource{
				CSI: &v1.CSIVolumeSource{
					Driver: "file.csi.azure.com",
					VolumeAttributes: map[string]string{
						azureFileSecretName: fakeVolumeSecret,
						azureFileShareName:  fakeShareName1,
					},
				},
			},
		},
	}

	mockCtrl := gomock.NewController(t)

	cases := []struct {
		description     string
		secretVolume    *v1.Secret
		volumes         []v1.Volume
		callSecretMocks func(secretMock *MockSecretLister)
		expectedError   error
	}{
		{
			description:  "Secret is nil",
			secretVolume: nil,
			volumes:      fakePodVolumes,
			callSecretMocks: func(secretMock *MockSecretLister) {
				for _, volume := range fakePodVolumes {
					if volume.Name == azureFileVolumeName {
						if len(volume.CSI.VolumeAttributes) != 0 {
							mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
							secretMock.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
							mockSecretNamespaceLister.EXPECT().Get(volume.CSI.VolumeAttributes[azureFileSecretName]).Return(nil, nil)
						}
					}
				}
			},
			expectedError: fmt.Errorf("the secret %s for AzureFile CSI driver %s is not found", fakeSecret.Name, fakePodVolumes[1].Name),
		},
		{
			description:  "Volume has a secret with a valid value",
			secretVolume: &fakeSecret,
			volumes:      fakePodVolumes,
			callSecretMocks: func(secretMock *MockSecretLister) {
				for _, volume := range fakePodVolumes {
					if volume.CSI != nil {
						if len(volume.CSI.VolumeAttributes) != 0 {
							mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
							secretMock.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
							mockSecretNamespaceLister.EXPECT().Get(volume.CSI.VolumeAttributes[azureFileSecretName]).Return(&fakeSecret, nil)
						}
					}
				}
			},
			expectedError: nil,
		},
		{
			description:  "Volume has no secret",
			secretVolume: &fakeSecret,
			volumes: []v1.Volume{{
				Name: azureFileVolumeName,
				VolumeSource: v1.VolumeSource{
					CSI: &v1.CSIVolumeSource{
						Driver:           "file.csi.azure.com",
						VolumeAttributes: map[string]string{},
					},
				},
			}},
			callSecretMocks: func(secretMock *MockSecretLister) {},
			expectedError:   fmt.Errorf("secret volume attribute for AzureFile CSI driver %s cannot be empty or nil", azureFileVolumeName),
		},
		{
			description:  "Volume has no share name",
			secretVolume: &fakeSecret,
			volumes: []v1.Volume{
				{
					Name: azureFileVolumeName,
					VolumeSource: v1.VolumeSource{
						CSI: &v1.CSIVolumeSource{
							Driver: "file.csi.azure.com",
							VolumeAttributes: map[string]string{
								azureFileSecretName: fakeVolumeSecret,
							},
						},
					},
				}},
			callSecretMocks: func(secretMock *MockSecretLister) {},
			expectedError:   fmt.Errorf("share name for AzureFile CSI driver %s cannot be empty or nil", fakePodVolumes[1].Name),
		},
		{
			description:  "Volume is Disk Driver",
			secretVolume: &fakeSecret,
			volumes: []v1.Volume{
				{
					Name: azureFileVolumeName,
					VolumeSource: v1.VolumeSource{
						CSI: &v1.CSIVolumeSource{
							Driver: "disk.csi.azure.com",
							VolumeAttributes: map[string]string{
								azureFileSecretName: fakeVolumeSecret,
								azureFileShareName:  fakeShareName1,
							},
						},
					},
				},
			},
			callSecretMocks: func(secretMock *MockSecretLister) {},
			expectedError:   fmt.Errorf("pod %s requires volume %s which is of an unsupported type %s", podName, azureFileVolumeName, "disk.csi.azure.com"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			mockSecretLister := NewMockSecretLister(mockCtrl)

			pod := testsutil.CreatePodObj(podName, podNamespace)
			tc.callSecretMocks(mockSecretLister)

			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, fakeVolumeMount)
			pod.Spec.Volumes = tc.volumes

			resourceManager, err := manager.NewResourceManager(
				NewMockPodLister(mockCtrl),
				mockSecretLister,
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

			err = provider.CreatePod(context.Background(), pod)

			if tc.expectedError == nil {
				assert.NilError(t, tc.expectedError, err)
			} else {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			}

		})
	}
}
