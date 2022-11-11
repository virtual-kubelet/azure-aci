/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package tests

import (
	"time"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/date"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	TimeLayout = "2006-01-02 15:04:05.999999999 -0700 MST"
)

var (
	CgCreationTime, _ = time.Parse(TimeLayout, time.Now().String())
	RestartCount      = int32(0)
	FakeIP            = "127.0.0.1"
	TestContainerName = "testContainer"
	TestImageNginx    = "nginx"
	testGPUCount      = int32(5)

	emptyStr          = ""
	cgCreationTime, _ = time.Parse(TimeLayout, time.Now().String())

	testCPU    = float64(0.99)
	testMemory = float64(1.5)
	port       = int32(80)
)

func CreateContainerGroupObj(cgName, cgState string, containers *[]azaci.Container, provisioningState string) *azaci.ContainerGroup {
	fakeIPAddress := azaci.IPAddress{
		IP: &FakeIP,
	}
	timeAsString := v1.NewTime(cgCreationTime).String()

	return &azaci.ContainerGroup{
		Tags: map[string]*string{
			"CreationTimestamp": &timeAsString,
		},
		Name: &cgName,
		ID:   &cgName,
		ContainerGroupProperties: &azaci.ContainerGroupProperties{
			Containers: containers,
			InstanceView: &azaci.ContainerGroupPropertiesInstanceView{
				State: &cgState,
			},
			ProvisioningState: &provisioningState,
			IPAddress:         &fakeIPAddress,
		},
	}
}

func CreateACIContainersListObj(currentState, PrevState string, startTime, finishTime time.Time, hasResources, hasLimits, hasGPU bool) *[]azaci.Container {
	containerList := append([]azaci.Container{}, *CreateACIContainerObj(currentState, PrevState, startTime, finishTime, hasResources, hasLimits, hasGPU))
	return &containerList
}

func CreateACIContainerObj(currentState, PrevState string, startTime, finishTime time.Time, hasResources, hasLimits, hasGPU bool) *azaci.Container {
	return &azaci.Container{
		Name: &TestContainerName,
		ContainerProperties: &azaci.ContainerProperties{
			Image: &TestImageNginx,
			Ports: &[]azaci.ContainerPort{
				{
					Protocol: azaci.ContainerNetworkProtocolTCP,
					Port:     &port,
				},
			},
			Resources: CreateContainerResources(hasResources, hasLimits, hasGPU),
			Command:   &[]string{"nginx", "-g", "daemon off;"},
			InstanceView: &azaci.ContainerPropertiesInstanceView{
				CurrentState:  CreateContainerStateObj(currentState, startTime, finishTime, 0),
				PreviousState: CreateContainerStateObj(PrevState, cgCreationTime, startTime, 0),
				RestartCount:  &RestartCount,
				Events:        &[]azaci.Event{},
			},
			LivenessProbe:  &azaci.ContainerProbe{},
			ReadinessProbe: &azaci.ContainerProbe{},
		},
	}
}

func CreateContainerResources(hasResources, hasLimits, hasGPU bool) *azaci.ResourceRequirements {
	if hasResources {
		return &azaci.ResourceRequirements{
			Requests: &azaci.ResourceRequests{
				CPU:        &testCPU,
				MemoryInGB: &testMemory,
				Gpu:        CreateGPUResource(hasGPU),
			},
			Limits: CreateResourceLimits(hasLimits, hasGPU),
		}
	}
	return nil
}

func CreateResourceLimits(hasLimits, hasGPU bool) *azaci.ResourceLimits {
	if hasLimits {
		return &azaci.ResourceLimits{
			CPU:        &testCPU,
			MemoryInGB: &testMemory,
			Gpu:        CreateGPUResource(hasGPU),
		}
	}
	return nil
}

func CreateGPUResource(hasGPU bool) *azaci.GpuResource {
	if hasGPU {
		return &azaci.GpuResource{
			Count: &testGPUCount,
			Sku:   azaci.GpuSkuP100,
		}
	}
	return nil
}

func CreateContainerStateObj(state string, startTime, finishTime time.Time, exitCode int32) *azaci.ContainerState {
	return &azaci.ContainerState{
		State: &state,
		StartTime: &date.Time{
			Time: startTime,
		},
		ExitCode: &exitCode,
		FinishTime: &date.Time{
			Time: finishTime,
		},
		DetailStatus: &emptyStr,
	}
}

func GetPodConditions(creationTime, readyConditionTime v1.Time, readyConditionStatus v12.ConditionStatus) []v12.PodCondition {
	return []v12.PodCondition{
		{
			Type:               v12.PodReady,
			Status:             readyConditionStatus,
			LastTransitionTime: readyConditionTime,
		}, {
			Type:               v12.PodInitialized,
			Status:             v12.ConditionTrue,
			LastTransitionTime: creationTime,
		}, {
			Type:               v12.PodScheduled,
			Status:             v12.ConditionTrue,
			LastTransitionTime: creationTime,
		},
	}
}

func CreatePodObj(podName, podNamespace string) *v12.Pod {
	return &v12.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v12.PodSpec{
			Containers: []v12.Container{
				{
					Name: "nginx",
					Ports: []v12.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
						},
					},
					Resources: v12.ResourceRequirements{
						Requests: v12.ResourceList{
							"cpu":    resource.MustParse("0.99"),
							"memory": resource.MustParse("1.5G"),
						},
						Limits: v12.ResourceList{
							"cpu":    resource.MustParse("3999m"),
							"memory": resource.MustParse("8010M"),
						},
					},

					LivenessProbe: &v12.Probe{
						Handler: v12.Handler{
							HTTPGet: &v12.HTTPGetAction{
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
					ReadinessProbe: &v12.Probe{
						Handler: v12.Handler{
							HTTPGet: &v12.HTTPGetAction{
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
}

func CreatePodVolumeObj(azureFileVolumeName string, fakeSecretName string, projectedVolumeName string) []v12.Volume {
	emptyVolumeName := "emptyVolumeName"
	fakeShareName1 := "aksshare1"

	return []v12.Volume{
		{
			Name: emptyVolumeName,
			VolumeSource: v12.VolumeSource{
				EmptyDir: &v12.EmptyDirVolumeSource{},
			},
		},
		{
			Name: azureFileVolumeName,
			VolumeSource: v12.VolumeSource{
				AzureFile: &v12.AzureFileVolumeSource{
					ShareName:  fakeShareName1,
					SecretName: fakeSecretName,
					ReadOnly:   true,
				},
			},
		}, {
			Name: projectedVolumeName,
			VolumeSource: v12.VolumeSource{
				Projected: &v12.ProjectedVolumeSource{
					Sources: []v12.VolumeProjection{
						{
							ConfigMap: &v12.ConfigMapProjection{
								LocalObjectReference: v12.LocalObjectReference{
									Name: "kube-root-ca.crt",
								},
								Items: []v12.KeyToPath{
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
}
