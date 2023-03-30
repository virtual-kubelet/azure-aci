/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package tests

import (
	"time"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/google/uuid"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	gpuSKUP100 = azaciv2.GpuSKUP100
	scheme     = azaciv2.SchemeHTTP
)

func CreateContainerGroupObj(cgName, cgNamespace, cgState string, containers []*azaciv2.Container, provisioningState string) *azaciv2.ContainerGroup {
	fakeIPAddress := azaciv2.IPAddress{
		IP: &FakeIP,
	}
	timeAsString := v1.NewTime(cgCreationTime).String()
	nodeName := "vk"

	return &azaciv2.ContainerGroup{
		Tags: map[string]*string{
			"CreationTimestamp": &timeAsString,
			"PodName":           &cgName,
			"Namespace":         &cgNamespace,
			"NodeName":          &nodeName,
			"ClusterName":       &nodeName,
			"UID":               &cgName,
		},
		Name: &cgName,
		ID:   &cgName,
		Properties: &azaciv2.ContainerGroupPropertiesProperties{
			Containers: containers,
			InstanceView: &azaciv2.ContainerGroupPropertiesInstanceView{
				State: &cgState,
			},
			ProvisioningState: &provisioningState,
			IPAddress:         &fakeIPAddress,
		},
	}
}

func CreateACIContainersListObj(currentState, PrevState string, startTime, finishTime time.Time, hasResources, hasLimits, hasGPU bool) []*azaciv2.Container {
	containerList := append([]*azaciv2.Container{}, CreateACIContainerObj(currentState, PrevState, startTime, finishTime, hasResources, hasLimits, hasGPU))
	return containerList
}

func CreateACIContainerObj(currentState, PrevState string, startTime, finishTime time.Time, hasResources, hasLimits, hasGPU bool) *azaciv2.Container {
	return &azaciv2.Container{
		Name: &TestContainerName,
		Properties: &azaciv2.ContainerProperties{
			Image: &TestImageNginx,
			Ports: []*azaciv2.ContainerPort{
				{
					Protocol: &util.ContainerNetworkProtocolTCP,
					Port:     &port,
				},
			},
			Resources: CreateContainerResources(hasResources, hasLimits, hasGPU),
			Command:   []*string{},
			InstanceView: &azaciv2.ContainerPropertiesInstanceView{
				CurrentState:  CreateContainerStateObj(currentState, startTime, finishTime, 0),
				PreviousState: CreateContainerStateObj(PrevState, cgCreationTime, startTime, 0),
				RestartCount:  &RestartCount,
				Events:        []*azaciv2.Event{},
			},
			LivenessProbe:  &azaciv2.ContainerProbe{},
			ReadinessProbe: &azaciv2.ContainerProbe{},
		},
	}
}

func CreateContainerResources(hasResources, hasLimits, hasGPU bool) *azaciv2.ResourceRequirements {
	if hasResources {
		return &azaciv2.ResourceRequirements{
			Requests: &azaciv2.ResourceRequests{
				CPU:        &testCPU,
				MemoryInGB: &testMemory,
				Gpu:        CreateGPUResource(hasGPU),
			},
			Limits: CreateResourceLimits(hasLimits, hasGPU),
		}
	}
	return nil
}

func CreateResourceLimits(hasLimits, hasGPU bool) *azaciv2.ResourceLimits {
	if hasLimits {
		return &azaciv2.ResourceLimits{
			CPU:        &testCPU,
			MemoryInGB: &testMemory,
			Gpu:        CreateGPUResource(hasGPU),
		}
	}
	return nil
}

func CreateGPUResource(hasGPU bool) *azaciv2.GpuResource {
	if hasGPU {
		return &azaciv2.GpuResource{
			Count: &testGPUCount,
			SKU:   &gpuSKUP100,
		}
	}
	return nil
}

func CreateContainerStateObj(state string, startTime, finishTime time.Time, exitCode int32) *azaciv2.ContainerState {
	return &azaciv2.ContainerState{
		State:        &state,
		StartTime:    &startTime,
		ExitCode:     &exitCode,
		FinishTime:   &finishTime,
		DetailStatus: &emptyStr,
	}
}

func CreateCGProbeObj(hasHTTPGet, hasExec bool) *azaciv2.ContainerProbe {
	var bin, c, command, path string

	bin = "/bin/sh"
	c = "-c"
	command = "/probes/"
	path = "/"
	port := int32(8080)
	fakeNum := int32(0)

	var exec *azaciv2.ContainerExec
	var httpGet *azaciv2.ContainerHTTPGet

	if hasExec {
		exec = &azaciv2.ContainerExec{
			Command: []*string{
				&bin,
				&c,
				&command,
			},
		}
	}
	if hasHTTPGet {
		httpGet = &azaciv2.ContainerHTTPGet{
			Port:   &port,
			Path:   &path,
			Scheme: &scheme,
		}
	}
	return &azaciv2.ContainerProbe{
		Exec:                exec,
		HTTPGet:             httpGet,
		InitialDelaySeconds: &fakeNum,
		FailureThreshold:    &fakeNum,
		SuccessThreshold:    &fakeNum,
		TimeoutSeconds:      &fakeNum,
		PeriodSeconds:       &fakeNum,
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
						ProbeHandler: v12.ProbeHandler{
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
						ProbeHandler: v12.ProbeHandler{
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

func CreatePodProbeObj(hasHTTPGet, hasExec bool) *v12.Probe {
	var httpGet *v12.HTTPGetAction
	var exec *v12.ExecAction

	if hasHTTPGet {
		httpGet = &v12.HTTPGetAction{
			Port:   intstr.FromString("http"),
			Path:   "/",
			Scheme: "http",
		}
	}
	if hasExec {
		exec = &v12.ExecAction{
			Command: []string{
				"/bin/sh",
				"-c",
				"/probes/",
			},
		}
	}

	return &v12.Probe{
		ProbeHandler: v12.ProbeHandler{
			HTTPGet: httpGet,
			Exec:    exec,
		},
	}
}

func CreateContainerPortObj(portName string, containerPort int32) []v12.ContainerPort {
	return []v12.ContainerPort{
		{
			Name:          portName,
			ContainerPort: containerPort,
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

func CreatePodsList(podNames []string, podNameSpace string) []*v12.Pod {
	result := make([]*v12.Pod, 0, len(podNames))
	for _, podName := range podNames {
		pod := &v12.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:              podName,
				Namespace:         podNameSpace,
				CreationTimestamp: v1.NewTime(time.Now()),
				UID:               types.UID(uuid.New().String()),
			},
			Status: v12.PodStatus{
				Phase: v12.PodRunning,
				ContainerStatuses: []v12.ContainerStatus {
					{
						State: v12.ContainerState {
							Running: &v12.ContainerStateRunning{
								StartedAt: v1.NewTime(time.Now()),
							},
						},
					},
				},
			},
		}
		result = append(result, pod)
	}
	return result
}
