package workflow

import (
	"testing"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/client/mocks"
)

func TestNewWorkflow_Preload(t *testing.T) {
	mockClient := mocks.NewClient()

	workflow, err := NewWorkflow("preload", mockClient)
	if err != nil {
		t.Fatalf("Expected no error for 'preload' workflow, got: %v", err)
	}

	if workflow == nil {
		t.Fatal("Expected workflow to be created, got nil")
	}

	if workflow.GetName() != "preload" {
		t.Errorf("Expected workflow name 'preload', got '%s'", workflow.GetName())
	}
}

func TestNewWorkflow_OSUpgradePreloadImage(t *testing.T) {
	mockClient := mocks.NewClient()

	workflow, err := NewWorkflow("OSUpgrade-PreloadImage", mockClient)
	if err != nil {
		t.Fatalf("Expected no error for 'OSUpgrade-PreloadImage' workflow, got: %v", err)
	}

	if workflow == nil {
		t.Fatal("Expected workflow to be created, got nil")
	}

	if workflow.GetName() != "preload" {
		t.Errorf("Expected workflow name 'preload' (implementation), got '%s'", workflow.GetName())
	}
}

func TestNewWorkflow_UnknownType(t *testing.T) {
	mockClient := mocks.NewClient()

	workflow, err := NewWorkflow("unknown", mockClient)
	if err == nil {
		t.Fatal("Expected error for unknown workflow type, got nil")
	}

	if workflow != nil {
		t.Error("Expected workflow to be nil for unknown type")
	}

	expectedMsg := "unknown workflow type: unknown"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestNewWorkflow_UnsupportedOperations(t *testing.T) {
	mockClient := mocks.NewClient()

	unsupportedWorkflows := []struct {
		name         string
		workflowType string
		description  string
	}{
		{"OSUpgrade-Install", "OSUpgrade-Install", "Install operation not implemented"},
		{"OSUpgrade-Activate", "OSUpgrade-Activate", "Activate operation not implemented"},
		{"ConfigUpdate-ApplyConfig", "ConfigUpdate-ApplyConfig", "ConfigUpdate operation not implemented"},
		{"Reboot-SoftReboot", "Reboot-SoftReboot", "Reboot operation not implemented"},
		{"Reboot-HardReboot", "Reboot-HardReboot", "Hard reboot operation not implemented"},
		{"Backup-CreateBackup", "Backup-CreateBackup", "Backup operation not implemented"},
		{"Test-RunDiagnostics", "Test-RunDiagnostics", "Test operation not implemented"},
		{"", "", "Empty workflow type"},
		{"   ", "   ", "Whitespace-only workflow type"},
		{"InvalidOperation-Action", "InvalidOperation-Action", "Invalid operation format"},
	}

	for _, tc := range unsupportedWorkflows {
		t.Run(tc.name, func(t *testing.T) {
			workflow, err := NewWorkflow(tc.workflowType, mockClient)

			if err == nil {
				t.Fatalf("Expected error for unsupported workflow type '%s', got nil", tc.workflowType)
			}

			if workflow != nil {
				t.Errorf("Expected workflow to be nil for unsupported type '%s'", tc.workflowType)
			}

			expectedMsg := "unknown workflow type: " + tc.workflowType
			if err.Error() != expectedMsg {
				t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
			}
		})
	}
}

func TestNewWorkflow_CaseSensitivity(t *testing.T) {
	mockClient := mocks.NewClient()

	caseSensitiveTests := []struct {
		name         string
		workflowType string
		shouldWork   bool
	}{
		{"Lowercase", "osupgrade-preloadimage", false},
		{"Mixed case", "OsUpgrade-PreloadImage", false},
		{"Wrong case", "OSUPGRADE-PRELOADIMAGE", false},
		{"Correct case", "OSUpgrade-PreloadImage", true},
		{"Preload lowercase", "preload", true}, // Legacy support
	}

	for _, tc := range caseSensitiveTests {
		t.Run(tc.name, func(t *testing.T) {
			workflow, err := NewWorkflow(tc.workflowType, mockClient)

			if tc.shouldWork {
				if err != nil {
					t.Fatalf("Expected no error for valid workflow type '%s', got: %v", tc.workflowType, err)
				}
				if workflow == nil {
					t.Errorf("Expected workflow to be created for valid type '%s'", tc.workflowType)
				}
			} else {
				if err == nil {
					t.Fatalf("Expected error for invalid workflow type '%s', got nil", tc.workflowType)
				}
				if workflow != nil {
					t.Errorf("Expected workflow to be nil for invalid type '%s'", tc.workflowType)
				}
			}
		})
	}
}
