/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package analytics

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestNewContainerGroupDiagnostics(t *testing.T) {
	cases := []struct {
		description     string
		logAnalyticsID  string
		logAnalyticsKey string
		expectedError   error
	}{
		{
			description:     "Empty values",
			logAnalyticsID:  "",
			logAnalyticsKey: "",
			expectedError:   errors.New("log Analytics configuration requires both the workspace ID and Key"),
		},
		{
			description:     "Valid values",
			logAnalyticsID:  "####-####-####-####",
			logAnalyticsKey: "loganalyticskey&%#$",
			expectedError:   nil,
		}}
	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {

			cgDiagnostics, err := NewContainerGroupDiagnostics(tc.logAnalyticsID, tc.logAnalyticsKey)
			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.Equal(t, tc.logAnalyticsID, *cgDiagnostics.LogAnalytics.WorkspaceID)
				assert.Equal(t, tc.logAnalyticsKey, *cgDiagnostics.LogAnalytics.WorkspaceKey)
				assert.NilError(t, tc.expectedError, err)
			}
		})
	}
}
func TestLogAnalyticsFileParsingSuccess(t *testing.T) {
	diagnostics, err := NewContainerGroupDiagnosticsFromFile(os.Getenv("LOG_ANALYTICS_AUTH_LOCATION"))

	assert.Equal(t, err, nil)
	assert.Check(t, diagnostics != nil || diagnostics.LogAnalytics != nil,
		"Unexpected nil diagnostics. Log Analytics file not parsed correctly")
	assert.Check(t, diagnostics.LogAnalytics.WorkspaceID != nil || diagnostics.LogAnalytics.WorkspaceKey != nil,
		"Unexpected empty analytics authentication credentials. Log Analytics file not parsed correctly")
}

func TestLogAnalyticsFileParsingFailure(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test.*.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewContainerGroupDiagnosticsFromFile(tempFile.Name())
	assert.Error(t, fmt.Errorf("log analytics auth file %q cannot be empty", tempFile.Name()), err.Error())

	// Cleanup
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

}
