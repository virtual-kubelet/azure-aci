package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	azaci "github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2021-10-01/containerinstance"
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
		return nil, errors.New(fmt.Sprintf("reading Log Analytics Auth file %q failed: %v", filepath, err))
	}

	fileStat, err := analyticsDataFile.Stat()
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
