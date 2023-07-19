/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
)

var (
	// Supported extension types

	ExtensionTypeKubeProxy       = "kube-proxy"
	ExtensionTypeRealtimeMetrics = "realtime-metrics"

	// ExtensionVersion_1 Supported extension version.
	ExtensionVersion_1 = "1.0"

	// Supported kube-proxy extension constants
	KubeProxyExtensionSettingClusterCIDR = "clusterCidr"
	KubeProxyExtensionSettingKubeVersion = "kubeVersion"
	KubeProxyExtensionSettingKubeConfig  = "kubeConfig"
	KubeProxyExtensionKubeVersion        = "v1.9.10"
)

// GetKubeProxyExtension gets the kubeProxy extension
func GetKubeProxyExtension(secretPath, masterURI, clusterCIDR string) (*azaciv2.DeploymentExtensionSpec, error) {
	name := "virtual-kubelet"
	var certAuthData []byte
	var authInfo *clientcmdapi.AuthInfo

	// Try loading kubeconfig if path to it is specified.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); !os.IsNotExist(err) {
			// Get the kubeconfig from the filepath.
			var configFromPath *clientcmdapi.Config
			configFromPath, err = clientcmd.LoadFromFile(kubeconfig)
			if err == nil &&
				len(configFromPath.Clusters) > 0 &&
				len(configFromPath.AuthInfos) > 0 {

				certAuthData = getKubeconfigCertAuthData(configFromPath.Clusters)
				authInfo = getKubeconfigAuthInfo(configFromPath.AuthInfos)
			}
		}
	}

	if len(certAuthData) <= 0 || authInfo == nil {
		var err error
		certAuthData, err = ioutil.ReadFile(secretPath + "/ca.crt")
		if err != nil {
			return nil, fmt.Errorf("failed to read ca.crt file: %v", err)
		}

		var token []byte
		token, err = ioutil.ReadFile(secretPath + "/token")
		if err != nil {
			return nil, fmt.Errorf("failed to read token file: %v", err)
		}

		authInfo = &clientcmdapi.AuthInfo{
			Token: string(token),
		}
	}

	config := clientcmdapiv1.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: []clientcmdapiv1.NamedCluster{
			{
				Name: name,
				Cluster: clientcmdapiv1.Cluster{
					Server:                   masterURI,
					CertificateAuthorityData: certAuthData,
				},
			},
		},
		AuthInfos: []clientcmdapiv1.NamedAuthInfo{
			{
				Name: name,
				AuthInfo: clientcmdapiv1.AuthInfo{
					ClientCertificate:     authInfo.ClientCertificate,
					ClientCertificateData: authInfo.ClientCertificateData,
					ClientKey:             authInfo.ClientKey,
					ClientKeyData:         authInfo.ClientKeyData,
					Token:                 authInfo.Token,
					Username:              authInfo.Username,
					Password:              authInfo.Password,
				},
			},
		},
		Contexts: []clientcmdapiv1.NamedContext{
			{
				Name: name,
				Context: clientcmdapiv1.Context{
					Cluster:  name,
					AuthInfo: name,
				},
			},
		},
		CurrentContext: name,
	}

	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(config); err != nil {
		return nil, fmt.Errorf("failed to encode the kubeconfig: %v", err)
	}

	extension := azaciv2.DeploymentExtensionSpec{
		Name: &ExtensionTypeKubeProxy,
		Properties: &azaciv2.DeploymentExtensionSpecProperties{
			ExtensionType: &ExtensionTypeKubeProxy,
			Version:       &ExtensionVersion_1,
			Settings: map[string]string{
				KubeProxyExtensionSettingClusterCIDR: clusterCIDR,
				KubeProxyExtensionSettingKubeVersion: KubeProxyExtensionKubeVersion,
			},
			ProtectedSettings: map[string]string{
				KubeProxyExtensionSettingKubeConfig: base64.StdEncoding.EncodeToString(b.Bytes()),
			},
		},
	}
	return &extension, nil
}

func getKubeconfigCertAuthData(clusters map[string]*clientcmdapi.Cluster) []byte {
	for _, v := range clusters {
		return v.CertificateAuthorityData
	}

	return make([]byte, 0)
}

func getKubeconfigAuthInfo(authInfos map[string]*clientcmdapi.AuthInfo) *clientcmdapi.AuthInfo {
	for _, v := range authInfos {
		return v
	}

	return nil
}

// GetRealtimeMetricsExtension gets the realtime extension
func GetRealtimeMetricsExtension() *azaciv2.DeploymentExtensionSpec {
	extension := azaciv2.DeploymentExtensionSpec{
		Name: &ExtensionTypeRealtimeMetrics,
		Properties: &azaciv2.DeploymentExtensionSpecProperties{
			ExtensionType:     &ExtensionTypeRealtimeMetrics,
			Version:           &ExtensionVersion_1,
			Settings:          map[string]string{},
			ProtectedSettings: map[string]string{},
		},
	}
	return &extension
}
