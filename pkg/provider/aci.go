/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	azaciv2 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/azure-aci/pkg/analytics"
	"github.com/virtual-kubelet/azure-aci/pkg/auth"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	"github.com/virtual-kubelet/azure-aci/pkg/featureflag"
	"github.com/virtual-kubelet/azure-aci/pkg/metrics"
	"github.com/virtual-kubelet/azure-aci/pkg/network"
	"github.com/virtual-kubelet/azure-aci/pkg/util"
	"github.com/virtual-kubelet/azure-aci/pkg/validation"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1listers "k8s.io/client-go/listers/core/v1"

	"github.com/cpuguy83/dockercfg"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
)

const (
	// The service account secret mount path.
	serviceAccountSecretMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"

	virtualKubeletDNSNameLabel = "virtualkubelet.io/dnsnamelabel"

	// Parameter names defined in azure file CSI driver, refer to
	// https://github.com/kubernetes-sigs/azurefile-csi-driver/blob/master/docs/driver-parameters.md
	azureFileShareName  = "shareName"
	azureFileSecretName = "secretName"
	// AzureFileDriverName is the name of the CSI driver for Azure File
	AzureFileDriverName         = "file.csi.azure.com"
	azureFileStorageAccountName = "azurestorageaccountname"
	azureFileStorageAccountKey  = "azurestorageaccountkey"
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

const (
	confidentialComputeSkuLabel       = "virtual-kubelet.io/container-sku"
	confidentialComputeCcePolicyLabel = "virtual-kubelet.io/confidential-compute-cce-policy"
)

// ACIProvider implements the virtual-kubelet provider interface and communicates with Azure's ACI APIs.
type ACIProvider struct {
	azClientsAPIs            client.AzClientsInterface
	containerGroupExtensions []*azaciv2.DeploymentExtensionSpec
	secretL                  corev1listers.SecretLister
	configL                  corev1listers.ConfigMapLister
	podsL                    corev1listers.PodLister
	enabledFeatures          *featureflag.FlagIdentifier
	providerNetwork          network.ProviderNetwork
	eventRecorder            record.EventRecorder

	resourceGroup      string
	region             string
	nodeName           string
	operatingSystem    string
	cpu                string
	memory             string
	pods               string
	gpu                string
	gpuSKUs            []azaciv2.GpuSKU
	internalIP         string
	daemonEndpointPort int32
	diagnostics        *azaciv2.ContainerGroupDiagnostics
	clusterDomain      string
	tracker            *PodsTracker

	*metrics.ACIPodMetricsProvider
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

// See https://learn.microsoft.com/en-us/azure/container-instances/container-instances-region-availability
var validAciRegions = []string{
	"australiacentral",
	"australiacentral2",
	"australiaeast",
	"australiasoutheast",
	"brazilsouth",
	"canadacentral",
	"canadaeast",
	"centralindia",
	"centralus",
	"centraluseuap",
	"chinaeast2",
	"chinaeast3",
	"chinanorth3",
	"eastasia",
	"eastus",
	"eastus2",
	"eastus2euap",
	"francecentral",
	"francesouth",
	"germanynorth",
	"germanywestcentral",
	"japaneast",
	"japanwest",
	"jioindiawest",
	"israelcentral",
	"italynorth",
	"koreacentral",
	"koreasouth",
	"northcentralus",
	"northeurope",
	"norwayeast",
	"norwaywest",
	"polandcentral",
	"qatarcentral",
	"southafricanorth",
	"southafricawest",
	"southcentralus",
	"southindia",
	"southeastasia",
	"swedencentral",
	"swedensouth",
	"switzerlandnorth",
	"switzerlandwest",
	"uaenorth",
	"uksouth",
	"ukwest",
	"westcentralus",
	"westeurope",
	"westindia",
	"westus",
	"westus2",
	"westus3",
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
func NewACIProvider(ctx context.Context, config string, azConfig auth.Config, azAPIs client.AzClientsInterface, pCfg nodeutil.ProviderConfig, nodeName, operatingSystem string, internalIP string, daemonEndpointPort int32, clusterDomain string, kubeClient kubernetes.Interface) (*ACIProvider, error) {
	var p ACIProvider
	var err error

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

	p.enabledFeatures = featureflag.InitFeatureFlag(ctx)

	p.azClientsAPIs = azAPIs
	p.configL = pCfg.ConfigMaps
	p.secretL = pCfg.Secrets
	p.podsL = pCfg.Pods
	p.clusterDomain = clusterDomain
	p.operatingSystem = operatingSystem
	p.nodeName = nodeName
	p.internalIP = internalIP
	p.daemonEndpointPort = daemonEndpointPort

	if azConfig.AKSCredential != nil {
		p.resourceGroup = azConfig.AKSCredential.ResourceGroup
		p.region = azConfig.AKSCredential.Region
		p.providerNetwork.VnetName = azConfig.AKSCredential.VNetName
		p.providerNetwork.VnetResourceGroup = azConfig.AKSCredential.VNetResourceGroup
	}

	if p.providerNetwork.VnetResourceGroup == "" {
		p.providerNetwork.VnetResourceGroup = p.resourceGroup
	}

	providerEB := record.NewBroadcaster()
	providerEB.StartLogging(log.G(ctx).Infof)
	providerEB.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: kubeClient.CoreV1().Events(v1.NamespaceAll)})
	p.eventRecorder = providerEB.NewRecorder(scheme.Scheme, v1.EventSource{Component: path.Join(nodeName, "pod-controller")})

	// If the log analytics file has been specified, load workspace credentials from the file
	if logAnalyticsAuthFile := os.Getenv("LOG_ANALYTICS_AUTH_LOCATION"); logAnalyticsAuthFile != "" {
		p.diagnostics, err = analytics.NewContainerGroupDiagnosticsFromFile(logAnalyticsAuthFile)
		if err != nil {
			return nil, err
		}
	}

	// If we have both the log analytics workspace id and key, add them to the provider
	// Environment variables overwrite the values provided in the file
	if logAnalyticsID := os.Getenv("LOG_ANALYTICS_ID"); logAnalyticsID != "" {
		if logAnalyticsKey := os.Getenv("LOG_ANALYTICS_KEY"); logAnalyticsKey != "" {
			p.diagnostics, err = analytics.NewContainerGroupDiagnostics(logAnalyticsID, logAnalyticsKey)
			if err != nil {
				return nil, err
			}
		}
	}

	if clusterResourceID := os.Getenv("CLUSTER_RESOURCE_ID"); clusterResourceID != "" {
		if p.diagnostics != nil && p.diagnostics.LogAnalytics != nil {
			p.diagnostics.LogAnalytics.LogType = &util.LogTypeContainerInsights
			p.diagnostics.LogAnalytics.Metadata = map[string]*string{
				analytics.LogAnalyticsMetadataKeyClusterResourceID: &clusterResourceID,
				analytics.LogAnalyticsMetadataKeyNodeName:          &nodeName,
			}
		}
	}

	if rg := os.Getenv("ACI_RESOURCE_GROUP"); rg != "" {
		p.resourceGroup = rg
	} else if p.resourceGroup == "" {
		return nil, errors.New("resource group can not be empty please set ACI_RESOURCE_GROUP")
	}

	if r := os.Getenv("ACI_REGION"); r != "" {
		p.region = r
	} else if p.region == "" {
		return nil, errors.New("region can not be empty please set ACI_REGION")
	}

	if r := p.region; !isValidACIRegion(r) {
		unsupportedRegionMessage := fmt.Sprintf("Region %s is invalid. Current supported regions are: %s",
			r, strings.Join(validAciRegions, ", "))
		return nil, errors.New(unsupportedRegionMessage)
	}

	if err := p.setupNodeCapacity(ctx); err != nil {
		return nil, err
	}

	if err := p.providerNetwork.SetVNETConfig(ctx, &azConfig); err != nil {
		return nil, err
	}

	if p.providerNetwork.SubnetName != "" {
		// windows containers don't support kube-proxy nor realtime metrics
		if p.operatingSystem != string(azaciv2.OperatingSystemTypesWindows) {
			err = p.setACIExtensions(ctx)
			if err != nil {
				return nil, err
			}
		}
	}

	p.ACIPodMetricsProvider = metrics.NewACIPodMetricsProvider(p.nodeName, p.resourceGroup, p.podsL, p.azClientsAPIs)
	return &p, err
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
	var err error
	ctx, span := trace.StartSpan(ctx, "aci.CreatePod")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg := &azaciv2.ContainerGroup{
		Properties: &azaciv2.ContainerGroupPropertiesProperties{},
	}

	os := azaciv2.OperatingSystemTypes(p.operatingSystem)
	policy := azaciv2.ContainerGroupRestartPolicy(pod.Spec.RestartPolicy)

	cg.Location = &p.region
	cg.Properties.RestartPolicy = &policy
	cg.Properties.OSType = &os

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
	volumes, err := p.getVolumes(ctx, pod)
	if err != nil {
		return err

	}

	if p.enabledFeatures.IsEnabled(ctx, featureflag.InitContainerFeature) {
		// get initContainers
		initContainers, err := p.getInitContainers(ctx, pod)
		if err != nil {
			return err
		}
		cg.Properties.InitContainers = initContainers
	}

	// confidential compute proeprties
	if p.enabledFeatures.IsEnabled(ctx, featureflag.ConfidentialComputeFeature) {
		// set confidentialComputeProperties
		p.setConfidentialComputeProperties(ctx, pod, cg)
	}

	// assign all the things
	cg.Properties.Containers = containers
	cg.Properties.Volumes = volumes
	cg.Properties.ImageRegistryCredentials = creds
	cg.Properties.Diagnostics = p.getDiagnostics(pod)

	filterWindowsServiceAccountSecretVolume(ctx, p.operatingSystem, cg)

	// create ipaddress if containerPort is used
	count := 0
	for _, container := range containers {
		count = count + len(container.Properties.Ports)
	}
	ports := make([]*azaciv2.Port, 0, count)
	for c := range containers {
		containerPorts := containers[c].Properties.Ports
		for p := range containerPorts {
			ports = append(ports, &azaciv2.Port{
				Port:     containerPorts[p].Port,
				Protocol: &util.ContainerGroupNetworkProtocolTCP,
			})
		}
	}
	if len(ports) > 0 && p.providerNetwork.SubnetName == "" {
		cg.Properties.IPAddress = &azaciv2.IPAddress{
			Ports: ports,
			Type:  &util.ContainerGroupIPAddressTypePublic,
		}

		if dnsNameLabel := pod.Annotations[virtualKubeletDNSNameLabel]; dnsNameLabel != "" {
			cg.Properties.IPAddress.DNSNameLabel = &dnsNameLabel
		}
	}

	podUID := string(pod.UID)
	podCreationTimestamp := pod.CreationTimestamp.String()
	cg.Tags = map[string]*string{
		"PodName":           &pod.Name,
		"NodeName":          &pod.Spec.NodeName,
		"Namespace":         &pod.Namespace,
		"UID":               &podUID,
		"CreationTimestamp": &podCreationTimestamp,
	}

	p.providerNetwork.AmendVnetResources(ctx, *cg, pod, p.clusterDomain)

	// windows containers don't support kube-proxy nor realtime metrics
	if cg.Properties.OSType != nil &&
		*cg.Properties.OSType != azaciv2.OperatingSystemTypesWindows {
		cg.Properties.Extensions = p.containerGroupExtensions
	}

	log.G(ctx).Debugf("start creating pod %v", pod.Name)
	// TODO: Run in a go routine to not block workers, and use tracker.UpdatePodStatus() based on result.
	return p.azClientsAPIs.CreateContainerGroup(ctx, p.resourceGroup, pod.Namespace, pod.Name, cg)
}

// setACIExtensions
func (p *ACIProvider) setACIExtensions(ctx context.Context) error {
	masterURI := os.Getenv("MASTER_URI")
	if masterURI == "" {
		masterURI = "10.0.0.1"
	}
	clusterCIDR := os.Getenv("CLUSTER_CIDR")
	if clusterCIDR == "" {
		clusterCIDR = "10.240.0.0/16"
	}

	kubeExtensions, err := client.GetKubeProxyExtension(serviceAccountSecretMountPath, masterURI, clusterCIDR)
	if err != nil {
		return fmt.Errorf("error creating kube proxy extension: %v", err)
	}

	p.containerGroupExtensions = append(p.containerGroupExtensions, kubeExtensions)

	enableRealTimeMetricsExtension := os.Getenv("ENABLE_REAL_TIME_METRICS")
	if enableRealTimeMetricsExtension == "true" {
		realtimeExtension := client.GetRealtimeMetricsExtension()
		p.containerGroupExtensions = append(p.containerGroupExtensions, realtimeExtension)
	}
	return nil
}

func (p *ACIProvider) getDiagnostics(pod *v1.Pod) *azaciv2.ContainerGroupDiagnostics {
	if p.diagnostics != nil && p.diagnostics.LogAnalytics != nil &&
		p.diagnostics.LogAnalytics.LogType != nil &&
		*p.diagnostics.LogAnalytics.LogType == azaciv2.LogAnalyticsLogTypeContainerInsights {
		d := *p.diagnostics
		uID := string(pod.ObjectMeta.UID)
		d.LogAnalytics.Metadata[analytics.LogAnalyticsMetadataKeyPodUUID] = &uID
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

	log.G(ctx).Debugf("start deleting pod %v", pod.Name)
	// TODO: Run in a go routine to not block workers.
	return p.deleteContainerGroup(ctx, pod.Namespace, pod.Name)
}

func (p *ACIProvider) deleteContainerGroup(ctx context.Context, podNS, podName string) error {
	ctx, span := trace.StartSpan(ctx, "aci.deleteContainerGroup")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cgName := containerGroupName(podNS, podName)

	err := p.azClientsAPIs.DeleteContainerGroup(ctx, p.resourceGroup, cgName)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("failed to delete container group %v", cgName)
		return err
	}

	if p.tracker != nil {
		// Delete is not a sync API on ACI yet, but will assume with current implementation that termination is completed. Also, till gracePeriod is supported.
		updateErr := p.tracker.UpdatePodStatus(ctx,
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

	cg, err := p.azClientsAPIs.GetContainerGroupInfo(ctx, p.resourceGroup, namespace, name, p.nodeName)
	if err != nil {
		return nil, err
	}

	err = validation.ValidateContainerGroup(ctx, cg)
	if err != nil {
		return nil, err
	}

	return p.containerGroupToPod(ctx, cg)
}

// GetContainerLogs returns the logs of a pod by name that is running inside ACI.
func (p *ACIProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetContainerLogs")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg, err := p.azClientsAPIs.GetContainerGroupInfo(ctx, p.resourceGroup, namespace, podName, p.nodeName)
	if err != nil {
		return nil, err
	}

	// get logs from cg
	logContent, err := p.azClientsAPIs.ListLogs(ctx, p.resourceGroup, *cg.Name, containerName, opts)
	if err != nil {
		return nil, err
	}
	if logContent != nil {
		logStr := *logContent
		return io.NopCloser(strings.NewReader(logStr)), nil
	}
	return nil, nil
}

// GetPodFullName as defined in the provider context
func (p *ACIProvider) GetPodFullName(namespace string, pod string) string {
	return fmt.Sprintf("%s-%s", namespace, pod)
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *ACIProvider) RunInContainer(ctx context.Context, namespace, name, container string, cmd []string, attach api.AttachIO) error {
	logger := log.G(ctx).WithField("method", "RunInContainer")
	ctx, span := trace.StartSpan(ctx, "aci.RunInContainer")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	out := attach.Stdout()
	if out != nil {
		defer out.Close()
	}

	cg, err := p.azClientsAPIs.GetContainerGroupInfo(ctx, p.resourceGroup, namespace, name, p.nodeName)
	if err != nil {
		return err
	}

	termSize := api.TermSize{
		Width:  60,
		Height: 120,
	}
	if attach.TTY() {
		resize := attach.Resize()
		select {
		case termSize = <-resize:
			break
		case <-time.After(5 * time.Second):
			break
		}
	}
	// Set default terminal size
	cols := int32(termSize.Width)
	rows := int32(termSize.Height)
	cmdParam := strings.Join(cmd, " ")
	req := azaciv2.ContainerExecRequest{
		Command: &cmdParam,
		TerminalSize: &azaciv2.ContainerExecRequestTerminalSize{
			Cols: &cols,
			Rows: &rows,
		},
	}

	xcrsp, err := p.azClientsAPIs.ExecuteContainerCommand(ctx, p.resourceGroup, *cg.Name, container, req)
	if err != nil {
		return err
	}

	wsURI := *xcrsp.WebSocketURI
	password := *xcrsp.Password

	c, _, err := websocket.DefaultDialer.Dial(wsURI, nil)
	if err != nil {
		return err
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte(password)); err != nil { // Websocket password needs to be sent before WS terminal is active
		return err
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
					if err = c.WriteMessage(websocket.BinaryMessage, msg[:n]); err != nil {
						logger.Errorf("an error has occurred while trying to write message")
						return
					}
				}
			}
		}()
	}
	if err != nil {
		return err
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
				logger.Errorf("an error has occurred while trying to copy message")
				break
			}
		}
	}
	if err != nil {
		return err
	}

	return ctx.Err()
}

// AttachToContainer Implementation placeholder
// TODO: complete the implementation for Attach functionality
func (p *ACIProvider) AttachToContainer(ctx context.Context, namespace string, podName string, containerName string, attach api.AttachIO) error {
	return nil
}

// GetPodStatus returns the status of a pod by name that is running inside ACI
// returns nil if a pod by that name is not found.
func (p *ACIProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetPodStatus")
	defer span.End()
	ctx = addAzureAttributes(ctx, span, p)

	cg, err := p.azClientsAPIs.GetContainerGroupInfo(ctx, p.resourceGroup, namespace, name, p.nodeName)
	if err != nil {
		return nil, err
	}

	err = validation.ValidateContainerGroup(ctx, cg)
	if err != nil {
		return nil, err
	}

	return p.getPodStatusFromContainerGroup(ctx, cg)
}

// GetPods returns a list of all pods known to be running within ACI.
func (p *ACIProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	ctx, span := trace.StartSpan(ctx, "aci.GetPods")
	defer span.End()

	ctx = addAzureAttributes(ctx, span, p)

	cgs, err := p.azClientsAPIs.GetContainerGroupListResult(ctx, p.resourceGroup)
	if err != nil {
		return nil, err
	}
	if cgs == nil {
		log.G(ctx).Infof("no container groups found for resource group %s", p.resourceGroup)
		return nil, nil
	}
	pods := make([]*v1.Pod, 0, len(cgs))

	for cgIndex := range cgs {
		cgName := cgs[cgIndex].Name
		if cgName == nil {
			continue
		}
		// The GetContainerGroupListResult API doesn't return InstanceView status which can cause nil.
		// For that, we had to get the CG info one more time.
		cg, err := p.azClientsAPIs.GetContainerGroup(ctx, p.resourceGroup, *cgName)
		// CG might get deleted between the getlist and get calls
		if errdefs.IsNotFound(err) || cg == nil {
			continue
		}
		if err != nil {
			log.G(ctx).WithFields(log.Fields{
				"name": *cgName,
				"id":   *cg.ID,
			}).WithError(err).Errorf("error getting container group %s", *cgName)
			continue
		}

		err2 := validation.ValidateContainerGroup(ctx, cg)
		if err2 != nil {
			log.G(ctx).WithFields(log.Fields{
				"name": *cgName,
				"id":   *cg.ID,
			}).WithError(err2).Errorf("error validating container group %s", *cgName)
			continue
		}

		if cg.Tags != nil && cg.Tags["NodeName"] != nil {
			if *cg.Tags["NodeName"] != p.nodeName {
				log.G(ctx).WithFields(log.Fields{
					"name": *cgName,
					"id":   *cg.ID,
				}).Warnf("container group %s node name does not match %s", *cgName, p.nodeName)
				continue
			}
		} else {
			log.G(ctx).WithFields(log.Fields{
				"name": *cgName,
				"id":   *cg.ID,
			}).Warnf("container group %s node name should not be nil", *cgName)
			continue
		}

		pod, err3 := p.containerGroupToPod(ctx, cg)
		if err3 != nil {
			log.G(ctx).WithFields(log.Fields{
				"name": *cgName,
				"id":   *cg.ID,
			}).WithError(err3).Errorf("error converting container group %s to pod", *cgName)
			continue
		}

		if pod != nil {
			pods = append(pods, pod)
		}
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
		pods:           p.podsL,
		updateCb:       notifierCb,
		handler:        p,
		lastEventCheck: time.UnixMicro(0),
		eventRecorder:  p.eventRecorder,
	}

	go p.tracker.StartTracking(ctx)
}

// ListActivePods interface impl.
func (p *ACIProvider) ListActivePods(ctx context.Context) ([]PodIdentifier, error) {
	ctx, span := trace.StartSpan(ctx, "ACIProvider.ListActivePods")
	defer span.End()

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

// FetchPodStatus interface impl
func (p *ACIProvider) FetchPodStatus(ctx context.Context, ns, name string) (*v1.PodStatus, error) {
	ctx, span := trace.StartSpan(ctx, "ACIProvider.FetchPodStatus")
	defer span.End()

	return p.GetPodStatus(ctx, ns, name)
}

func (p *ACIProvider) FetchPodEvents(ctx context.Context, pod *v1.Pod, evtSink func(timestamp *time.Time, object runtime.Object, eventtype, reason, messageFmt string, args ...interface{})) error {
	ctx, span := trace.StartSpan(ctx, "ACIProvider.FetchPodEvents")
	defer span.End()
	if !p.enabledFeatures.IsEnabled(ctx, featureflag.Events) {
		return nil
	}

	ctx = addAzureAttributes(ctx, span, p)
	cgName := containerGroupName(pod.Namespace, pod.Name)
	cg, err := p.azClientsAPIs.GetContainerGroup(ctx, p.resourceGroup, cgName)
	if err != nil {
		return err
	}
	if *cg.Tags["NodeName"] != p.nodeName {
		return errors.Wrapf(err, "container group %s found with mismatching node", cgName)
	}

	if cg.Properties != nil && cg.Properties.InstanceView != nil && cg.Properties.InstanceView.Events != nil {
		for _, evt := range cg.Properties.InstanceView.Events {
			evtSink(evt.LastTimestamp, pod, *evt.Type, *evt.Name, *evt.Message)
		}
	}

	if cg.Properties != nil && cg.Properties.Containers != nil {
		for _, container := range cg.Properties.Containers {
			if container.Properties != nil && container.Properties.InstanceView != nil && container.Properties.InstanceView.Events != nil {
				for _, evt := range container.Properties.InstanceView.Events {
					podReference, err := reference.GetReference(scheme.Scheme, pod)
					if err != nil {
						log.G(ctx).WithError(err).Warnf("cannot get k8s object reference from pod %s in namespace %s", pod.Name, pod.Namespace)
					}
					podReference.FieldPath = fmt.Sprintf("spec.containers{%s}", *container.Name)
					evtSink(evt.LastTimestamp, podReference, *evt.Type, *evt.Name, *evt.Message)
				}
			}
		}
	}
	return nil
}

// CleanupPod interface impl
func (p *ACIProvider) CleanupPod(ctx context.Context, ns, name string) error {
	ctx, span := trace.StartSpan(ctx, "ACIProvider.CleanupPod")
	defer span.End()

	return p.deleteContainerGroup(ctx, ns, name)
}

// PortForward
func (p *ACIProvider) PortForward(ctx context.Context, namespace, pod string, port int32, stream io.ReadWriteCloser) error {
	log.G(ctx).Info("Port Forward is not supported in AZure ACI")
	return nil
}

func (p *ACIProvider) getImagePullSecrets(pod *v1.Pod) ([]*azaciv2.ImageRegistryCredential, error) {
	ips := make([]*azaciv2.ImageRegistryCredential, 0, len(pod.Spec.ImagePullSecrets))
	for _, ref := range pod.Spec.ImagePullSecrets {
		secret, err := p.secretL.Secrets(pod.Namespace).Get(ref.Name)
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

func makeRegistryCredential(server string, authConfig AuthConfig) (*azaciv2.ImageRegistryCredential, error) {
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

	cred := azaciv2.ImageRegistryCredential{
		Server:   &server,
		Username: &username,
		Password: &password,
	}

	return &cred, nil
}

func makeRegistryCredentialFromDockerConfig(server string, configEntry dockercfg.AuthConfig) (*azaciv2.ImageRegistryCredential, error) {
	if configEntry.Username == "" {
		return nil, fmt.Errorf("no username present in auth config for server: %s", server)
	}

	username := configEntry.Username
	password := configEntry.Password
	if configEntry.Auth != "" {
		var err error
		username, password, err = dockercfg.DecodeBase64Auth(configEntry)
		if err != nil {
			return nil, fmt.Errorf("error decoding docker auth: %w", err)
		}
	}

	cred := azaciv2.ImageRegistryCredential{
		Server:   &server,
		Username: &username,
		Password: &password,
	}

	return &cred, nil
}

func readDockerCfgSecret(secret *v1.Secret, ips []*azaciv2.ImageRegistryCredential) ([]*azaciv2.ImageRegistryCredential, error) {
	var err error
	var authConfigs map[string]AuthConfig
	repoData, ok := secret.Data[v1.DockerConfigKey]

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

		ips = append(ips, cred)
	}

	return ips, err
}

func readDockerConfigJSONSecret(secret *v1.Secret, ips []*azaciv2.ImageRegistryCredential) ([]*azaciv2.ImageRegistryCredential, error) {
	var err error
	repoData, ok := secret.Data[v1.DockerConfigJsonKey]

	if !ok {
		return ips, fmt.Errorf("no dockerconfigjson present in secret")
	}

	// Will use K8s config models to handle marshaling (including auth field handling).
	var cfgJson dockercfg.Config

	err = json.Unmarshal(repoData, &cfgJson)
	if err != nil {
		return ips, err
	}

	auths := cfgJson.AuthConfigs
	if len(auths) == 0 {
		return ips, fmt.Errorf("malformed dockerconfigjson in secret")
	}

	for server := range auths {
		cred, err := makeRegistryCredentialFromDockerConfig(server, auths[server])
		if err != nil {
			return ips, err
		}

		ips = append(ips, cred)
	}

	return ips, err
}

// verify if Container is properly declared for the use on ACI
func (p *ACIProvider) verifyContainer(container *v1.Container) error {
	if len(container.Command) == 0 && len(container.Args) > 0 {
		return errdefs.InvalidInput("ACI does not support providing args without specifying the command. Please supply both command and args to the pod spec.")
	}
	return nil
}

// this method is used for both initConainers and containers
func (p *ACIProvider) getCommand(container v1.Container) []*string {
	command := make([]*string, 0)
	for c := range container.Command {
		command = append(command, &container.Command[c])
	}

	args := make([]*string, 0)
	for a := range container.Args {
		args = append(args, &container.Args[a])
	}

	return append(command, args...)
}

// get VolumeMounts declared on Container as []aci.VolumeMount
func (p *ACIProvider) getVolumeMounts(container v1.Container) []*azaciv2.VolumeMount {
	volumeMounts := make([]*azaciv2.VolumeMount, 0, len(container.VolumeMounts))
	for i := range container.VolumeMounts {
		volumeMounts = append(volumeMounts, &azaciv2.VolumeMount{
			Name:      &container.VolumeMounts[i].Name,
			MountPath: &container.VolumeMounts[i].MountPath,
			ReadOnly:  &container.VolumeMounts[i].ReadOnly,
		})
	}
	return volumeMounts
}

// get EnvironmentVariables declared on Container as []aci.EnvironmentVariable
func (p *ACIProvider) getEnvironmentVariables(container v1.Container) []*azaciv2.EnvironmentVariable {
	environmentVariable := make([]*azaciv2.EnvironmentVariable, 0, len(container.Env))
	for i := range container.Env {
		if container.Env[i].Value != "" {
			envVar := getACIEnvVar(container.Env[i])
			environmentVariable = append(environmentVariable, envVar)
		}
	}
	return environmentVariable
}

// get InitContainers defined in Pod as []aci.InitContainerDefinition
func (p *ACIProvider) getInitContainers(ctx context.Context, pod *v1.Pod) ([]*azaciv2.InitContainerDefinition, error) {
	initContainers := make([]*azaciv2.InitContainerDefinition, 0, len(pod.Spec.InitContainers))
	for i, initContainer := range pod.Spec.InitContainers {
		err := p.verifyContainer(&initContainer)
		if err != nil {
			log.G(ctx).Errorf("couldn't verify container %v", err)
			return nil, err
		}

		if initContainer.Ports != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support ports")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support ports")
		}
		if initContainer.Resources.Requests != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support resources requests")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support resources requests")
		}
		if initContainer.Resources.Limits != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support resources limits")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support resources limits")
		}
		if initContainer.LivenessProbe != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support livenessProbe")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support livenessProbe")
		}
		if initContainer.ReadinessProbe != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support readinessProbe")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support readinessProbe")
		}
		if hasLifecycleHook(initContainer) {
			log.G(ctx).Errorf("azure container instances initcontainers do not support lifecycle hooks")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support lifecycle hooks")
		}
		if initContainer.StartupProbe != nil {
			log.G(ctx).Errorf("azure container instances initcontainers do not support startupProbe")
			return nil, errdefs.InvalidInput("azure container instances initContainers do not support startupProbe")
		}

		newInitContainer := azaciv2.InitContainerDefinition{
			Name: &pod.Spec.InitContainers[i].Name,
			Properties: &azaciv2.InitContainerPropertiesDefinition{
				Image:                &pod.Spec.InitContainers[i].Image,
				Command:              p.getCommand(pod.Spec.InitContainers[i]),
				VolumeMounts:         p.getVolumeMounts(pod.Spec.InitContainers[i]),
				EnvironmentVariables: p.getEnvironmentVariables(pod.Spec.InitContainers[i]),
			},
		}

		initContainers = append(initContainers, &newInitContainer)
	}
	return initContainers, nil
}

func (p *ACIProvider) getContainers(pod *v1.Pod) ([]*azaciv2.Container, error) {
	containers := make([]*azaciv2.Container, 0, len(pod.Spec.Containers))

	podContainers := pod.Spec.Containers
	for c := range podContainers {

		if len(podContainers[c].Command) == 0 && len(podContainers[c].Args) > 0 {
			return nil, errdefs.InvalidInput("ACI does not support providing args without specifying the command. Please supply both command and args to the pod spec.")
		}
		if hasLifecycleHook(podContainers[c]) {
			return nil, errdefs.InvalidInput("ACI does not support lifecycle hooks")
		}
		if podContainers[c].StartupProbe != nil {
			return nil, errdefs.InvalidInput("ACI does not support startupProbe")
		}
		cmd := p.getCommand(podContainers[c])
		ports := make([]*azaciv2.ContainerPort, 0, len(podContainers[c].Ports))
		aciContainer := azaciv2.Container{
			Name: &podContainers[c].Name,
			Properties: &azaciv2.ContainerProperties{
				Image:   &podContainers[c].Image,
				Command: cmd,
				Ports:   ports,
			},
		}

		for i := range podContainers[c].Ports {
			aciContainer.Properties.Ports = append(aciContainer.Properties.Ports, &azaciv2.ContainerPort{
				Port:     &podContainers[c].Ports[i].ContainerPort,
				Protocol: util.GetProtocol(podContainers[c].Ports[i].Protocol),
			})
		}

		volMount := make([]*azaciv2.VolumeMount, 0, len(podContainers[c].VolumeMounts))
		aciContainer.Properties.VolumeMounts = volMount
		for v := range podContainers[c].VolumeMounts {
			aciContainer.Properties.VolumeMounts = append(aciContainer.Properties.VolumeMounts, &azaciv2.VolumeMount{
				Name:      &podContainers[c].VolumeMounts[v].Name,
				MountPath: &podContainers[c].VolumeMounts[v].MountPath,
				ReadOnly:  &podContainers[c].VolumeMounts[v].ReadOnly,
			})
		}

		initEnv := make([]*azaciv2.EnvironmentVariable, 0, len(podContainers[c].Env))
		aciContainer.Properties.EnvironmentVariables = initEnv
		for _, e := range podContainers[c].Env {
			if e.Value != "" {
				envVar := getACIEnvVar(e)
				envList := append(aciContainer.Properties.EnvironmentVariables, envVar)
				aciContainer.Properties.EnvironmentVariables = envList
			}
		}

		// NOTE(robbiezhang): ACI CPU request must be times of 10m
		cpuRequest := 1.00
		if _, ok := podContainers[c].Resources.Requests[v1.ResourceCPU]; ok {
			cpuRequest = float64(podContainers[c].Resources.Requests.Cpu().MilliValue()/10.00) / 100.00
			if cpuRequest < 0.01 {
				cpuRequest = 0.01
			}
		}

		// NOTE(robbiezhang): ACI memory request must be times of 0.1 GB
		memoryRequest := 1.50
		if _, ok := podContainers[c].Resources.Requests[v1.ResourceMemory]; ok {
			memoryRequest = float64(podContainers[c].Resources.Requests.Memory().Value()/100000000.00) / 10.00
			if memoryRequest < 0.10 {
				memoryRequest = 0.10
			}
		}

		aciContainer.Properties.Resources = &azaciv2.ResourceRequirements{
			Requests: &azaciv2.ResourceRequests{
				CPU:        &cpuRequest,
				MemoryInGB: &memoryRequest,
			},
		}

		if podContainers[c].Resources.Limits != nil {
			cpuLimit := cpuRequest
			if _, ok := podContainers[c].Resources.Limits[v1.ResourceCPU]; ok {
				cpuLimit = float64(podContainers[c].Resources.Limits.Cpu().MilliValue()) / 1000.00
			}

			// NOTE(jahstreet): ACI memory limit must be times of 0.1 GB
			memoryLimit := memoryRequest
			if _, ok := podContainers[c].Resources.Limits[v1.ResourceMemory]; ok {
				memoryLimit = float64(podContainers[c].Resources.Limits.Memory().Value()/100000000.00) / 10.00
			}
			aciContainer.Properties.Resources.Limits = &azaciv2.ResourceLimits{
				CPU:        &cpuLimit,
				MemoryInGB: &memoryLimit,
			}

			if gpu, ok := podContainers[c].Resources.Limits[gpuResourceName]; ok {
				sku, err := p.getGPUSKU(pod)
				if err != nil {
					return nil, err
				}

				if gpu.Value() == 0 {
					return nil, errors.New("GPU must be a integer number")
				}

				count := int32(gpu.Value())

				gpuResource := &azaciv2.GpuResource{
					Count: &count,
					SKU:   &sku,
				}

				aciContainer.Properties.Resources.Requests.Gpu = gpuResource
				aciContainer.Properties.Resources.Limits.Gpu = gpuResource
			}
		}

		if podContainers[c].LivenessProbe != nil {
			probe, err := getProbe(podContainers[c].LivenessProbe, podContainers[c].Ports)
			if err != nil {
				return nil, err
			}
			aciContainer.Properties.LivenessProbe = probe
		}

		if podContainers[c].ReadinessProbe != nil {
			probe, err := getProbe(podContainers[c].ReadinessProbe, podContainers[c].Ports)
			if err != nil {
				return nil, err
			}
			aciContainer.Properties.ReadinessProbe = probe
		}

		containers = append(containers, &aciContainer)
	}
	return containers, nil
}

func (p *ACIProvider) setConfidentialComputeProperties(ctx context.Context, pod *v1.Pod, cg *azaciv2.ContainerGroup) {
	containerGroupSku := pod.Annotations[confidentialComputeSkuLabel]
	ccePolicy := pod.Annotations[confidentialComputeCcePolicyLabel]
	confidentialSku := azaciv2.ContainerGroupSKUConfidential

	l := log.G(ctx).WithField("containerGroup", cg.Name)

	if ccePolicy != "" {
		cg.Properties.SKU = &confidentialSku
		confidentialComputeProperties := azaciv2.ConfidentialComputeProperties{
			CcePolicy: &ccePolicy,
		}
		cg.Properties.ConfidentialComputeProperties = &confidentialComputeProperties
		l.Infof("setting confidential compute properties with CCE Policy")

	} else if strings.ToLower(containerGroupSku) == "confidential" {
		cg.Properties.SKU = &confidentialSku
		l.Infof("setting confidential container group SKU")
	}

	l.Infof("no annotations for confidential SKU")
}

func (p *ACIProvider) getGPUSKU(pod *v1.Pod) (azaciv2.GpuSKU, error) {
	if len(p.gpuSKUs) == 0 {
		return "", fmt.Errorf("the pod requires GPU resource, but ACI doesn't provide GPU enabled container group in region %s", p.region)
	}

	if desiredSKU, ok := pod.Annotations[gpuTypeAnnotation]; ok {
		for _, supportedSKU := range p.gpuSKUs {
			if strings.EqualFold(desiredSKU, string(supportedSKU)) {
				return supportedSKU, nil
			}
		}

		return "", fmt.Errorf("the pod requires GPU SKU %s, but ACI only supports SKUs %v in region %s", desiredSKU, p.gpuSKUs, p.region)
	}

	return p.gpuSKUs[0], nil
}

func getProbe(probe *v1.Probe, ports []v1.ContainerPort) (*azaciv2.ContainerProbe, error) {

	if probe.ProbeHandler.Exec != nil && probe.ProbeHandler.HTTPGet != nil {
		return nil, fmt.Errorf("probe may not specify more than one of \"exec\" and \"httpGet\"")
	}

	if probe.ProbeHandler.Exec == nil && probe.ProbeHandler.HTTPGet == nil {
		return nil, fmt.Errorf("probe must specify one of \"exec\" and \"httpGet\"")
	}

	// Probes have can have an Exec or HTTP Get ProbeHandler.
	// Create those if they exist, then add to the
	// ContainerProbe struct
	var exec *azaciv2.ContainerExec
	commands := make([]*string, 0)
	if probe.ProbeHandler.Exec != nil {
		for i := range probe.ProbeHandler.Exec.Command {
			commands = append(commands, &probe.ProbeHandler.Exec.Command[i])
		}
		exec = &azaciv2.ContainerExec{
			Command: commands,
		}
	}
	var httpGET *azaciv2.ContainerHTTPGet
	if probe.ProbeHandler.HTTPGet != nil {
		var portValue int32
		port := probe.ProbeHandler.HTTPGet.Port
		switch port.Type {
		case intstr.Int:
			portValue = int32(port.IntValue())
		case intstr.String:
			portName := port.String()
			for _, p := range ports {
				if portName == p.Name {
					portValue = p.ContainerPort
					break
				}
			}
			if portValue == 0 {
				return nil, fmt.Errorf("unable to find named port: %s", portName)
			}
		}

		scheme := azaciv2.Scheme(probe.ProbeHandler.HTTPGet.Scheme)
		httpGET = &azaciv2.ContainerHTTPGet{
			Port:   &portValue,
			Path:   &probe.ProbeHandler.HTTPGet.Path,
			Scheme: &scheme,
		}
	}

	return &azaciv2.ContainerProbe{
		Exec:                exec,
		HTTPGet:             httpGET,
		InitialDelaySeconds: &probe.InitialDelaySeconds,
		FailureThreshold:    &probe.FailureThreshold,
		SuccessThreshold:    &probe.SuccessThreshold,
		TimeoutSeconds:      &probe.TimeoutSeconds,
		PeriodSeconds:       &probe.PeriodSeconds,
	}, nil
}

// Filters service account secret volume for Windows.
// Service account secret volume gets automatically turned on if not specified otherwise.
// ACI doesn't support secret volume for Windows, so we need to filter it.
func filterWindowsServiceAccountSecretVolume(ctx context.Context, osType string, cgw *azaciv2.ContainerGroup) {
	if strings.EqualFold(osType, "Windows") {
		serviceAccountSecretVolumeName := make(map[string]bool)

		for index, container := range cgw.Properties.Containers {
			volumeMounts := make([]*azaciv2.VolumeMount, 0, len(container.Properties.VolumeMounts))
			for _, volumeMount := range container.Properties.VolumeMounts {
				if !strings.EqualFold(serviceAccountSecretMountPath, *volumeMount.MountPath) {
					volumeMounts = append(volumeMounts, volumeMount)
				} else {
					serviceAccountSecretVolumeName[*volumeMount.Name] = true
				}
			}
			cgw.Properties.Containers[index].Properties.VolumeMounts = volumeMounts
		}

		if len(serviceAccountSecretVolumeName) == 0 {
			return
		}

		l := log.G(ctx).WithField("containerGroup", cgw.Name)
		l.Infof("Ignoring service account secret volumes '%v' for Windows", reflect.ValueOf(serviceAccountSecretVolumeName).MapKeys())

		volumes := make([]*azaciv2.Volume, 0, len(cgw.Properties.Volumes))
		for _, volume := range cgw.Properties.Volumes {
			if _, ok := serviceAccountSecretVolumeName[*volume.Name]; !ok {
				volumes = append(volumes, volume)
			}
		}

		cgw.Properties.Volumes = volumes
	}
}

func getACIEnvVar(e v1.EnvVar) *azaciv2.EnvironmentVariable {
	var envVar azaciv2.EnvironmentVariable
	// If the variable is a secret, use SecureValue
	if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
		envVar = azaciv2.EnvironmentVariable{
			Name:        &e.Name,
			SecureValue: &e.Value,
		}
	} else {
		envVar = azaciv2.EnvironmentVariable{
			Name:  &e.Name,
			Value: &e.Value,
		}
	}
	return &envVar
}

func hasLifecycleHook(c v1.Container) bool {
	hasHandler := func(l *v1.LifecycleHandler) bool {
		return l != nil && (l.HTTPGet != nil || l.Exec != nil || l.TCPSocket != nil)
	}
	return c.Lifecycle != nil && (hasHandler(c.Lifecycle.PreStop) || hasHandler(c.Lifecycle.PostStart))
}
