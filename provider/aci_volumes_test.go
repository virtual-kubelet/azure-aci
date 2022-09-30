package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	aciServerMocker := NewACIMock()
	mockCtrl := gomock.NewController(t)

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}
		return http.StatusOK, manifest
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(3, len(cg.Volumes)), "volume count not match")
		assert.Check(t, is.Equal(azureFileVolumeName1, *(cg.Volumes[1]).Name), "volume name is not matched")
		assert.Check(t, is.Equal(fakeShareName1, *(cg.Volumes[1]).AzureFile.ShareName), "volume share name is not matched")
		assert.Check(t, is.Equal(azureFileVolumeName2, *(cg.Volumes[2]).Name), "volume name is not matched")
		assert.Check(t, is.Equal(fakeShareName2, *(cg.Volumes[2]).AzureFile.ShareName), "volume share name is not matched")

		return http.StatusOK, cg
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
			mockSecretLister := NewMockSecretLister(mockCtrl)

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "nginx",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Port: intstr.FromInt(8080),
										Path: "/",
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      60,
								SuccessThreshold:    3,
								FailureThreshold:    5,
							},
						},
					},
				},
			}

			pod.Spec.Containers[0].VolumeMounts = fakeVolumeMount

			tc.callSecretMocks(mockSecretLister)
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

			provider, err := createTestProvider(NewAADMock(), aciServerMocker, resourceManager)
			if err != nil {
				t.Fatal("Unable to create test provider", err)
			}

			err = provider.CreatePod(context.Background(), pod)

			for _, vol := range tc.volumes {
				if vol.Name == azureFileVolumeName1 || vol.Name == azureFileVolumeName2 {
					if tc.expectedError != nil {
						assert.Equal(t, tc.expectedError.Error(), err.Error())
					} else {
						assert.NilError(t, tc.expectedError, err)
					}
				}
			}
		})
	}
}
func TestCreatePodWithProjectedVolume(t *testing.T) {
	projectedVolumeName := "projectedvolume"
	fakeSecretName := "fake-secret"
	azureFileVolumeName := "azurefile"

	aciServerMocker := NewACIMock()

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}

		return http.StatusOK, manifest
	}

	mockCtrl := gomock.NewController(t)
	secretLister := NewMockSecretLister(mockCtrl)
	configMapLister := NewMockConfigMapLister(mockCtrl)
	mockSecretNamespaceLister := NewMockSecretNamespaceLister(mockCtrl)
	configMapNamespaceLister := NewMockConfigMapNamespaceLister(mockCtrl)

	encodedSecretVal := base64.StdEncoding.EncodeToString([]byte("fake-ca-data"))

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(3, len(cg.Volumes)), "volume count not match")
		certVal := cg.Volumes[2].Secret["ca.crt"]
		assert.Check(t, is.Equal(encodedSecretVal, *certVal), "configmap data doesn't match")
		return http.StatusOK, cg
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

	fakeVolumes := []v1.Volume{
		{
			Name: emptyVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: azureFileVolumeName,
			VolumeSource: v1.VolumeSource{
				AzureFile: &v1.AzureFileVolumeSource{
					ShareName:  fakeShareName1,
					SecretName: fakeSecretName,
					ReadOnly:   true,
				},
			},
		}, {
			Name: projectedVolumeName,
			VolumeSource: v1.VolumeSource{
				Projected: &v1.ProjectedVolumeSource{
					Sources: []v1.VolumeProjection{
						{
							ConfigMap: &v1.ConfigMapProjection{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "kube-root-ca.crt",
								},
								Items: []v1.KeyToPath{
									{
										Key:  "ca.crt",
										Path: "ca.crt",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "nginx",
					ReadinessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromInt(8080),
								Path: "/",
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       5,
						TimeoutSeconds:      60,
						SuccessThreshold:    3,
						FailureThreshold:    5,
					},
				},
			},
		},
	}
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, fakeVolumeMount)

	for _, volume := range fakeVolumes {
		if volume.AzureFile != nil {
			secretLister.EXPECT().Secrets(podNamespace).Return(mockSecretNamespaceLister)
			mockSecretNamespaceLister.EXPECT().Get(volume.AzureFile.SecretName).Return(fakeSecret, nil)
		}
	}

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

	pod.Spec.Volumes = fakeVolumes

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

	provider, err := createTestProvider(NewAADMock(), aciServerMocker, resourceManager)
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

	aciServerMocker := NewACIMock()

	aciServerMocker.OnGetRPManifest = func() (int, interface{}) {
		manifest := &aci.ResourceProviderManifest{
			Metadata: &aci.ResourceProviderMetadata{
				GPURegionalSKUs: []*aci.GPURegionalSKU{
					{
						Location: fakeRegion,
						SKUs:     []aci.GPUSKU{aci.K80, aci.P100, aci.V100},
					},
				},
			},
		}
		return http.StatusOK, manifest
	}

	aciServerMocker.OnCreate = func(subscription, resourceGroup, containerGroup string, cg *aci.ContainerGroup) (int, interface{}) {
		assert.Check(t, is.Equal(fakeSubscription, subscription), "Subscription doesn't match")
		assert.Check(t, is.Equal(fakeResourceGroup, resourceGroup), "Resource group doesn't match")
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, is.Equal(podNamespace+"-"+podName, containerGroup), "Container group name is not expected")
		assert.Check(t, cg.ContainerGroupProperties.Containers != nil, "Containers should not be nil")
		assert.Check(t, is.Equal(1, len(cg.ContainerGroupProperties.Containers)), "1 Container is expected")
		assert.Check(t, is.Equal("nginx", cg.ContainerGroupProperties.Containers[0].Name), "Container nginx is expected")
		assert.Check(t, is.Equal(2, len(cg.Volumes)), "volume count not match")

		return http.StatusOK, cg
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

			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "nginx",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Port: intstr.FromInt(8080),
										Path: "/",
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      60,
								SuccessThreshold:    3,
								FailureThreshold:    5,
							},
						},
					},
				},
			}

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

			provider, err := createTestProvider(NewAADMock(), aciServerMocker, resourceManager)
			if err != nil {
				t.Fatal("Unable to create test provider", err)
			}

			err = provider.CreatePod(context.Background(), pod)

			for _, vol := range tc.volumes {
				if vol.Name == azureFileVolumeName {
					if tc.expectedError != nil {
						assert.Equal(t, tc.expectedError.Error(), err.Error())
					} else {
						assert.NilError(t, tc.expectedError, err)
					}
				}
			}
		})
	}
}
