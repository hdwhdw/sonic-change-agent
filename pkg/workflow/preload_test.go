package workflow

import (
	"context"
	"testing"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/mocks"
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
	expectedURL := "http://image-repo.example.com/sonic-202505.01-SONiC-Mellanox-2700-ToRRouter-Storage.bin"
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
