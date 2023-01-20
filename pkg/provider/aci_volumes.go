/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"encoding/base64"
	"fmt"

	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
)

func (p *ACIProvider) getAzureFileCSI(volume v1.Volume, namespace string) (*azaci.Volume, error) {
	var secretName, shareName string
	if volume.CSI.VolumeAttributes != nil && len(volume.CSI.VolumeAttributes) != 0 {
		for k, v := range volume.CSI.VolumeAttributes {
			switch k {
			case azureFileSecretName:
				secretName = v
			case azureFileShareName:
				shareName = v
			}
		}
	} else {
		return nil, fmt.Errorf("secret volume attribute for AzureFile CSI driver %s cannot be empty or nil", volume.Name)
	}

	if shareName == "" {
		return nil, fmt.Errorf("share name for AzureFile CSI driver %s cannot be empty or nil", volume.Name)
	}

	if secretName == "" {
		return nil, fmt.Errorf("secret name for AzureFile CSI driver %s cannot be empty or nil", volume.Name)
	}

	secret, err := p.resourceManager.GetSecret(secretName, namespace)

	if err != nil || secret == nil {
		return nil, fmt.Errorf("the secret %s for AzureFile CSI driver %s is not found", secretName, volume.Name)
	}

	storageAccountNameStr := string(secret.Data[azureFileStorageAccountName])
	storageAccountKeyStr := string(secret.Data[azureFileStorageAccountKey])

	return &azaci.Volume{
		Name: &volume.Name,
		AzureFile: &azaci.AzureFileVolume{
			ShareName:          &shareName,
			StorageAccountName: &storageAccountNameStr,
			StorageAccountKey:  &storageAccountKeyStr,
		}}, nil
}

func (p *ACIProvider) getVolumes(ctx context.Context, pod *v1.Pod) ([]*azaci.Volume, error) {
	volumes := make([]*azaci.Volume, 0, len(pod.Spec.Volumes))
	podVolumes := pod.Spec.Volumes
	for i := range podVolumes {
		// Handle the case for Azure File CSI driver
		if podVolumes[i].CSI != nil {
			// Check if the CSI driver is file (Disk is not supported by ACI)
			if podVolumes[i].CSI.Driver == AzureFileDriverName {
				csiVolume, err := p.getAzureFileCSI(podVolumes[i], pod.Namespace)
				if err != nil {
					return nil, err
				}
				volumes = append(volumes, csiVolume)
				continue
			} else {
				return nil, fmt.Errorf("pod %s requires volume %s which is of an unsupported type %s", pod.Name, podVolumes[i].Name, podVolumes[i].CSI.Driver)
			}
		}

		// Handle the case for the AzureFile volume.
		if podVolumes[i].AzureFile != nil {
			secret, err := p.resourceManager.GetSecret(podVolumes[i].AzureFile.SecretName, pod.Namespace)
			if err != nil {
				return volumes, err
			}

			if secret == nil {
				return nil, fmt.Errorf("getting secret for AzureFile volume returned an empty secret")
			}
			storageAccountNameStr := string(secret.Data[azureFileStorageAccountName])
			storageAccountKeyStr := string(secret.Data[azureFileStorageAccountKey])

			volumes = append(volumes, &azaci.Volume{
				Name: &podVolumes[i].Name,
				AzureFile: &azaci.AzureFileVolume{
					ShareName:          &podVolumes[i].AzureFile.ShareName,
					ReadOnly:           &podVolumes[i].AzureFile.ReadOnly,
					StorageAccountName: &storageAccountNameStr,
					StorageAccountKey:  &storageAccountKeyStr,
				},
			})
			continue
		}

		// Handle the case for the EmptyDir.
		if podVolumes[i].EmptyDir != nil {
			log.G(ctx).Info("empty volume name ", podVolumes[i].Name)
			volumes = append(volumes, &azaci.Volume{
				Name:     &podVolumes[i].Name,
				EmptyDir: map[string]interface{}{},
			})
			continue
		}

		// Handle the case for GitRepo volume.
		if podVolumes[i].GitRepo != nil {
			volumes = append(volumes, &azaci.Volume{
				Name: &podVolumes[i].Name,
				GitRepo: &azaci.GitRepoVolume{
					Directory:  &podVolumes[i].GitRepo.Directory,
					Repository: &podVolumes[i].GitRepo.Repository,
					Revision:   &podVolumes[i].GitRepo.Revision,
				},
			})
			continue
		}

		// Handle the case for Secret volume.
		if podVolumes[i].Secret != nil {
			paths := make(map[string]*string)
			secret, err := p.resourceManager.GetSecret(podVolumes[i].Secret.SecretName, pod.Namespace)
			if podVolumes[i].Secret.Optional != nil && !*podVolumes[i].Secret.Optional && k8serr.IsNotFound(err) {
				return nil, fmt.Errorf("secret %s is required by Pod %s and does not exist", podVolumes[i].Secret.SecretName, pod.Name)
			}
			if secret == nil {
				continue
			}

			for k, v := range secret.Data {
				strV := base64.StdEncoding.EncodeToString(v)
				paths[k] = &strV
			}

			if len(paths) != 0 {
				volumes = append(volumes, &azaci.Volume{
					Name:   &podVolumes[i].Name,
					Secret: paths,
				})
			}
			continue
		}

		// Handle the case for ConfigMap volume.
		if podVolumes[i].ConfigMap != nil {
			paths := make(map[string]*string)
			configMap, err := p.resourceManager.GetConfigMap(podVolumes[i].ConfigMap.Name, pod.Namespace)
			if podVolumes[i].ConfigMap.Optional != nil && !*podVolumes[i].ConfigMap.Optional && k8serr.IsNotFound(err) {
				return nil, fmt.Errorf("ConfigMap %s is required by Pod %s and does not exist", podVolumes[i].ConfigMap.Name, pod.Name)
			}
			if configMap == nil {
				continue
			}

			for k, v := range configMap.Data {
				strV := base64.StdEncoding.EncodeToString([]byte(v))
				paths[k] = &strV
			}
			for k, v := range configMap.BinaryData {
				strV := base64.StdEncoding.EncodeToString(v)
				paths[k] = &strV
			}

			if len(paths) != 0 {
				volumes = append(volumes, &azaci.Volume{
					Name:   &podVolumes[i].Name,
					Secret: paths,
				})
			}
			continue
		}

		if podVolumes[i].Projected != nil {
			log.G(ctx).Info("Found projected volume")
			paths := make(map[string]*string)

			for _, source := range podVolumes[i].Projected.Sources {
				switch {
				case source.ServiceAccountToken != nil:
					// This is still stored in a secret, hence the dance to figure out what secret.
					secrets, err := p.resourceManager.GetSecrets(pod.Namespace)
					if err != nil {
						return nil, err
					}
				Secrets:
					for _, secret := range secrets {
						if secret.Type != v1.SecretTypeServiceAccountToken {
							continue
						}
						// annotation now needs to match the pod.ServiceAccountName
						for k, a := range secret.ObjectMeta.Annotations {
							if k == "kubernetes.io/service-account.name" && a == pod.Spec.ServiceAccountName {
								for k, v := range secret.StringData {
									data, err := base64.StdEncoding.DecodeString(v)
									if err != nil {
										return nil, err
									}
									dataStr := string(data)
									paths[k] = &dataStr
								}

								for k, v := range secret.Data {
									strV := base64.StdEncoding.EncodeToString(v)
									paths[k] = &strV
								}

								break Secrets
							}
						}
					}

				case source.Secret != nil:
					secret, err := p.resourceManager.GetSecret(source.Secret.Name, pod.Namespace)
					if source.Secret.Optional != nil && !*source.Secret.Optional && k8serr.IsNotFound(err) {
						return nil, fmt.Errorf("projected secret %s is required by pod %s and does not exist", source.Secret.Name, pod.Name)
					}
					if secret == nil {
						continue
					}

					for _, keyToPath := range source.Secret.Items {
						for k, v := range secret.StringData {
							if keyToPath.Key == k {
								data, err := base64.StdEncoding.DecodeString(v)
								if err != nil {
									return nil, err
								}
								dataStr := string(data)
								paths[k] = &dataStr
							}
						}

						for k, v := range secret.Data {
							if keyToPath.Key == k {
								strV := base64.StdEncoding.EncodeToString(v)
								paths[k] = &strV
							}
						}
					}

				case source.ConfigMap != nil:
					configMap, err := p.resourceManager.GetConfigMap(source.ConfigMap.Name, pod.Namespace)
					if source.ConfigMap.Optional != nil && !*source.ConfigMap.Optional && k8serr.IsNotFound(err) {
						return nil, fmt.Errorf("projected configMap %s is required by pod %s and does not exist", source.ConfigMap.Name, pod.Name)
					}
					if configMap == nil {
						continue
					}

					for _, keyToPath := range source.ConfigMap.Items {
						for k, v := range configMap.Data {
							if keyToPath.Key == k {
								strV := base64.StdEncoding.EncodeToString([]byte(v))
								paths[k] = &strV
							}
						}
						for k, v := range configMap.BinaryData {
							if keyToPath.Key == k {
								strV := base64.StdEncoding.EncodeToString(v)
								paths[k] = &strV
							}
						}
					}
				}
			}
			if len(paths) != 0 {
				volumes = append(volumes, &azaci.Volume{
					Name:   &podVolumes[i].Name,
					Secret: paths,
				})
			}
			continue
		}

		// If we've made it this far we have found a volume type that isn't supported
		return nil, fmt.Errorf("pod %s requires volume %s which is of an unsupported type", pod.Name, podVolumes[i].Name)
	}

	return volumes, nil
}
