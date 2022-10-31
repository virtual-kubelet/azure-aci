package provider


import (
	"context"
	"fmt"
	"testing"

	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/golang/mock/gomock"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/node-cli/manager"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCreatePodWithInitContainers(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	aciMocks := createNewACIMock()
	aciMocks.MockCreateContainerGroup = func(ctx context.Context, resourceGroup, podNS, podName string, cg *client.ContainerGroupWrapper) error {
		containers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.Containers
		initContainers := *cg.ContainerGroupPropertiesWrapper.ContainerGroupProperties.InitContainers
		assert.Check(t, cg != nil, "Container group is nil")
		assert.Check(t, containers != nil, "Containers should not be nil")
		assert.Check(t, initContainers != nil, "Container group is nil")
		assert.Check(t, is.Equal(len(containers), 1), "1 Container is expected")
		assert.Check(t, is.Equal(len(initContainers), 2), "2 init containers are expected")
		assert.Check(t, initContainers[0].VolumeMounts != nil, "Volume mount should be present")
		assert.Check(t, initContainers[0].EnvironmentVariables != nil, "Volume mount should be present")
		assert.Check(t, initContainers[0].Command != nil, "Command mount should be present")
		assert.Check(t, initContainers[0].Image != nil, "Image should be present")

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
					Name: "container name",
				},
			},
		},
	}
	cases := []struct {
		description     string
		initContainers	[]v1.Container
		expectedError   error
	}{
		{
			description:  "Init Containers with Supported fields",
			expectedError: nil,
			initContainers: []v1.Container{
				v1.Container{
					Name:  "initContainer 01",
					Image: "alpine",
					VolumeMounts: []v1.VolumeMount{
						v1.VolumeMount{
							Name:      "fakeVolumeName",
							MountPath: "/mnt/azure",
						},
					},
					Command: []string{"/bin/bash"},
					Args: []string{"-c echo test"},
					Env: []v1.EnvVar{
						v1.EnvVar{
							Name: "TEST_ENV",
							Value: "testvalue",
						},
					},
				},
				v1.Container{
					Name:  "initContainer 02",
					Image: "alpine",
				},
			},
		},
		{
			description:  "Init Containers with ports",
			initContainers: []v1.Container{
				v1.Container{
					Name:  "initContainer 01",
					Image: "alpine",
					Ports: []v1.ContainerPort{
						v1.ContainerPort{
							Name: "http",
							ContainerPort: 80,
							Protocol: "TCP",
						},
					},
				},
			},
			expectedError: errdefs.InvalidInput("ACI initContainers does not support ports."),
		},
		{
			description:  "Init Containers with liveness probe",
			initContainers: []v1.Container{
				v1.Container{
					Name: "initContainer 01",
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
						HTTPGet: &v1.HTTPGetAction{
								Port: intstr.FromString("http"),
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
			expectedError: errdefs.InvalidInput("ACI initContainers does not support livenessProbe."),
		},
		{
			description:  "Init Containers with readiness probe",
			initContainers: []v1.Container{
				v1.Container{
					Name: "initContainer 01",
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
			expectedError: errdefs.InvalidInput("ACI initContainers does not support readinessProbe."),
		},
		{
			description:  "Init Containers with resource request",
			initContainers: []v1.Container{
				{
					Name: "initContainer 01",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("1.981"),
							"memory": resource.MustParse("3.49G"),
						},
						Limits: v1.ResourceList{
							gpuResourceName: resource.MustParse("10"),
						},
					},
				},
			},
			expectedError: errdefs.InvalidInput("ACI initContainers does not support resources requests."),
		},
	}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

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

			pod.Spec.InitContainers = tc.initContainers
			err = provider.CreatePod(context.Background(), pod)

			// check that the correct error is returned
			if tc.expectedError != nil && err != tc.expectedError {
				assert.Equal(t, tc.expectedError.Error(), err.Error(), "expected error and actual error don't match")
			}
		})
	}
}
