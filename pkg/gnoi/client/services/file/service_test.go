package file

import (
	"context"
	"os"
	"testing"
)

func TestService_TransferToRemote_DryRun(t *testing.T) {
	// Set DRY_RUN mode
	os.Setenv("DRY_RUN", "true")
	defer os.Unsetenv("DRY_RUN")

	// No need for real gRPC connection in DRY_RUN
	service := &Service{}

	err := service.TransferToRemote(context.Background(), "http://example.com/sonic.bin", "/tmp/test.bin")
	if err != nil {
		t.Errorf("DRY_RUN should not return error, got: %v", err)
	}
}
