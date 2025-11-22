package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/client/mocks"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPreloadWorkflow_GetName(t *testing.T) {
	mockClient := mocks.NewClient()
	workflow := NewPreloadWorkflow(mockClient)

	if workflow.GetName() != "preload" {
		t.Errorf("Expected workflow name 'preload', got '%s'", workflow.GetName())
	}
}

func TestPreloadWorkflow_Execute(t *testing.T) {
	mockClient := mocks.NewClient()
	workflow := NewPreloadWorkflow(mockClient)

	// Create a mock device with new CRD structure
	device := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
			},
		},
	}

	// Execute workflow
	err := workflow.Execute(context.Background(), device)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify TransferToRemote was called
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() != 1 {
		t.Fatalf("Expected 1 TransferToRemote call, got %d", fileService.GetTransferToRemoteCallCount())
	}

	call, _ := fileService.GetLastTransferToRemoteCall()
	expectedURL := "http://localhost:8080/images/sonic-mellanox-202505.01.bin"
	if call.SourceURL != expectedURL {
		t.Errorf("Expected sourceURL '%s', got '%s'", expectedURL, call.SourceURL)
	}
	if call.RemotePath != "/tmp/sonic-image.bin" {
		t.Errorf("Expected remotePath '/tmp/sonic-image.bin', got '%s'", call.RemotePath)
	}
}

func TestPreloadWorkflow_Execute_MissingOSVersion(t *testing.T) {
	mockClient := mocks.NewClient()
	workflow := NewPreloadWorkflow(mockClient)

	// Create a mock device without osVersion
	device := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
			},
		},
	}

	// Execute workflow
	err := workflow.Execute(context.Background(), device)
	if err == nil {
		t.Fatalf("Expected error for missing osVersion, got nil")
	}

	// Verify no transfer was attempted
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls, got %d", fileService.GetTransferToRemoteCallCount())
	}
}

func TestPreloadWorkflow_Execute_EmptyOSVersion(t *testing.T) {
	mockClient := mocks.NewClient()
	workflow := NewPreloadWorkflow(mockClient)

	// Create a mock device with empty osVersion
	device := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"osVersion":       "",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
			},
		},
	}

	// Execute workflow
	err := workflow.Execute(context.Background(), device)
	if err == nil {
		t.Fatalf("Expected error for empty osVersion, got nil")
	}

	// Verify no transfer was attempted
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls, got %d", fileService.GetTransferToRemoteCallCount())
	}
}

func TestPreloadWorkflow_PlatformExtraction(t *testing.T) {
	testCases := []struct {
		firmwareProfile  string
		expectedPlatform string
		expectedAboot    bool
		expectedFilename string
	}{
		{
			firmwareProfile:  "SONiC-Mellanox-2700-ToRRouter-Storage",
			expectedPlatform: "mellanox",
			expectedAboot:    false,
			expectedFilename: "sonic-mellanox-202505.01.bin",
		},
		{
			firmwareProfile:  "SONiC-Broadcom-Profile",
			expectedPlatform: "broadcom",
			expectedAboot:    false,
			expectedFilename: "sonic-broadcom-202505.01.bin",
		},
		{
			firmwareProfile:  "SONiC-Broadcom-Aboot-Profile",
			expectedPlatform: "broadcom",
			expectedAboot:    true,
			expectedFilename: "sonic-aboot-broadcom-202505.01.swi",
		},
		{
			firmwareProfile:  "SONiC-Cisco-Profile",
			expectedPlatform: "cisco",
			expectedAboot:    false,
			expectedFilename: "sonic-cisco-202505.01.bin",
		},
		{
			firmwareProfile:  "SONiC-Arista-Profile",
			expectedPlatform: "arista",
			expectedAboot:    false,
			expectedFilename: "sonic-arista-202505.01.bin",
		},
		{
			firmwareProfile:  "SONiC-Unknown-Profile",
			expectedPlatform: "mellanox", // default fallback
			expectedAboot:    false,
			expectedFilename: "sonic-mellanox-202505.01.bin",
		},
		{
			firmwareProfile:  "SONiC-Test-Profile",
			expectedPlatform: "mellanox", // default fallback
			expectedAboot:    false,
			expectedFilename: "sonic-mellanox-202505.01.bin",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.firmwareProfile, func(t *testing.T) {
			mockClient := mocks.NewClient()
			workflow := NewPreloadWorkflow(mockClient)

			// Test platform extraction
			platform := workflow.getPlatformFromProfile(tc.firmwareProfile)
			if platform != tc.expectedPlatform {
				t.Errorf("Expected platform '%s', got '%s'", tc.expectedPlatform, platform)
			}

			// Test aboot detection
			isAboot := workflow.isAbootProfile(tc.firmwareProfile)
			if isAboot != tc.expectedAboot {
				t.Errorf("Expected aboot %t, got %t", tc.expectedAboot, isAboot)
			}

			// Test filename construction
			filename := workflow.constructFilename("202505.01", tc.firmwareProfile)
			if filename != tc.expectedFilename {
				t.Errorf("Expected filename '%s', got '%s'", tc.expectedFilename, filename)
			}
		})
	}
}

func TestPreloadWorkflow_ConfigurationVariations(t *testing.T) {
	testCases := []struct {
		name            string
		osVersion       string
		firmwareProfile string
		expectedURL     string
		shouldSucceed   bool
		errorContains   string
	}{
		{
			name:            "Standard configuration",
			osVersion:       "202505.01",
			firmwareProfile: "SONiC-Profile-A",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.01.bin",
			shouldSucceed:   true,
		},
		{
			name:            "Different OS version",
			osVersion:       "202505.02",
			firmwareProfile: "SONiC-Profile-B",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.02.bin",
			shouldSucceed:   true,
		},
		{
			name:            "Long firmware profile name",
			osVersion:       "202505.01",
			firmwareProfile: "SONiC-Mellanox-2700-ToRRouter-Storage",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.01.bin",
			shouldSucceed:   true,
		},
		{
			name:            "Version with build number",
			osVersion:       "202505.01-build.123",
			firmwareProfile: "SONiC-Profile-Test",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.01-build.123.bin",
			shouldSucceed:   true,
		},
		{
			name:            "Beta version",
			osVersion:       "202505.01-beta",
			firmwareProfile: "SONiC-Beta-Profile",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.01-beta.bin",
			shouldSucceed:   true,
		},
		{
			name:            "Missing OS version",
			firmwareProfile: "SONiC-Profile-A",
			shouldSucceed:   false,
			errorContains:   "osVersion not specified in device spec",
		},
		{
			name:            "Empty OS version",
			osVersion:       "",
			firmwareProfile: "SONiC-Profile-A",
			shouldSucceed:   false,
			errorContains:   "osVersion not specified in device spec",
		},
		{
			name:            "Whitespace OS version allowed",
			osVersion:       "   ",
			firmwareProfile: "SONiC-Profile-A",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-   .bin",
			shouldSucceed:   true,
		},
		{
			name:          "Missing firmware profile uses default",
			osVersion:     "202505.01",
			expectedURL:   "http://localhost:8080/images/sonic-mellanox-202505.01.bin",
			shouldSucceed: true,
		},
		{
			name:            "Empty firmware profile uses default",
			osVersion:       "202505.01",
			firmwareProfile: "",
			expectedURL:     "http://localhost:8080/images/sonic-mellanox-202505.01.bin",
			shouldSucceed:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := mocks.NewClient()
			workflow := NewPreloadWorkflow(mockClient)

			// Create device with test configuration
			device := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			}

			// Set osVersion if provided
			if tc.osVersion != "" {
				device.Object["spec"].(map[string]interface{})["osVersion"] = tc.osVersion
			}

			// Set firmwareProfile if provided
			if tc.firmwareProfile != "" {
				device.Object["spec"].(map[string]interface{})["firmwareProfile"] = tc.firmwareProfile
			}

			// Execute workflow
			err := workflow.Execute(context.Background(), device)

			if tc.shouldSucceed {
				if err != nil {
					t.Fatalf("Expected success for %s, got error: %v", tc.name, err)
				}

				// Verify transfer was called with correct parameters
				fileService := mockClient.GetFileService()
				if fileService.GetTransferToRemoteCallCount() != 1 {
					t.Fatalf("Expected 1 TransferToRemote call, got %d", fileService.GetTransferToRemoteCallCount())
				}

				call, _ := fileService.GetLastTransferToRemoteCall()
				if call.SourceURL != tc.expectedURL {
					t.Errorf("Expected sourceURL '%s', got '%s'", tc.expectedURL, call.SourceURL)
				}

				if call.RemotePath != "/tmp/sonic-image.bin" {
					t.Errorf("Expected remotePath '/tmp/sonic-image.bin', got '%s'", call.RemotePath)
				}
			} else {
				if err == nil {
					t.Fatalf("Expected error for %s, got nil", tc.name)
				}

				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.errorContains, err.Error())
				}

				// Verify no transfer was attempted
				fileService := mockClient.GetFileService()
				if fileService.GetTransferToRemoteCallCount() != 0 {
					t.Errorf("Expected 0 TransferToRemote calls for failed case, got %d", fileService.GetTransferToRemoteCallCount())
				}
			}
		})
	}
}
