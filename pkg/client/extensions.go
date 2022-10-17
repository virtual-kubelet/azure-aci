package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
)

// Extension is the container group extension
type Extension struct {
	Name       string               `json:"name"`
	Properties *ExtensionProperties `json:"properties"`
}

// ExtensionProperties is the properties for extension
type ExtensionProperties struct {
	Type              ExtensionType     `json:"extensionType"`
	Version           ExtensionVersion  `json:"version"`
	Settings          map[string]string `json:"settings,omitempty"`
	ProtectedSettings map[string]string `json:"protectedSettings,omitempty"`
}

// ExtensionType is an enum type for defining supported extension types
type ExtensionType string

// Supported extension types
const (
	ExtensionTypeKubeProxy       ExtensionType = "kube-proxy"
	ExtensionTypeRealtimeMetrics ExtensionType = "realtime-metrics"
)

// ExtensionVersion is an enum type for defining supported extension versions
type ExtensionVersion string

const (
	// ExtensionVersion_1 Supported extension version.
	ExtensionVersion_1 ExtensionVersion = "1.0"
)

// Supported kube-proxy extension constants
const (
	KubeProxyExtensionSettingClusterCIDR string = "clusterCidr"
	KubeProxyExtensionSettingKubeVersion string = "kubeVersion"
	KubeProxyExtensionSettingKubeConfig  string = "kubeConfig"
	KubeProxyExtensionKubeVersion        string = "v1.9.10"
)

// GetKubeProxyExtension gets the kubeproxy extension
func GetKubeProxyExtension(secretPath, masterURI, clusterCIDR string) (*Extension, error) {
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

	extension := Extension{
		Name: "kube-proxy",
		Properties: &ExtensionProperties{
			Type:    ExtensionTypeKubeProxy,
			Version: ExtensionVersion_1,
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
func GetRealtimeMetricsExtension() *Extension {
	extension := Extension{
		Name: "vk-realtime-metrics",
		Properties: &ExtensionProperties{
			Type:              ExtensionTypeRealtimeMetrics,
			Version:           ExtensionVersion_1,
			Settings:          map[string]string{},
			ProtectedSettings: map[string]string{},
		},
	}
	return &extension
}
