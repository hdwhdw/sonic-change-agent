package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/mocks"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestReconcile_NoReconciliationNeeded_NoOperation(t *testing.T) {
	mockClient := mocks.NewClient()

	// Create controller (we'll call reconcile directly, not through informer)
	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Create NetworkDevice object without operation
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
			},
			"status": map[string]interface{}{
				"state":     "Healthy",
				"osVersion": "202505.01",
			},
		},
	}

	// Reconcile should do nothing since no operation is specified
	ctrl.reconcile(obj)

	// Verify no workflow execution was attempted
	if mockClient.GetFileService().GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls, got %d", mockClient.GetFileService().GetTransferToRemoteCallCount())
	}
}

func TestReconcile_PreloadImage_Success(t *testing.T) {
	mockClient := mocks.NewClient()

	// Mock successful transfer
	mockClient.GetFileService().TransferToRemoteFunc = func(ctx context.Context, sourceURL, remotePath string) error {
		return nil
	}

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Create NetworkDevice with OSUpgrade-PreloadImage operation
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"state":                "Healthy",
				"osVersion":            "202505.01",
				"operationState":       "proceed",
				"operationActionState": "proceed",
			},
		},
	}

	// Reconcile should trigger preload workflow
	ctrl.reconcile(obj)

	// Verify transfer was attempted
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() != 1 {
		t.Fatalf("Expected 1 TransferToRemote call, got %d", fileService.GetTransferToRemoteCallCount())
	}

	// Verify transfer parameters (constructed URL based on osVersion and firmwareProfile)
	call, _ := fileService.GetLastTransferToRemoteCall()
	expectedURL := "http://image-repo.example.com/sonic-202505.01-SONiC-Mellanox-2700-ToRRouter-Storage.bin"
	if call.SourceURL != expectedURL {
		t.Errorf("Expected sourceURL '%s', got '%s'", expectedURL, call.SourceURL)
	}

	expectedPath := "/tmp/sonic-image.bin"
	if call.RemotePath != expectedPath {
		t.Errorf("Expected remotePath '%s', got '%s'", expectedPath, call.RemotePath)
	}
}

func TestReconcile_OperationAlreadyCompleted(t *testing.T) {
	mockClient := mocks.NewClient()

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Create NetworkDevice where operation is already completed
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"state":                "Healthy",
				"osVersion":            "202505.01",
				"operationState":       "completed",
				"operationActionState": "completed",
			},
		},
	}

	// Reconcile should do nothing since operation is completed
	ctrl.reconcile(obj)

	// Verify no transfer was attempted
	if mockClient.GetFileService().GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls, got %d", mockClient.GetFileService().GetTransferToRemoteCallCount())
	}
}

func TestReconcile_OperationNotReady(t *testing.T) {
	mockClient := mocks.NewClient()

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Create NetworkDevice where operation state is not ready
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"state":                "Healthy",
				"osVersion":            "202505.01",
				"operationState":       "pending",
				"operationActionState": "pending",
			},
		},
	}

	// Reconcile should skip since operation state is not "proceed"
	ctrl.reconcile(obj)

	// Verify no transfer was attempted
	if mockClient.GetFileService().GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls, got %d", mockClient.GetFileService().GetTransferToRemoteCallCount())
	}
}

func TestReconcile_ConcurrentCalls(t *testing.T) {
	mockClient := mocks.NewClient()

	// Make transfer slow to test mutex
	mockClient.GetFileService().TransferToRemoteFunc = func(ctx context.Context, sourceURL, remotePath string) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"state":                "Healthy",
				"operationState":       "proceed",
				"operationActionState": "proceed",
			},
		},
	}

	// Try to reconcile concurrently
	done := make(chan bool, 2)
	go func() {
		ctrl.reconcile(obj)
		done <- true
	}()
	go func() {
		ctrl.reconcile(obj)
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Due to mutex, verify no crash occurred and at most 2 calls were made
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() > 2 {
		t.Errorf("Expected at most 2 TransferToRemote calls, got %d", fileService.GetTransferToRemoteCallCount())
	}
}

func TestReconcile_WorkflowFailure(t *testing.T) {
	mockClient := mocks.NewClient()

	// Mock failed transfer
	mockClient.GetFileService().TransferToRemoteFunc = func(ctx context.Context, sourceURL, remotePath string) error {
		return fmt.Errorf("network error: connection timeout")
	}

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Mellanox-2700-ToRRouter-Storage",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"operationState":       "proceed",
				"operationActionState": "proceed",
			},
		},
	}

	// Reconcile should attempt workflow and handle error
	ctrl.reconcile(obj)

	// Verify transfer was attempted
	fileService := mockClient.GetFileService()
	if fileService.GetTransferToRemoteCallCount() != 1 {
		t.Errorf("Expected 1 TransferToRemote call, got %d", fileService.GetTransferToRemoteCallCount())
	}

	// Verify status was updated to failed
	operationState, _, _ := unstructured.NestedString(obj.Object, "status", "operationState")
	if operationState != "failed" {
		t.Errorf("Expected operationState 'failed', got '%s'", operationState)
	}

	operationActionState, _, _ := unstructured.NestedString(obj.Object, "status", "operationActionState")
	if operationActionState != "failed" {
		t.Errorf("Expected operationActionState 'failed', got '%s'", operationActionState)
	}

	state, _, _ := unstructured.NestedString(obj.Object, "status", "state")
	if state != "Failed" {
		t.Errorf("Expected state 'Failed', got '%s'", state)
	}
}

func TestReconcile_InvalidObject(t *testing.T) {
	mockClient := mocks.NewClient()

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Invalid object (not unstructured)
	ctrl.reconcile("not an unstructured object")

	// Should not crash, just return early
	if mockClient.GetFileService().GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls for invalid object, got %d", mockClient.GetFileService().GetTransferToRemoteCallCount())
	}
}

func TestReconcile_MissingOSVersion(t *testing.T) {
	mockClient := mocks.NewClient()

	ctrl := &Controller{
		deviceName: "test-device",
		gnoiClient: mockClient,
	}

	// Object without osVersion
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"type":            "leafRouter",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			"status": map[string]interface{}{
				"operationState":       "proceed",
				"operationActionState": "proceed",
			},
		},
	}

	ctrl.reconcile(obj)

	// Should still attempt workflow, but workflow will fail
	if mockClient.GetFileService().GetTransferToRemoteCallCount() != 0 {
		t.Errorf("Expected 0 TransferToRemote calls when workflow fails, got %d", mockClient.GetFileService().GetTransferToRemoteCallCount())
	}

	// Should have failed due to missing osVersion
	operationState, _, _ := unstructured.NestedString(obj.Object, "status", "operationState")
	if operationState != "failed" {
		t.Errorf("Expected operationState 'failed' due to missing osVersion, got '%s'", operationState)
	}
}

func TestUpdateOperationStatus_WithDynamicClient(t *testing.T) {
	// This tests the status update logic itself
	ctrl := &Controller{
		deviceName:    "test-device",
		dynamicClient: nil, // Will skip actual API call
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name":      "test-device",
				"namespace": "default",
			},
			"spec": map[string]interface{}{},
		},
	}

	// Call updateOperationStatus directly
	ctrl.updateOperationStatus(obj, "completed", "completed", "Test message")

	// Verify status was updated in object
	operationState, _, _ := unstructured.NestedString(obj.Object, "status", "operationState")
	if operationState != "completed" {
		t.Errorf("Expected operationState 'completed', got '%s'", operationState)
	}

	operationActionState, _, _ := unstructured.NestedString(obj.Object, "status", "operationActionState")
	if operationActionState != "completed" {
		t.Errorf("Expected operationActionState 'completed', got '%s'", operationActionState)
	}

	state, _, _ := unstructured.NestedString(obj.Object, "status", "state")
	if state != "Healthy" {
		t.Errorf("Expected state 'Healthy', got '%s'", state)
	}

	lastTransitionTime, _, _ := unstructured.NestedString(obj.Object, "status", "lastTransitionTime")
	if lastTransitionTime == "" {
		t.Error("Expected lastTransitionTime to be set")
	}
}

func TestUpdateOperationStatus_StateMapping(t *testing.T) {
	ctrl := &Controller{
		deviceName:    "test-device",
		dynamicClient: nil,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
		},
	}

	// Test different state mappings
	testCases := []struct {
		operationState string
		expectedState  string
	}{
		{"completed", "Healthy"},
		{"failed", "Failed"},
		{"in_progress", "Updating"},
		{"unknown_state", "Unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.operationState, func(t *testing.T) {
			ctrl.updateOperationStatus(obj, tc.operationState, tc.operationState, "Test message")

			state, _, _ := unstructured.NestedString(obj.Object, "status", "state")
			if state != tc.expectedState {
				t.Errorf("Expected state '%s' for operationState '%s', got '%s'",
					tc.expectedState, tc.operationState, state)
			}
		})
	}
}
