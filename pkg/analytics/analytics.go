package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	azaci "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
)

const (
	LogAnalyticsMetadataKeyPodUUID           string = "pod-uuid"
	LogAnalyticsMetadataKeyNodeName          string = "node-name"
	LogAnalyticsMetadataKeyClusterResourceID string = "cluster-resource-id"
)

// NewContainerGroupDiagnostics creates a container group diagnostics object
func NewContainerGroupDiagnostics(logAnalyticsID, logAnalyticsKey string) (*azaci.ContainerGroupDiagnostics, error) {
	if logAnalyticsID == "" || logAnalyticsKey == "" {
		return nil, errors.New("log Analytics configuration requires both the workspace ID and Key")
	}

	return &azaci.ContainerGroupDiagnostics{
		LogAnalytics: &azaci.LogAnalytics{
			WorkspaceID:  &logAnalyticsID,
			WorkspaceKey: &logAnalyticsKey,
		},
	}, nil
}

// NewContainerGroupDiagnosticsFromFile creates a container group diagnostics object from the specified file
func NewContainerGroupDiagnosticsFromFile(filepath string) (*azaci.ContainerGroupDiagnostics, error) {
	analyticsDataFile, err := os.Open(filepath)
	defer analyticsDataFile.Close()
	if err != nil {
		return nil, fmt.Errorf("reading Log Analytics Auth file %q failed: %v", filepath, err)
	}

	fileStat, err := analyticsDataFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading Log Analytics Auth file %q failed: %v", filepath, err)
	}
	if fileStat.Size() == 0 {
		return nil, fmt.Errorf("log analytics auth file %q cannot be empty", filepath)
	}

	analyticsData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("reading Log Analytics Auth file %q failed: %v", filepath, err)
	}

	// Unmarshal the log analytics file.
	var logAnalytics azaci.LogAnalytics
	if err := json.Unmarshal(analyticsData, &logAnalytics); err != nil {
		return nil, err
	}

	return &azaci.ContainerGroupDiagnostics{
		LogAnalytics: &logAnalytics,
	}, err
}
