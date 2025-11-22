package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNetworkDeviceCRDSchema(t *testing.T) {
	// Test that NetworkDevice resources follow expected schema
	testCases := []struct {
		name          string
		device        *unstructured.Unstructured
		shouldBeValid bool
		description   string
	}{
		{
			name: "Valid complete NetworkDevice",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name":      "sonic-device-01",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"type":            "leafRouter",
						"osVersion":       "202505.01",
						"firmwareProfile": "SONiC-Test-Profile",
						"operation":       "OSUpgrade",
						"operationAction": "PreloadImage",
					},
					"status": map[string]interface{}{
						"state":                "Healthy",
						"osVersion":            "202505.01",
						"operationState":       "proceed",
						"operationActionState": "proceed",
						"lastTransitionTime":   "2023-11-21T10:00:00Z",
					},
				},
			},
			shouldBeValid: true,
			description:   "Standard valid NetworkDevice resource",
		},
		{
			name: "Minimal valid NetworkDevice",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name": "minimal-device",
					},
					"spec": map[string]interface{}{
						"type": "leafRouter",
					},
				},
			},
			shouldBeValid: true,
			description:   "Minimal NetworkDevice without optional fields",
		},
		{
			name: "Invalid API version",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name": "invalid-device",
					},
					"spec": map[string]interface{}{
						"type": "leafRouter",
					},
				},
			},
			shouldBeValid: false,
			description:   "Wrong API version",
		},
		{
			name: "Invalid kind",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "Device",
					"metadata": map[string]interface{}{
						"name": "invalid-device",
					},
					"spec": map[string]interface{}{
						"type": "leafRouter",
					},
				},
			},
			shouldBeValid: false,
			description:   "Wrong kind",
		},
		{
			name: "Missing metadata name",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata":   map[string]interface{}{},
					"spec": map[string]interface{}{
						"type": "leafRouter",
					},
				},
			},
			shouldBeValid: false,
			description:   "Missing required metadata.name",
		},
		{
			name: "Missing spec",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name": "no-spec-device",
					},
				},
			},
			shouldBeValid: false,
			description:   "Missing required spec section",
		},
		{
			name: "Valid operation combinations",
			device: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name": "operation-device",
					},
					"spec": map[string]interface{}{
						"type":            "leafRouter",
						"operation":       "OSUpgrade",
						"operationAction": "PreloadImage",
						"osVersion":       "202505.01",
					},
				},
			},
			shouldBeValid: true,
			description:   "Valid operation and operationAction combination",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid := validateNetworkDeviceSchema(tc.device)
			if valid != tc.shouldBeValid {
				t.Errorf("Expected validation result %t for %s, got %t",
					tc.shouldBeValid, tc.description, valid)
			}
		})
	}
}

func TestNetworkDeviceGVK(t *testing.T) {
	device := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
		},
	}

	expectedGVK := schema.GroupVersionKind{
		Group:   "sonic.k8s.io",
		Version: "v1",
		Kind:    "NetworkDevice",
	}

	gvk := device.GetObjectKind().GroupVersionKind()
	if gvk != expectedGVK {
		t.Errorf("Expected GVK %+v, got %+v", expectedGVK, gvk)
	}
}

func TestNetworkDeviceStatusFields(t *testing.T) {
	// Test status field validation and access
	device := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "sonic.k8s.io/v1",
			"kind":       "NetworkDevice",
			"metadata": map[string]interface{}{
				"name": "status-test-device",
			},
			"spec": map[string]interface{}{
				"type": "leafRouter",
			},
			"status": map[string]interface{}{
				"state":                "Healthy",
				"osVersion":            "202505.01",
				"operationState":       "completed",
				"operationActionState": "completed",
				"lastTransitionTime":   "2023-11-21T10:00:00Z",
			},
		},
	}

	// Test status field access
	state, found, err := unstructured.NestedString(device.Object, "status", "state")
	if err != nil {
		t.Fatalf("Error accessing status.state: %v", err)
	}
	if !found {
		t.Error("status.state field not found")
	}
	if state != "Healthy" {
		t.Errorf("Expected state 'Healthy', got '%s'", state)
	}

	// Test nested status fields
	operationState, found, err := unstructured.NestedString(device.Object, "status", "operationState")
	if err != nil {
		t.Fatalf("Error accessing status.operationState: %v", err)
	}
	if !found {
		t.Error("status.operationState field not found")
	}
	if operationState != "completed" {
		t.Errorf("Expected operationState 'completed', got '%s'", operationState)
	}

	// Test setting status fields
	err = unstructured.SetNestedField(device.Object, "Failed", "status", "state")
	if err != nil {
		t.Fatalf("Error setting status.state: %v", err)
	}

	newState, found, err := unstructured.NestedString(device.Object, "status", "state")
	if err != nil {
		t.Fatalf("Error accessing updated status.state: %v", err)
	}
	if !found {
		t.Error("Updated status.state field not found")
	}
	if newState != "Failed" {
		t.Errorf("Expected updated state 'Failed', got '%s'", newState)
	}
}

func TestNetworkDeviceSpecValidation(t *testing.T) {
	testCases := []struct {
		name     string
		spec     map[string]interface{}
		isValid  bool
		testDesc string
	}{
		{
			name: "Valid complete spec",
			spec: map[string]interface{}{
				"type":            "leafRouter",
				"osVersion":       "202505.01",
				"firmwareProfile": "SONiC-Test-Profile",
				"operation":       "OSUpgrade",
				"operationAction": "PreloadImage",
			},
			isValid:  true,
			testDesc: "All fields present and valid",
		},
		{
			name: "Valid minimal spec",
			spec: map[string]interface{}{
				"type": "leafRouter",
			},
			isValid:  true,
			testDesc: "Only required type field",
		},
		{
			name: "Invalid device type",
			spec: map[string]interface{}{
				"type": "unknownDevice",
			},
			isValid:  false,
			testDesc: "Invalid device type",
		},
		{
			name: "Operation without action",
			spec: map[string]interface{}{
				"type":      "leafRouter",
				"operation": "OSUpgrade",
			},
			isValid:  false,
			testDesc: "Operation specified without operationAction",
		},
		{
			name: "Action without operation",
			spec: map[string]interface{}{
				"type":            "leafRouter",
				"operationAction": "PreloadImage",
			},
			isValid:  false,
			testDesc: "OperationAction specified without operation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			device := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "sonic.k8s.io/v1",
					"kind":       "NetworkDevice",
					"metadata": map[string]interface{}{
						"name": "test-device",
					},
					"spec": tc.spec,
				},
			}

			valid := validateNetworkDeviceSpec(device)
			if valid != tc.isValid {
				t.Errorf("Expected spec validation %t for %s, got %t",
					tc.isValid, tc.testDesc, valid)
			}
		})
	}
}

// Helper functions for validation
func validateNetworkDeviceSchema(device *unstructured.Unstructured) bool {
	// Check API version
	if device.GetAPIVersion() != "sonic.k8s.io/v1" {
		return false
	}

	// Check kind
	if device.GetKind() != "NetworkDevice" {
		return false
	}

	// Check required metadata
	if device.GetName() == "" {
		return false
	}

	// Check spec exists
	spec, found, err := unstructured.NestedMap(device.Object, "spec")
	if err != nil || !found || spec == nil {
		return false
	}

	return validateNetworkDeviceSpec(device)
}

func validateNetworkDeviceSpec(device *unstructured.Unstructured) bool {
	// Check required type field
	deviceType, found, err := unstructured.NestedString(device.Object, "spec", "type")
	if err != nil || !found || deviceType == "" {
		return false
	}

	// Validate device type
	validTypes := []string{"leafRouter", "spineRouter", "borderRouter"}
	isValidType := false
	for _, validType := range validTypes {
		if deviceType == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return false
	}

	// Check operation/operationAction consistency
	operation, hasOperation, _ := unstructured.NestedString(device.Object, "spec", "operation")
	operationAction, hasOperationAction, _ := unstructured.NestedString(device.Object, "spec", "operationAction")

	// Both should be present or both should be absent
	if hasOperation != hasOperationAction {
		return false
	}

	// If present, validate operation combinations
	if hasOperation && hasOperationAction {
		validCombinations := map[string][]string{
			"OSUpgrade":    {"PreloadImage", "Install", "Activate"},
			"ConfigUpdate": {"ApplyConfig"},
			"Reboot":       {"SoftReboot", "HardReboot"},
		}

		validActions, ok := validCombinations[operation]
		if !ok {
			return false
		}

		isValidAction := false
		for _, validAction := range validActions {
			if operationAction == validAction {
				isValidAction = true
				break
			}
		}
		if !isValidAction {
			return false
		}
	}

	return true
}
