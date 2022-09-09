package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
	"github.com/gorilla/websocket"
	client "github.com/virtual-kubelet/azure-aci/client"
	"github.com/virtual-kubelet/azure-aci/client/aci"
	"github.com/virtual-kubelet/azure-aci/client/network"
	"github.com/virtual-kubelet/azure-aci/provider/metrics"
	"github.com/virtual-kubelet/node-cli/manager"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
)

const (
	// The service account secret mount path.
	serviceAccountSecretMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"

	virtualKubeletDNSNameLabel = "virtualkubelet.io/dnsnamelabel"

	subnetDelegationService = "Microsoft.ContainerInstance/containerGroups"
	// Parameter names defined in azure file CSI driver, refer to
	// https://github.com/kubernetes-sigs/azurefile-csi-driver/blob/master/docs/driver-parameters.md
	azureFileShareName  = "shareName"
	azureFileSecretName = "secretName"
	// AzureFileDriverName is the name of the CSI driver for Azure File
	AzureFileDriverName         = "file.csi.azure.com"
	azureFileStorageAccountName = "azurestorageaccountname"
	azureFileStorageAccountKey  = "azurestorageaccountkey"
)

// DNS configuration settings
const (
	maxDNSNameservers     = 3
	maxDNSSearchPaths     = 6
	maxDNSSearchListChars = 256
)

const (
	gpuResourceName   = "nvidia.com/gpu"
	gpuTypeAnnotation = "virtual-kubelet.io/gpu-type"
)

const (
	statusReasonPodDeleted            = "NotFound"
	statusMessagePodDeleted           = "The pod may have been deleted from the provider"
	containerExitCodePodDeleted int32 = 0
)

// ACIProvider implements the virtual-kubelet provider interface and communicates with Azure's ACI APIs.
type ACIProvider struct {
	aciClient                *aci.Client
	resourceManager          *manager.ResourceManager
	resourceGroup            string
	region                   string
	nodeName                 string
	operatingSystem          string
	cpu                      string
	memory                   string
	pods                     string
	gpu                      string
	gpuSKUs                  []aci.GPUSKU
	internalIP               string
	daemonEndpointPort       int32
	diagnostics              *aci.ContainerGroupDiagnostics
	subnetName               string
	subnetCIDR               string
	vnetSubscriptionID       string
	vnetName                 string
	vnetResourceGroup        string
	clusterDomain            string
	kubeProxyExtension       *aci.Extension
	realtimeMetricsExtension *aci.Extension
	kubeDNSIP                string
	extraUserAgent           string
	retryConfig              client.HTTPRetryConfig
	tracker                  *PodsTracker

	metrics.ACIPodMetricsProvider
}

// AuthConfig is the secret returned from an ImageRegistryCredential
type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	Email         string `json:"email,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// See https://azure.microsoft.com/en-us/status/ for valid regions.
var validAciRegions = []string{
	"australiaeast",
	"brazilsouth",
	"canadacentral",
	"centralindia",
	"centralus",
	"centraluseuap",
	"chinaeast2",
	"eastasia",
	"eastus",
	"eastus2",
	"eastus2euap",
	"francecentral",
	"germanywestcentral",
	"japaneast",
	"koreacentral",
	"northcentralus",
	"northeurope",
	"southcentralus",
	"southeastasia",
	"southindia",
	"switzerlandnorth",
	"uksouth",
	"uaenorth",
	"westcentralus",
	"westeurope",
	"westus",
	"westus2",
	"usgovvirginia",
	"usgovarizona",
}

// isValidACIRegion checks to make sure we're using a valid ACI region
func isValidACIRegion(region string) bool {
	regionLower := strings.ToLower(region)
	regionTrimmed := strings.Replace(regionLower, " ", "", -1)

	for _, validRegion := range validAciRegions {
		if regionTrimmed == validRegion {
			return true
		}
	}

	return false
}

// NewACIProvider creates a new ACIProvider.
func NewACIProvider(config string, rm *manager.ResourceManager, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32, clusterDomain string) (*ACIProvider, error) {
	var p ACIProvider
	var err error

	p.resourceManager = rm
	p.clusterDomain = clusterDomain

	if config != "" {
		f, err := os.Open(config)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		if err := p.loadConfig(f); err != nil {
			return nil, err
		}
	}

	var azAuth *client.Authentication

	if authFilepath := os.Getenv("AZURE_AUTH_LOCATION"); authFilepath != "" {
		auth, err := client.NewAuthenticationFromFile(authFilepath)
		if err != nil {
			return nil, err
		}

		azAuth = auth
	}

	if acsFilepath := os.Getenv("ACS_CREDENTIAL_LOCATION"); acsFilepath != "" {
		acsCredential, err := NewAcsCredential(acsFilepath)
		if err != nil {
			return nil, err
		}

		if acsCredential != nil {
			var clientId string
			if !strings.EqualFold(acsCredential.ClientID, "msi") {
				clientId = acsCredential.ClientID
			}

			azAuth = client.NewAuthentication(
				acsCredential.Cloud,
				clientId,
				acsCredential.ClientSecret,
				acsCredential.SubscriptionID,
				acsCredential.TenantID,
				acsCredential.UserAssignedIdentityID)

			p.resourceGroup = acsCredential.ResourceGroup
			p.region = acsCredential.Region

			p.vnetName = acsCredential.VNetName
			p.vnetResourceGroup = acsCredential.VNetResourceGroup
			if p.vnetResourceGroup == "" {
				p.vnetResourceGroup = p.resourceGroup
			}
		}
	}

	if clientID := os.Getenv("AZURE_CLIENT_ID"); clientID != "" {
		azAuth.ClientID = clientID
	}

	if clientSecret := os.Getenv("AZURE_CLIENT_SECRET"); clientSecret != "" {
		azAuth.ClientSecret = clientSecret
	}

	if userIdentityClientId := os.Getenv("VIRTUALNODE_USER_IDENTITY_CLIENTID"); userIdentityClientId != "" {
		azAuth.UserIdentityClientId = userIdentityClientId
	}
	azAuth.UseUserIdentity = (len(azAuth.ClientID) == 0)

	if azAuth.UseUserIdentity {
		if len(azAuth.UserIdentityClientId) == 0 {
			return nil, fmt.Errorf("Neither AZURE_CLIENT_ID or VIRTUALNODE_USER_IDENTITY_CLIENTID is being set")
		}

		log.G(context.TODO()).Info("Using user identity for authentication")
	}

	if tenantID := os.Getenv("AZURE_TENANT_ID"); tenantID != "" {
		azAuth.TenantID = tenantID
	}

	if subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID"); subscriptionID != "" {
		azAuth.SubscriptionID = subscriptionID
	}

	p.extraUserAgent = os.Getenv("ACI_EXTRA_USER_AGENT")

	retryWaitMin := client.DefaultRetryIntervalMin
	if value := os.Getenv("RETRY_MINIMUM_INTERVAL_IN_SECOND"); value != "" {
		ret, err := strconv.Atoi(value)
		if err == nil {
			return nil, fmt.Errorf("env RETRY_MINIMUM_INTERVAL_IN_SECOND is not able to convert to int, err: %s", err)
		}
		retryWaitMin = time.Duration(ret) * time.Second
	}

	retryWaitMax := client.DefaultRetryIntervalMax
	if value := os.Getenv("RETRY_MAXIMUM_INTERVAL_IN_SECOND"); value != "" {
		ret, err := strconv.Atoi(value)
		if err == nil {
			return nil, fmt.Errorf("env RETRY_MAXIMUM_INTERVAL_IN_SECOND is not able to convert to int, err: %s", err)
		}
		retryWaitMax = time.Duration(ret) * time.Second
	}

	retryMax := client.DefaultRetryMax
	if value := os.Getenv("RETRY_MAXIMUM_COUNT"); value != "" {
		ret, err := strconv.Atoi(value)
		if err == nil {
			return nil, fmt.Errorf("env RETRY_MAXIMUM_COUNT is not able to convert to int, err: %s", err)
		}
		retryMax = ret
	}

	p.retryConfig = client.HTTPRetryConfig{
		RetryWaitMin: retryWaitMin,
		RetryWaitMax: retryWaitMax,
		RetryMax:     retryMax,
	}

	p.aciClient, err = aci.NewClient(azAuth, p.extraUserAgent, p.retryConfig)
	if err != nil {
		return nil, err
	}

	// If the log analytics file has been specified, load workspace credentials from the file
	if logAnalyticsAuthFile := os.Getenv("LOG_ANALYTICS_AUTH_LOCATION"); logAnalyticsAuthFile != "" {
		p.diagnostics, err = aci.NewContainerGroupDiagnosticsFromFile(logAnalyticsAuthFile)
		if err != nil {
			return nil, err
		}
	}

	// If we have both the log analytics workspace id and key, add them to the provider
	// Environment variables overwrite the values provided in the file
	if logAnalyticsID := os.Getenv("LOG_ANALYTICS_ID"); logAnalyticsID != "" {
		if logAnalyticsKey := os.Getenv("LOG_ANALYTICS_KEY"); logAnalyticsKey != "" {
			p.diagnostics, err = aci.NewContainerGroupDiagnostics(logAnalyticsID, logAnalyticsKey)
			if err != nil {
				return nil, err
			}
		}
	}

	if clusterResourceID := os.Getenv("CLUSTER_RESOURCE_ID"); clusterResourceID != "" {
		if p.diagnostics != nil && p.diagnostics.LogAnalytics != nil {
			p.diagnostics.LogAnalytics.LogType = aci.LogAnlyticsLogTypeContainerInsights
			p.diagnostics.LogAnalytics.Metadata = map[string]string{
				aci.LogAnalyticsMetadataKeyClusterResourceID: clusterResourceID,
				aci.LogAnalyticsMetadataKeyNodeName:          nodeName,
			}
		}
	}

	if rg := os.Getenv("ACI_RESOURCE_GROUP"); rg != "" {
		p.resourceGroup = rg
	}
	if p.resourceGroup == "" {
		return nil, errors.New("Resource group can not be empty please set ACI_RESOURCE_GROUP")
	}

	if r := os.Getenv("ACI_REGION"); r != "" {
		p.region = r
	}
	if p.region == "" {
		return nil, errors.New("Region can not be empty please set ACI_REGION")
	}
	if r := p.region; !isValidACIRegion(r) {
		unsupportedRegionMessage := fmt.Sprintf("Region %s is invalid. Current supported regions are: %s",
			r, strings.Join(validAciRegions, ", "))

		return nil, errors.New(unsupportedRegionMessage)
	}

	if err := p.setupCapacity(context.TODO()); err != nil {
		return nil, err
	}

	p.operatingSystem = operatingSystem
	p.nodeName = nodeName
	p.internalIP = internalIP
	p.daemonEndpointPort = daemonEndpointPort

	// the VNET subscription ID defaultly is authentication subscription ID.
	// We need to override when using cross subscription virtual network resource
	p.vnetSubscriptionID = azAuth.SubscriptionID
	if vnetSubscriptionID := os.Getenv("ACI_VNET_SUBSCRIPTION_ID"); vnetSubscriptionID != "" {
		p.vnetSubscriptionID = vnetSubscriptionID
	}
	if vnetName := os.Getenv("ACI_VNET_NAME"); vnetName != "" {
		p.vnetName = vnetName
	}
	if vnetResourceGroup := os.Getenv("ACI_VNET_RESOURCE_GROUP"); vnetResourceGroup != "" {
		p.vnetResourceGroup = vnetResourceGroup
	}
	if subnetName := os.Getenv("ACI_SUBNET_NAME"); p.vnetName != "" && subnetName != "" {
		p.subnetName = subnetName
	}
	if subnetCIDR := os.Getenv("ACI_SUBNET_CIDR"); subnetCIDR != "" {
		if p.subnetName == "" {
			return nil, fmt.Errorf("subnet CIDR defined but no subnet name, subnet name is required to set a subnet CIDR")
		}
		if _, _, err := net.ParseCIDR(subnetCIDR); err != nil {
			return nil, fmt.Errorf("error parsing provided subnet range: %v", err)
		}
		p.subnetCIDR = subnetCIDR
	}

	if p.subnetName != "" {
		if err := p.setupNetwork(azAuth); err != nil {
			return nil, fmt.Errorf("error setting up network: %v", err)
		}

		masterURI := os.Getenv("MASTER_URI")
		if masterURI == "" {
			masterURI = "10.0.0.1"
		}

		clusterCIDR := os.Getenv("CLUSTER_CIDR")
		if clusterCIDR == "" {
			clusterCIDR = "10.240.0.0/16"
		}

		p.kubeProxyExtension, err = getKubeProxyExtension(serviceAccountSecretMountPath, masterURI, clusterCIDR)
		if err != nil {
			return nil, fmt.Errorf("error creating kube proxy extension: %v", err)
		}

		enableRealTimeMetricsExtension := os.Getenv("ENABLE_REAL_TIME_METRICS")
		if enableRealTimeMetricsExtension == "true" {
			p.realtimeMetricsExtension, err = getRealtimeMetricsExtension()
		}

		p.kubeDNSIP = "10.0.0.10"
		if kubeDNSIP := os.Getenv("KUBE_DNS_IP"); kubeDNSIP != "" {
			p.kubeDNSIP = kubeDNSIP
		}
	}

	p.ACIPodMetricsProvider = *metrics.NewACIPodMetricsProvider(nodeName, p.resourceGroup, p.resourceManager, p.aciClient, p.aciClient)
	return &p, err
}

func (p *ACIProvider) setupCapacity(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "setupCapacity")
	defer span.End()
	logger := log.G(ctx).WithField("method", "setupCapacity")

	// Set sane defaults for Capacity in case config is not supplied
	p.cpu = "10000"
	p.memory = "4Ti"
	p.pods = "5000"

	if cpuQuota := os.Getenv("ACI_QUOTA_CPU"); cpuQuota != "" {
		p.cpu = cpuQuota
	}

	if memoryQuota := os.Getenv("ACI_QUOTA_MEMORY"); memoryQuota != "" {
		p.memory = memoryQuota
	}

	if podsQuota := os.Getenv("ACI_QUOTA_POD"); podsQuota != "" {
		p.pods = podsQuota
	}

	metadata, err := p.aciClient.GetResourceProviderMetadata(ctx)

	if err != nil {
		msg := "Unable to fetch the ACI metadata, skipping GPU availability check. GPU capacity will be disabled"
		logger.WithError(err).Error(msg)
		return nil
	}

	if metadata == nil || metadata.GPURegionalSKUs == nil {
		logger.Warn("ACI GPU capacity is not enabled. GPU capacity will be disabled")
		return nil
	}

	for _, regionalSKU := range metadata.GPURegionalSKUs {
		if strings.EqualFold(regionalSKU.Location, p.region) && len(regionalSKU.SKUs) != 0 {
			p.gpu = "100"
			if gpu := os.Getenv("ACI_QUOTA_GPU"); gpu != "" {
				p.gpu = gpu
			}
			p.gpuSKUs = regionalSKU.SKUs
		}
	}

	return nil
}

func (p *ACIProvider) setupNetwork(auth *client.Authentication) error {
	c, err := network.NewClient(auth, p.extraUserAgent, p.retryConfig)
	if err != nil {
		return fmt.Errorf("error creating azure networking client: %v", err)
	}

	createSubnet := true
	subnet, err := c.GetSubnet(p.vnetSubscriptionID, p.vnetResourceGroup, p.vnetName, p.subnetName)
	if err != nil && !network.IsNotFound(err) {
		return fmt.Errorf("error while looking up subnet: %v", err)
	}
	if network.IsNotFound(err) && p.subnetCIDR == "" {
		return fmt.Errorf("subnet '%s' is not found in vnet '%s' in resource group '%s' and subscription '%s' and subnet CIDR is not specified", p.subnetName, p.vnetName, p.vnetResourceGroup, p.vnetSubscriptionID)
	}
	if err == nil {
		if p.subnetCIDR == "" {
			p.subnetCIDR = *subnet.SubnetPropertiesFormat.AddressPrefix
		}
		if p.subnetCIDR != *subnet.SubnetPropertiesFormat.AddressPrefix {
			return fmt.Errorf("found subnet '%s' using different CIDR: '%s'. desired: '%s'", p.subnetName, *subnet.SubnetPropertiesFormat.AddressPrefix, p.subnetCIDR)
		}
		if subnet.SubnetPropertiesFormat.RouteTable != nil {
			return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance since it references the route table '%s'.", p.subnetName, *subnet.SubnetPropertiesFormat.RouteTable.ID)
		}
		if subnet.SubnetPropertiesFormat.ServiceAssociationLinks != nil {
			for _, l := range *subnet.SubnetPropertiesFormat.ServiceAssociationLinks {
				if l.ServiceAssociationLinkPropertiesFormat != nil {
					if *l.ServiceAssociationLinkPropertiesFormat.LinkedResourceType == subnetDelegationService {
						createSubnet = false
						break
					} else {
						return fmt.Errorf("unable to delegate subnet '%s' to Azure Container Instance as it is used by other Azure resource: '%v'.", p.subnetName, l)
					}
				}
			}
		} else {
			for _, d := range *subnet.SubnetPropertiesFormat.Delegations {
				if d.ServiceDelegationPropertiesFormat != nil && *d.ServiceDelegationPropertiesFormat.ServiceName == subnetDelegationService {
					createSubnet = false
					break
				}
			}
		}
	}

	if createSubnet {
		subnet = network.NewSubnetWithContainerInstanceDelegation(p.subnetName, p.subnetCIDR)
		_, err = c.CreateOrUpdateSubnet(p.vnetSubscriptionID, p.vnetResourceGroup, p.vnetName, subnet)
		if err != nil {
			return fmt.Errorf("error creating subnet: %v", err)
		}
	}
	return nil
}

func getRealtimeMetricsExtension() (*aci.Extension, error) {
	extension := aci.Extension{
		Name: "vk-realtime-metrics",
		Properties: &aci.ExtensionProperties{
			Type:              aci.ExtensionTypeRealtimeMetrics,
			Version:           aci.ExtensionVersion1_0,
			Settings:          map[string]string{},
			ProtectedSettings: map[string]string{},
		},
	}
	return &extension, nil
}

func getKubeProxyExtension(secretPath, masterURI, clusterCIDR string) (*aci.Extension, error) {
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

	extension := aci.Extension{
		Name: "kube-proxy",
		Properties: &aci.ExtensionProperties{
			Type:    aci.ExtensionTypeKubeProxy,
			Version: aci.ExtensionVersion1_0,
			Settings: map[string]string{
				aci.KubeProxyExtensionSettingClusterCIDR: clusterCIDR,
				aci.KubeProxyExtensionSettingKubeVersion: aci.KubeProxyExtensionKubeVersion,
			},
			ProtectedSettings: map[string]string{
				aci.KubeProxyExtensionSettingKubeConfig: base64.StdEncoding.EncodeToString(b.Bytes()),
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

func addAzureAttributes(ctx context.Context, span trace.Span, p *ACIProvider) context.Context {
	return span.WithFields(ctx, log.Fields{
		"azure.resourceGroup": p.resourceGroup,
		"azure.region":        p.region,
	})
}

// CreatePod accepts a Pod definition and creates
// an ACI deployment
func (p *ACIProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "aci.CreatePod")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	var containerGroup aci.ContainerGroup
	containerGroup.Location = p.region
	containerGroup.RestartPolicy = aci.ContainerGroupRestartPolicy(pod.Spec.RestartPolicy)
	containerGroup.ContainerGroupProperties.OsType = aci.OperatingSystemTypes(p.operatingSystem)

	// get containers
	containers, err := p.getContainers(pod)
	if err != nil {
		return err
	}
	// get registry creds
	creds, err := p.getImagePullSecrets(pod)
	if err != nil {
		return err
	}
	// get volumes
	volumes, err := p.getVolumes(pod)
	if err != nil {
		return err
	}
	// assign all the things
	containerGroup.ContainerGroupProperties.Containers = containers
	containerGroup.ContainerGroupProperties.Volumes = volumes
	containerGroup.ContainerGroupProperties.ImageRegistryCredentials = creds
	containerGroup.ContainerGroupProperties.Diagnostics = p.getDiagnostics(pod)

	filterServiceAccountSecretVolume(p.operatingSystem, &containerGroup)

	// create ipaddress if containerPort is used
	count := 0
	for _, container := range containers {
		count = count + len(container.Ports)
	}
	ports := make([]aci.Port, 0, count)
	for _, container := range containers {
		for _, containerPort := range container.Ports {

			ports = append(ports, aci.Port{
				Port:     containerPort.Port,
				Protocol: aci.ContainerGroupNetworkProtocol("TCP"),
			})
		}
	}
	if len(ports) > 0 && p.subnetName == "" {
		containerGroup.ContainerGroupProperties.IPAddress = &aci.IPAddress{
			Ports: ports,
			Type:  "Public",
		}

		if dnsNameLabel := pod.Annotations[virtualKubeletDNSNameLabel]; dnsNameLabel != "" {
			containerGroup.ContainerGroupProperties.IPAddress.DNSNameLabel = dnsNameLabel
		}
	}

	podUID := string(pod.UID)
	podCreationTimestamp := pod.CreationTimestamp.String()
	containerGroup.Tags = map[string]string{
		"PodName":           pod.Name,
		"ClusterName":       pod.ClusterName,
		"NodeName":          pod.Spec.NodeName,
		"Namespace":         pod.Namespace,
		"UID":               podUID,
		"CreationTimestamp": podCreationTimestamp,
	}

	p.amendVnetResources(&containerGroup, pod)
	if p.realtimeMetricsExtension != nil {
		containerGroup.ContainerGroupProperties.Extensions = append(containerGroup.ContainerGroupProperties.Extensions, p.realtimeMetricsExtension)
	}

	log.G(ctx).Infof("start creating pod %v", pod.Name)
	// TODO: Run in a go routine to not block workers, and use taracker.UpdatePodStatus() based on result.
	return p.createContainerGroup(ctx, pod.Namespace, pod.Name, &containerGroup)
}

func (p *ACIProvider) createContainerGroup(ctx context.Context, podNS, podName string, cg *aci.ContainerGroup) error {
	ctx, span := trace.StartSpan(ctx, "aci.createContainerGroup")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cgName := containerGroupName(podNS, podName)
	_, err := p.aciClient.CreateContainerGroup(
		ctx,
		p.resourceGroup,
		cgName,
		*cg,
	)

	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to create container group %v", cgName)
	}

	return err
}

func (p *ACIProvider) amendVnetResources(containerGroup *aci.ContainerGroup, pod *v1.Pod) {
	if p.subnetName == "" {
		return
	}

	containerGroup.ContainerGroupProperties.SubnetIds = []*aci.SubnetIdDefinition{&aci.SubnetIdDefinition{ID: "/subscriptions/" + p.vnetSubscriptionID + "/resourceGroups/" + p.vnetResourceGroup + "/providers/Microsoft.Network/virtualNetworks/" + p.vnetName + "/subnets/" + p.subnetName}}
	containerGroup.ContainerGroupProperties.DNSConfig = p.getDNSConfig(pod)
	containerGroup.ContainerGroupProperties.Extensions = []*aci.Extension{p.kubeProxyExtension}
}

func (p *ACIProvider) getDNSConfig(pod *v1.Pod) *aci.DNSConfig {
	nameServers := make([]string, 0)
	searchDomains := []string{}

	// Adding default Azure dns name explicitly
	// if any other dns names are provided by the user ACI will use those instead of azure dns
	// which may cause issues while looking up other Azure resources
	AzureDNSIP := "168.63.129.16"
	if pod.Spec.DNSPolicy == v1.DNSClusterFirst || pod.Spec.DNSPolicy == v1.DNSClusterFirstWithHostNet {
		nameServers = append(nameServers, p.kubeDNSIP)
		nameServers = append(nameServers, AzureDNSIP)
		searchDomains = p.generateSearchesForDNSClusterFirst(pod.Spec.DNSConfig, pod)
	}

	options := []string{}

	if pod.Spec.DNSConfig != nil {
		nameServers = omitDuplicates(append(nameServers, pod.Spec.DNSConfig.Nameservers...))
		searchDomains = omitDuplicates(append(searchDomains, pod.Spec.DNSConfig.Searches...))

		for _, option := range pod.Spec.DNSConfig.Options {
			op := option.Name
			if option.Value != nil && *(option.Value) != "" {
				op = op + ":" + *(option.Value)
			}
			options = append(options, op)
		}
	}

	if len(nameServers) == 0 {
		return nil
	}

	result := aci.DNSConfig{
		NameServers:   formDNSNameserversFitsLimits(nameServers),
		SearchDomains: formDNSSearchFitsLimits(searchDomains),
		Options:       strings.Join(options, " "),
	}

	return &result
}

// This is taken from the kubelet equivalent -  https://github.com/kubernetes/kubernetes/blob/d24fe8a801748953a5c34fd34faa8005c6ad1770/pkg/kubelet/network/dns/dns.go#L141-L151
func (p *ACIProvider) generateSearchesForDNSClusterFirst(dnsConfig *v1.PodDNSConfig, pod *v1.Pod) []string {

	hostSearch := []string{}

	if dnsConfig != nil {
		hostSearch = dnsConfig.Searches
	}
	if p.clusterDomain == "" {
		return hostSearch
	}

	nsSvcDomain := fmt.Sprintf("%s.svc.%s", pod.Namespace, p.clusterDomain)
	svcDomain := fmt.Sprintf("svc.%s", p.clusterDomain)
	clusterSearch := []string{nsSvcDomain, svcDomain, p.clusterDomain}

	return omitDuplicates(append(clusterSearch, hostSearch...))
}

func omitDuplicates(strs []string) []string {
	uniqueStrs := make(map[string]bool)

	var ret []string
	for _, str := range strs {
		if !uniqueStrs[str] {
			ret = append(ret, str)
			uniqueStrs[str] = true
		}
	}
	return ret
}

func formDNSNameserversFitsLimits(nameservers []string) []string {
	if len(nameservers) > maxDNSNameservers {
		nameservers = nameservers[:maxDNSNameservers]
		msg := fmt.Sprintf("Nameserver limits were exceeded, some nameservers have been omitted, the applied nameserver line is: %s", strings.Join(nameservers, ";"))
		log.G(context.TODO()).WithField("method", "formDNSNameserversFitsLimits").Warn(msg)
	}
	return nameservers
}

func formDNSSearchFitsLimits(searches []string) string {
	limitsExceeded := false

	if len(searches) > maxDNSSearchPaths {
		searches = searches[:maxDNSSearchPaths]
		limitsExceeded = true
	}

	if resolvSearchLineStrLen := len(strings.Join(searches, " ")); resolvSearchLineStrLen > maxDNSSearchListChars {
		cutDomainsNum := 0
		cutDomainsLen := 0
		for i := len(searches) - 1; i >= 0; i-- {
			cutDomainsLen += len(searches[i]) + 1
			cutDomainsNum++

			if (resolvSearchLineStrLen - cutDomainsLen) <= maxDNSSearchListChars {
				break
			}
		}

		searches = searches[:(len(searches) - cutDomainsNum)]
		limitsExceeded = true
	}

	if limitsExceeded {
		msg := fmt.Sprintf("Search Line limits were exceeded, some search paths have been omitted, the applied search line is: %s", strings.Join(searches, ";"))
		log.G(context.TODO()).WithField("method", "formDNSSearchFitsLimits").Warn(msg)
	}

	return strings.Join(searches, " ")
}

func (p *ACIProvider) getDiagnostics(pod *v1.Pod) *aci.ContainerGroupDiagnostics {
	if p.diagnostics != nil && p.diagnostics.LogAnalytics != nil && p.diagnostics.LogAnalytics.LogType == aci.LogAnlyticsLogTypeContainerInsights {
		d := *p.diagnostics
		d.LogAnalytics.Metadata[aci.LogAnalyticsMetadataKeyPodUUID] = string(pod.ObjectMeta.UID)
		return &d
	}
	return p.diagnostics
}

func containerGroupName(podNS, podName string) string {
	return fmt.Sprintf("%s-%s", podNS, podName)
}

// UpdatePod is a noop, ACI currently does not support live updates of a pod.
func (p *ACIProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	return nil
}

// DeletePod deletes the specified pod out of ACI.
func (p *ACIProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "aci.DeletePod")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	log.G(ctx).Infof("start deleting pod %v", pod.Name)
	// TODO: Run in a go routine to not block workers.
	return p.deleteContainerGroup(ctx, pod.Namespace, pod.Name)
}

func (p *ACIProvider) deleteContainerGroup(ctx context.Context, podNS, podName string) error {
	ctx, span := trace.StartSpan(ctx, "aci.deleteContainerGroup")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cgName := containerGroupName(podNS, podName)
	err := p.aciClient.DeleteContainerGroup(ctx, p.resourceGroup, cgName)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to delete container group %v", cgName)
		return err
	}

	if p.tracker != nil {
		// Delete is not an sync API on ACI yet, but will assume with current implementation that termination is completed. Also, till gracePeriod is supported.
		updateErr := p.tracker.UpdatePodStatus(
			podNS,
			podName,
			func(podStatus *v1.PodStatus) {
				now := metav1.NewTime(time.Now())
				for i := range podStatus.ContainerStatuses {
					if podStatus.ContainerStatuses[i].State.Running == nil {
						continue
					}

					podStatus.ContainerStatuses[i].State.Terminated = &v1.ContainerStateTerminated{
						ExitCode:    containerExitCodePodDeleted,
						Reason:      statusReasonPodDeleted,
						Message:     statusMessagePodDeleted,
						FinishedAt:  now,
						StartedAt:   podStatus.ContainerStatuses[i].State.Running.StartedAt,
						ContainerID: podStatus.ContainerStatuses[i].ContainerID,
					}
					podStatus.ContainerStatuses[i].State.Running = nil
				}
			},
			false,
		)

		if updateErr != nil && !errdefs.IsNotFound(updateErr) {
			log.G(ctx).WithError(updateErr).Errorf("failed to update termination status for cg %v", cgName)
		}
	}

	return nil
}

// GetPod returns a pod by name that is running inside ACI
// returns nil if a pod by that name is not found.
func (p *ACIProvider) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetPod")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg, err := p.getContainerGroup(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	return containerGroupToPod(cg)
}

// GetContainerLogs returns the logs of a pod by name that is running inside ACI.
func (p *ACIProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerLogs")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg, err := p.getContainerGroup(ctx, namespace, podName)
	if err != nil {
		return nil, err
	}

	// get logs from cg
	retry := 10
	logContent := ""
	var retries int
	for retries = 0; retries < retry; retries++ {
		cLogs, err := p.aciClient.GetContainerLogs(ctx, p.resourceGroup, cg.Name, containerName, opts.Tail)
		if err != nil {
			log.G(ctx).WithField("method", "GetContainerLogs").WithError(err).Debug("Error getting container logs, retrying")
			time.Sleep(5000 * time.Millisecond)
		} else {
			logContent = cLogs.Content
			break
		}
	}
	return ioutil.NopCloser(strings.NewReader(logContent)), err
}

// GetPodFullName as defined in the provider context
func (p *ACIProvider) GetPodFullName(namespace string, pod string) string {
	return fmt.Sprintf("%s-%s", namespace, pod)
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *ACIProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	out := attach.Stdout()
	if out != nil {
		defer out.Close()
	}

	cg, err := p.getContainerGroup(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Set default terminal size
	size := api.TermSize{
		Height: 60,
		Width:  120,
	}

	resize := attach.Resize()
	if resize != nil {
		select {
		case size = <-resize:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	ts := aci.TerminalSizeRequest{Height: int(size.Height), Width: int(size.Width)}
	xcrsp, err := p.aciClient.LaunchExec(p.resourceGroup, cg.Name, container, strings.Join(cmd, " "), ts)
	if err != nil {
		return err
	}

	wsURI := xcrsp.WebSocketURI
	password := xcrsp.Password

	c, _, _ := websocket.DefaultDialer.Dial(wsURI, nil)
	if err := c.WriteMessage(websocket.TextMessage, []byte(password)); err != nil { // Websocket password needs to be sent before WS terminal is active
		panic(err)
	}

	// Cleanup on exit
	defer c.Close()

	in := attach.Stdin()
	if in != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				var msg = make([]byte, 512)
				n, err := in.Read(msg)
				if err != nil {
					// Handle errors
					return
				}
				if n > 0 { // Only call WriteMessage if there is data to send
					if err := c.WriteMessage(websocket.BinaryMessage, msg[:n]); err != nil {
						panic(err)
					}
				}
			}
		}()
	}

	if out != nil {
		for {
			select {
			case <-ctx.Done():
				break
			default:
			}

			_, cr, err := c.NextReader()
			if err != nil {
				// Handle errors
				break
			}
			if _, err := io.Copy(out, cr); err != nil {
				panic(err)
			}
		}
	}

	return ctx.Err()
}

// ConfigureNode enables a provider to configure the node object that
// will be used for Kubernetes.
func (p *ACIProvider) ConfigureNode(ctx context.Context, node *v1.Node) {
	node.Status.Capacity = p.capacity()
	node.Status.Allocatable = p.capacity()
	node.Status.Conditions = p.nodeConditions()
	node.Status.Addresses = p.nodeAddresses()
	node.Status.DaemonEndpoints = p.nodeDaemonEndpoints()
	node.Status.NodeInfo.OperatingSystem = p.operatingSystem
	node.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
	node.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"

	// Virtual node would be skipped for cloud provider operations (e.g. CP should not add route).
	node.ObjectMeta.Labels["kubernetes.azure.com/managed"] = "false"
}

// GetPodStatus returns the status of a pod by name that is running inside ACI
// returns nil if a pod by that name is not found.
func (p *ACIProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetPodStatus")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg, err := p.getContainerGroup(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	return podStatusFromContainerGroup(cg), nil
}

// GetPods returns a list of all pods known to be running within ACI.
func (p *ACIProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetPods")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cgs, err := p.aciClient.ListContainerGroups(ctx, p.resourceGroup)
	if err != nil {
		return nil, err
	}
	pods := make([]*v1.Pod, 0, len(cgs.Value))

	for _, cg := range cgs.Value {
		c := cg
		if cg.Tags["NodeName"] != p.nodeName {
			continue
		}

		p, err := containerGroupToPod(&c)
		if err != nil {
			log.G(ctx).WithFields(log.Fields{
				"name": c.Name,
				"id":   c.ID,
			}).WithError(err).Error("error converting container group to pod")

			continue
		}
		pods = append(pods, p)
	}

	return pods, nil
}

// NotifyPods instructs the notifier to call the passed in function when
// the pod status changes.
// The provided pointer to a Pod is guaranteed to be used in a read-only
// fashion.
func (p *ACIProvider) NotifyPods(ctx context.Context, notifierCb func(*v1.Pod)) {
	ctx, span := trace.StartSpan(ctx, "ACIProvider.NotifyPods")
	defer span.End()

	// Capture the notifier to be used for communicating updates to VK
	p.tracker = &PodsTracker{
		rm:       p.resourceManager,
		updateCb: notifierCb,
		handler:  p,
	}

	go p.tracker.StartTracking(ctx)
}

// PodsTrackerHandler interface impl.
func (p *ACIProvider) ListActivePods(ctx context.Context) ([]PodIdentifier, error) {
	providerPods, err := p.GetPods(ctx)
	if err != nil {
		return nil, err
	}

	podsIdentifiers := make([]PodIdentifier, 0, len(providerPods))
	for _, pod := range providerPods {
		podsIdentifiers = append(
			podsIdentifiers,
			PodIdentifier{
				namespace: pod.Namespace,
				name:      pod.Name,
			})
	}

	return podsIdentifiers, nil
}

func (p *ACIProvider) FetchPodStatus(ctx context.Context, ns, name string) (*v1.PodStatus, error) {
	return p.GetPodStatus(ctx, ns, name)
}

func (p *ACIProvider) CleanupPod(ctx context.Context, ns, name string) error {
	return p.deleteContainerGroup(ctx, ns, name)
}

// implement NodeProvider

// Ping checks if the node is still active/ready.
func (p *ACIProvider) Ping(ctx context.Context) error {
	return nil
}

// getContainerGroup returns a container group from ACI.
func (p *ACIProvider) getContainerGroup(ctx context.Context, namespace, name string) (*aci.ContainerGroup, error) {
	cg, status, err := p.aciClient.GetContainerGroup(ctx, p.resourceGroup, fmt.Sprintf("%s-%s", namespace, name))
	if err != nil {
		if status != nil && *status == http.StatusNotFound {
			return nil, errdefs.NotFound("cg not found")
		}
		return nil, err
	}

	if cg.Tags["NodeName"] != p.nodeName {
		return nil, errdefs.NotFound("cg found with mismatching node")
	}

	return cg, nil
}

// capacity returns a resource list containing the capacity limits set for ACI.
func (p *ACIProvider) capacity() v1.ResourceList {
	resourceList := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse(p.cpu),
		v1.ResourceMemory: resource.MustParse(p.memory),
		v1.ResourcePods:   resource.MustParse(p.pods),
	}

	if p.gpu != "" {
		resourceList[gpuResourceName] = resource.MustParse(p.gpu)
	}

	return resourceList
}

// nodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func (p *ACIProvider) nodeConditions() []v1.NodeCondition {
	// TODO: Make these dynamic and augment with custom ACI specific conditions of interest
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionTrue,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is ready.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}
}

// nodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *ACIProvider) nodeAddresses() []v1.NodeAddress {
	// TODO: Make these dynamic and augment with custom ACI specific conditions of interest
	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: p.internalIP,
		},
	}
}

// nodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *ACIProvider) nodeDaemonEndpoints() v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}

func (p *ACIProvider) getImagePullSecrets(pod *v1.Pod) ([]aci.ImageRegistryCredential, error) {
	ips := make([]aci.ImageRegistryCredential, 0, len(pod.Spec.ImagePullSecrets))
	for _, ref := range pod.Spec.ImagePullSecrets {
		secret, err := p.resourceManager.GetSecret(ref.Name, pod.Namespace)
		if err != nil {
			return ips, err
		}
		if secret == nil {
			return nil, fmt.Errorf("error getting image pull secret")
		}
		switch secret.Type {
		case v1.SecretTypeDockercfg:
			ips, err = readDockerCfgSecret(secret, ips)
		case v1.SecretTypeDockerConfigJson:
			ips, err = readDockerConfigJSONSecret(secret, ips)
		default:
			return nil, fmt.Errorf("image pull secret type is not one of kubernetes.io/dockercfg or kubernetes.io/dockerconfigjson")
		}

		if err != nil {
			return ips, err
		}

	}
	return ips, nil
}

func makeRegistryCredential(server string, authConfig AuthConfig) (*aci.ImageRegistryCredential, error) {
	username := authConfig.Username
	password := authConfig.Password

	if username == "" {
		if authConfig.Auth == "" {
			return nil, fmt.Errorf("no username present in auth config for server: %s", server)
		}

		decoded, err := base64.StdEncoding.DecodeString(authConfig.Auth)
		if err != nil {
			return nil, fmt.Errorf("error decoding the auth for server: %s Error: %v", server, err)
		}

		parts := strings.Split(string(decoded), ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed auth for server: %s", server)
		}

		username = parts[0]
		password = parts[1]
	}

	cred := aci.ImageRegistryCredential{
		Server:   server,
		Username: username,
		Password: password,
	}

	return &cred, nil
}

func makeRegistryCredentialFromDockerConfig(server string, configEntry DockerConfigEntry) (*aci.ImageRegistryCredential, error) {
	if configEntry.Username == "" {
		return nil, fmt.Errorf("no username present in auth config for server: %s", server)
	}

	cred := aci.ImageRegistryCredential{
		Server:   server,
		Username: configEntry.Username,
		Password: configEntry.Password,
	}

	return &cred, nil
}

func readDockerCfgSecret(secret *v1.Secret, ips []aci.ImageRegistryCredential) ([]aci.ImageRegistryCredential, error) {
	var err error
	var authConfigs map[string]AuthConfig
	repoData, ok := secret.Data[string(v1.DockerConfigKey)]

	if !ok {
		return ips, fmt.Errorf("no dockercfg present in secret")
	}

	err = json.Unmarshal(repoData, &authConfigs)
	if err != nil {
		return ips, err
	}

	for server := range authConfigs {
		cred, err := makeRegistryCredential(server, authConfigs[server])
		if err != nil {
			return ips, err
		}

		ips = append(ips, *cred)
	}

	return ips, err
}

func readDockerConfigJSONSecret(secret *v1.Secret, ips []aci.ImageRegistryCredential) ([]aci.ImageRegistryCredential, error) {
	var err error
	repoData, ok := secret.Data[string(v1.DockerConfigJsonKey)]

	if !ok {
		return ips, fmt.Errorf("no dockerconfigjson present in secret")
	}

	// Will use K8s config models to handle marshaling (including auth field handling).
	var cfgJson DockerConfigJSON

	err = json.Unmarshal(repoData, &cfgJson)
	if err != nil {
		return ips, err
	}

	auths := cfgJson.Auths
	if len(cfgJson.Auths) == 0 {
		return ips, fmt.Errorf("malformed dockerconfigjson in secret")
	}

	for server := range auths {
		cred, err := makeRegistryCredentialFromDockerConfig(server, auths[server])
		if err != nil {
			return ips, err
		}

		ips = append(ips, *cred)
	}

	return ips, err
}

func (p *ACIProvider) getContainers(pod *v1.Pod) ([]aci.Container, error) {
	containers := make([]aci.Container, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {

		if len(container.Command) == 0 && len(container.Args) > 0 {
			return nil, errdefs.InvalidInput("ACI does not support providing args without specifying the command. Please supply both command and args to the pod spec.")
		}
		c := aci.Container{
			Name: container.Name,
			ContainerProperties: aci.ContainerProperties{
				Image:   container.Image,
				Command: append(container.Command, container.Args...),
				Ports:   make([]aci.ContainerPort, 0, len(container.Ports)),
			},
		}

		for _, p := range container.Ports {
			c.Ports = append(c.Ports, aci.ContainerPort{
				Port:     p.ContainerPort,
				Protocol: getProtocol(p.Protocol),
			})
		}

		c.VolumeMounts = make([]aci.VolumeMount, 0, len(container.VolumeMounts))
		for _, v := range container.VolumeMounts {
			c.VolumeMounts = append(c.VolumeMounts, aci.VolumeMount{
				Name:      v.Name,
				MountPath: v.MountPath,
				ReadOnly:  v.ReadOnly,
			})
		}

		c.EnvironmentVariables = make([]aci.EnvironmentVariable, 0, len(container.Env))
		for _, e := range container.Env {
			if e.Value != "" {
				envVar := getACIEnvVar(e)
				c.EnvironmentVariables = append(c.EnvironmentVariables, envVar)
			}
		}

		// NOTE(robbiezhang): ACI CPU request must be times of 10m
		cpuRequest := 1.00
		if _, ok := container.Resources.Requests[v1.ResourceCPU]; ok {
			cpuRequest = float64(container.Resources.Requests.Cpu().MilliValue()/10.00) / 100.00
			if cpuRequest < 0.01 {
				cpuRequest = 0.01
			}
		}

		// NOTE(robbiezhang): ACI memory request must be times of 0.1 GB
		memoryRequest := 1.50
		if _, ok := container.Resources.Requests[v1.ResourceMemory]; ok {
			memoryRequest = float64(container.Resources.Requests.Memory().Value()/100000000.00) / 10.00
			if memoryRequest < 0.10 {
				memoryRequest = 0.10
			}
		}

		c.Resources = aci.ResourceRequirements{
			Requests: &aci.ComputeResources{
				CPU:        cpuRequest,
				MemoryInGB: memoryRequest,
			},
		}

		if container.Resources.Limits != nil {
			cpuLimit := cpuRequest
			if _, ok := container.Resources.Limits[v1.ResourceCPU]; ok {
				cpuLimit = float64(container.Resources.Limits.Cpu().MilliValue()) / 1000.00
			}

			// NOTE(jahstreet): ACI memory limit must be times of 0.1 GB
			memoryLimit := memoryRequest
			if _, ok := container.Resources.Limits[v1.ResourceMemory]; ok {
				memoryLimit = float64(container.Resources.Limits.Memory().Value()/100000000.00) / 10.00
			}
			c.Resources.Limits = &aci.ComputeResources{
				CPU:        cpuLimit,
				MemoryInGB: memoryLimit,
			}

			if gpu, ok := container.Resources.Limits[gpuResourceName]; ok {
				sku, err := p.getGPUSKU(pod)
				if err != nil {
					return nil, err
				}

				if gpu.Value() == 0 {
					return nil, errors.New("GPU must be a integer number")
				}

				gpuResource := &aci.GPUResource{
					Count: int32(gpu.Value()),
					SKU:   sku,
				}

				c.Resources.Requests.GPU = gpuResource
				c.Resources.Limits.GPU = gpuResource
			}
		}

		if container.LivenessProbe != nil {
			probe, err := getProbe(container.LivenessProbe, container.Ports)
			if err != nil {
				return nil, err
			}
			c.LivenessProbe = probe
		}

		if container.ReadinessProbe != nil {
			probe, err := getProbe(container.ReadinessProbe, container.Ports)
			if err != nil {
				return nil, err
			}
			c.ReadinessProbe = probe
		}

		containers = append(containers, c)
	}
	return containers, nil
}

func (p *ACIProvider) getGPUSKU(pod *v1.Pod) (aci.GPUSKU, error) {
	if len(p.gpuSKUs) == 0 {
		return "", fmt.Errorf("The pod requires GPU resource, but ACI doesn't provide GPU enabled container group in region %s", p.region)
	}

	if desiredSKU, ok := pod.Annotations[gpuTypeAnnotation]; ok {
		for _, supportedSKU := range p.gpuSKUs {
			if strings.EqualFold(string(desiredSKU), string(supportedSKU)) {
				return supportedSKU, nil
			}
		}

		return "", fmt.Errorf("The pod requires GPU SKU %s, but ACI only supports SKUs %v in region %s", desiredSKU, p.region, p.gpuSKUs)
	}

	return p.gpuSKUs[0], nil
}

func getProbe(probe *v1.Probe, ports []v1.ContainerPort) (*aci.ContainerProbe, error) {

	if probe.Handler.Exec != nil && probe.Handler.HTTPGet != nil {
		return nil, fmt.Errorf("probe may not specify more than one of \"exec\" and \"httpGet\"")
	}

	if probe.Handler.Exec == nil && probe.Handler.HTTPGet == nil {
		return nil, fmt.Errorf("probe must specify one of \"exec\" and \"httpGet\"")
	}

	// Probes have can have a Exec or HTTP Get Handler.
	// Create those if they exist, then add to the
	// ContainerProbe struct
	var exec *aci.ContainerExecProbe
	if probe.Handler.Exec != nil {
		exec = &aci.ContainerExecProbe{
			Command: probe.Handler.Exec.Command,
		}
	}

	var httpGET *aci.ContainerHTTPGetProbe
	if probe.Handler.HTTPGet != nil {
		var portValue int
		port := probe.Handler.HTTPGet.Port
		switch port.Type {
		case intstr.Int:
			portValue = port.IntValue()
		case intstr.String:
			portName := port.String()
			for _, p := range ports {
				if portName == p.Name {
					portValue = int(p.ContainerPort)
					break
				}
			}
			if portValue == 0 {
				return nil, fmt.Errorf("unable to find named port: %s", portName)
			}
		}

		httpGET = &aci.ContainerHTTPGetProbe{
			Port:   portValue,
			Path:   probe.Handler.HTTPGet.Path,
			Scheme: string(probe.Handler.HTTPGet.Scheme),
		}
	}

	return &aci.ContainerProbe{
		Exec:                exec,
		HTTPGet:             httpGET,
		InitialDelaySeconds: probe.InitialDelaySeconds,
		Period:              probe.PeriodSeconds,
		FailureThreshold:    probe.FailureThreshold,
		SuccessThreshold:    probe.SuccessThreshold,
		TimeoutSeconds:      probe.TimeoutSeconds,
	}, nil
}

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

func (p *ACIProvider) getVolumes(pod *v1.Pod) ([]azaci.Volume, error) {
	volumes := make([]azaci.Volume, 0, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		// Handle the case for Azure File CSI driver
		if v.CSI != nil {
			// Check if the CSI driver is file (Disk is not supported by ACI)
			if v.CSI.Driver == AzureFileDriverName {
				csiVolume, err := p.getAzureFileCSI(v, pod.Namespace)
				if err != nil {
					return nil, err
				}
				volumes = append(volumes, *csiVolume)
				continue
			} else {
				return nil, fmt.Errorf("pod %s requires volume %s which is of an unsupported type %s", pod.Name, v.Name, v.CSI.Driver)
			}
		}

		// Handle the case for the AzureFile volume.
		if v.AzureFile != nil {
			secret, err := p.resourceManager.GetSecret(v.AzureFile.SecretName, pod.Namespace)
			if err != nil {
				return volumes, err
			}

			if secret == nil {
				return nil, fmt.Errorf("Getting secret for AzureFile volume returned an empty secret")
			}
			storageAccountNameStr := string(secret.Data[azureFileStorageAccountName])
			storageAccountKeyStr := string(secret.Data[azureFileStorageAccountKey])

			volumes = append(volumes, azaci.Volume{
				Name: &v.Name,
				AzureFile: &azaci.AzureFileVolume{
					ShareName:          &v.AzureFile.ShareName,
					ReadOnly:           &v.AzureFile.ReadOnly,
					StorageAccountName: &storageAccountNameStr,
					StorageAccountKey:  &storageAccountKeyStr,
				},
			})
			continue
		}

		// Handle the case for the EmptyDir.
		if v.EmptyDir != nil {
			volumes = append(volumes, azaci.Volume{
				Name:     &v.Name,
				EmptyDir: map[string]interface{}{},
			})
			continue
		}

		// Handle the case for GitRepo volume.
		if v.GitRepo != nil {
			volumes = append(volumes, azaci.Volume{
				Name: &v.Name,
				GitRepo: &azaci.GitRepoVolume{
					Directory:  &v.GitRepo.Directory,
					Repository: &v.GitRepo.Repository,
					Revision:   &v.GitRepo.Revision,
				},
			})
			continue
		}

		// Handle the case for Secret volume.
		if v.Secret != nil {
			paths := make(map[string]*string)
			secret, err := p.resourceManager.GetSecret(v.Secret.SecretName, pod.Namespace)
			if v.Secret.Optional != nil && !*v.Secret.Optional && k8serr.IsNotFound(err) {
				return nil, fmt.Errorf("Secret %s is required by Pod %s and does not exist", v.Secret.SecretName, pod.Name)
			}
			if secret == nil {
				continue
			}

			for k, v := range secret.Data {
				strV := base64.StdEncoding.EncodeToString(v)
				paths[k] = &strV
			}

			if len(paths) != 0 {
				volumes = append(volumes, azaci.Volume{
					Name:   &v.Name,
					Secret: paths,
				})
			}
			continue
		}

		// Handle the case for ConfigMap volume.
		if v.ConfigMap != nil {
			paths := make(map[string]*string)
			configMap, err := p.resourceManager.GetConfigMap(v.ConfigMap.Name, pod.Namespace)
			if v.ConfigMap.Optional != nil && !*v.ConfigMap.Optional && k8serr.IsNotFound(err) {
				return nil, fmt.Errorf("ConfigMap %s is required by Pod %s and does not exist", v.ConfigMap.Name, pod.Name)
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
				volumes = append(volumes, azaci.Volume{
					Name:   &v.Name,
					Secret: paths,
				})
			}
			continue
		}

		if v.Projected != nil {
			log.G(context.TODO()).Info("Found projected volume")
			paths := make(map[string]*string)

			for _, source := range v.Projected.Sources {
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
				volumes = append(volumes, azaci.Volume{
					Name:   &v.Name,
					Secret: paths,
				})
			}

			continue
		}

		// If we've made it this far we have found a volume type that isn't supported
		return nil, fmt.Errorf("Pod %s requires volume %s which is of an unsupported type", pod.Name, v.Name)
	}

	return volumes, nil
}

func getProtocol(pro v1.Protocol) aci.ContainerNetworkProtocol {
	switch pro {
	case v1.ProtocolUDP:
		return aci.ContainerNetworkProtocolUDP
	default:
		return aci.ContainerNetworkProtocolTCP
	}
}

func containerGroupToPod(cg *aci.ContainerGroup) (*v1.Pod, error) {
	_, creationTime := aciResourceMetaFromContainerGroup(cg)

	containers := make([]v1.Container, 0, len(cg.Containers))
	for _, c := range cg.Containers {
		container := v1.Container{
			Name:    c.Name,
			Image:   c.Image,
			Command: c.Command,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", c.Resources.Requests.CPU)),
					v1.ResourceMemory: resource.MustParse(fmt.Sprintf("%gG", c.Resources.Requests.MemoryInGB)),
				},
			},
		}

		if c.Resources.Requests.GPU != nil {
			container.Resources.Requests[gpuResourceName] = resource.MustParse(fmt.Sprintf("%d", c.Resources.Requests.GPU.Count))
		}

		if c.Resources.Limits != nil {
			container.Resources.Limits = v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%g", c.Resources.Limits.CPU)),
				v1.ResourceMemory: resource.MustParse(fmt.Sprintf("%gG", c.Resources.Limits.MemoryInGB)),
			}

			if c.Resources.Limits.GPU != nil {
				container.Resources.Limits[gpuResourceName] = resource.MustParse(fmt.Sprintf("%d", c.Resources.Requests.GPU.Count))
			}
		}

		containers = append(containers, container)
	}

	p := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              cg.Tags["PodName"],
			Namespace:         cg.Tags["Namespace"],
			ClusterName:       cg.Tags["ClusterName"],
			UID:               types.UID(cg.Tags["UID"]),
			CreationTimestamp: creationTime,
		},
		Spec: v1.PodSpec{
			NodeName:   cg.Tags["NodeName"],
			Volumes:    []v1.Volume{},
			Containers: containers,
		},
		Status: *podStatusFromContainerGroup(cg),
	}

	return &p, nil
}

func aciResourceMetaFromContainerGroup(cg *aci.ContainerGroup) (string, metav1.Time) {
	// Use the Provisioning State if it's not Succeeded,
	// otherwise use the state of the instance.
	aciState := cg.ContainerGroupProperties.ProvisioningState
	if aciState == "Succeeded" {
		aciState = cg.ContainerGroupProperties.InstanceView.State
	}

	var creationTime metav1.Time
	if cg.Tags["CreationTimestamp"] != "" {
		t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", cg.Tags["CreationTimestamp"])
		if err == nil {
			creationTime = metav1.NewTime(t)
		}
	}

	return aciState, creationTime
}

func podStatusFromContainerGroup(cg *aci.ContainerGroup) *v1.PodStatus {
	aciState, creationTime := aciResourceMetaFromContainerGroup(cg)
	containerStatuses := make([]v1.ContainerStatus, 0, len(cg.Containers))

	firstContainerStartTime := metav1.NewTime(time.Time(cg.Containers[0].ContainerProperties.InstanceView.CurrentState.StartTime))
	lastUpdateTime := firstContainerStartTime
	allReady := true
	for _, c := range cg.Containers {
		containerStartTime := metav1.NewTime(time.Time(c.ContainerProperties.InstanceView.CurrentState.StartTime))
		containerStatus := v1.ContainerStatus{
			Name:                 c.Name,
			State:                aciContainerStateToContainerState(c.InstanceView.CurrentState),
			LastTerminationState: aciContainerStateToContainerState(c.InstanceView.PreviousState),
			Ready:                aciStateToPodPhase(c.InstanceView.CurrentState.State) == v1.PodRunning,
			RestartCount:         c.InstanceView.RestartCount,
			Image:                c.Image,
			ImageID:              "",
			ContainerID:          getContainerID(cg.ID, c.Name),
		}

		if aciStateToPodPhase(c.InstanceView.CurrentState.State) != v1.PodRunning &&
			aciStateToPodPhase(c.InstanceView.CurrentState.State) != v1.PodSucceeded {
			allReady = false
		}
		if containerStartTime.Time.After(lastUpdateTime.Time) {
			lastUpdateTime = containerStartTime
		}

		// Add to containerStatuses
		containerStatuses = append(containerStatuses, containerStatus)
	}

	ip := ""
	if cg.IPAddress != nil {
		ip = cg.IPAddress.IP
	}

	return &v1.PodStatus{
		Phase:             aciStateToPodPhase(aciState),
		Conditions:        aciStateToPodConditions(aciState, creationTime, lastUpdateTime, allReady),
		Message:           "",
		Reason:            "",
		HostIP:            "",
		PodIP:             ip,
		StartTime:         &firstContainerStartTime,
		ContainerStatuses: containerStatuses,
	}
}

func getContainerID(cgID, containerName string) string {
	if cgID == "" {
		return ""
	}

	containerResourceID := fmt.Sprintf("%s/containers/%s", cgID, containerName)

	h := sha256.New()
	if _, err := h.Write([]byte(strings.ToUpper(containerResourceID))); err != nil {
		panic(err)
	}
	hashBytes := h.Sum(nil)
	return fmt.Sprintf("aci://%s", hex.EncodeToString(hashBytes))
}

func aciStateToPodPhase(state string) v1.PodPhase {
	switch state {
	case "Running":
		return v1.PodRunning
	case "Succeeded":
		return v1.PodSucceeded
	case "Failed":
		return v1.PodFailed
	case "Canceled":
		return v1.PodFailed
	case "Creating":
		return v1.PodPending
	case "Repairing":
		return v1.PodPending
	case "Pending":
		return v1.PodPending
	case "Accepted":
		return v1.PodPending
	}

	return v1.PodUnknown
}

func aciStateToPodConditions(state string, creationTime, lastUpdateTime metav1.Time, allReady bool) []v1.PodCondition {
	switch state {
	case "Running", "Succeeded":
		readyConditionStatus := v1.ConditionFalse
		readyConditionTime := creationTime
		if allReady {
			readyConditionStatus = v1.ConditionTrue
			readyConditionTime = lastUpdateTime
		}

		return []v1.PodCondition{
			{
				Type:               v1.PodReady,
				Status:             readyConditionStatus,
				LastTransitionTime: readyConditionTime,
			}, {
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTime,
			}, {
				Type:               v1.PodScheduled,
				Status:             v1.ConditionTrue,
				LastTransitionTime: creationTime,
			},
		}
	}
	return []v1.PodCondition{}
}

func aciContainerStateToContainerState(cs aci.ContainerState) v1.ContainerState {
	startTime := metav1.NewTime(time.Time(cs.StartTime))

	// Handle the case where the container is running.
	if cs.State == "Running" {
		return v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: startTime,
			},
		}
	}

	// Handle the case of completion.
	if cs.State == "Succeeded" {
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				StartedAt:  startTime,
				Reason:     "Completed",
				FinishedAt: metav1.NewTime(time.Time(cs.FinishTime)),
			},
		}
	}

	// Handle the case where the container failed.
	if cs.State == "Failed" || cs.State == "Canceled" {
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   cs.ExitCode,
				Reason:     cs.State,
				Message:    cs.DetailStatus,
				StartedAt:  startTime,
				FinishedAt: metav1.NewTime(time.Time(cs.FinishTime)),
			},
		}
	}

	state := cs.State
	if state == "" {
		state = "Creating"
	}

	// Handle the case where the container is pending.
	// Which should be all other aci states.
	return v1.ContainerState{
		Waiting: &v1.ContainerStateWaiting{
			Reason:  state,
			Message: cs.DetailStatus,
		},
	}
}

// Filters service account secret volume for Windows.
// Service account secret volume gets automatically turned on if not specified otherwise.
// ACI doesn't support secret volume for Windows, so we need to filter it.
func filterServiceAccountSecretVolume(osType string, containerGroup *aci.ContainerGroup) {
	if strings.EqualFold(osType, "Windows") {
		serviceAccountSecretVolumeName := make(map[string]bool)

		for index, container := range containerGroup.ContainerGroupProperties.Containers {
			volumeMounts := make([]aci.VolumeMount, 0, len(container.VolumeMounts))
			for _, volumeMount := range container.VolumeMounts {
				if !strings.EqualFold(serviceAccountSecretMountPath, volumeMount.MountPath) {
					volumeMounts = append(volumeMounts, volumeMount)
				} else {
					serviceAccountSecretVolumeName[volumeMount.Name] = true
				}
			}
			containerGroup.ContainerGroupProperties.Containers[index].VolumeMounts = volumeMounts
		}

		if len(serviceAccountSecretVolumeName) == 0 {
			return
		}

		l := log.G(context.TODO()).WithField("containerGroup", containerGroup.Name)
		l.Infof("Ignoring service account secret volumes '%v' for Windows", reflect.ValueOf(serviceAccountSecretVolumeName).MapKeys())

		volumes := make([]azaci.Volume, 0, len(containerGroup.ContainerGroupProperties.Volumes))
		for _, volume := range containerGroup.ContainerGroupProperties.Volumes {
			if _, ok := serviceAccountSecretVolumeName[*volume.Name]; !ok {
				volumes = append(volumes, volume)
			}
		}

		containerGroup.ContainerGroupProperties.Volumes = volumes
	}
}

func getACIEnvVar(e v1.EnvVar) aci.EnvironmentVariable {
	var envVar aci.EnvironmentVariable
	// If the variable is a secret, use SecureValue
	if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
		envVar = aci.EnvironmentVariable{
			Name:        e.Name,
			SecureValue: e.Value,
		}
	} else {
		envVar = aci.EnvironmentVariable{
			Name:  e.Name,
			Value: e.Value,
		}
	}
	return envVar
}
