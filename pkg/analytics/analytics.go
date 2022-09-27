package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

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

	analyticsdata, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("Reading Log Analytics Auth file %q failed: %v", filepath, err)
	}
	// Unmarshal the log analytics file.
	var law azaci.LogAnalytics
	if err := json.Unmarshal(analyticsdata, &law); err != nil {
		return nil, err
	}

	return &azaci.ContainerGroupDiagnostics{
		LogAnalytics: &law,
	}, nil
}
