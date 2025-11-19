package client

import (
	"testing"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/client/services/file"
)

func TestGrpcClient_Close_NilConnection(t *testing.T) {
	client := &grpcClient{conn: nil}

	err := client.Close()
	if err != nil {
		t.Errorf("Close with nil connection should not error, got: %v", err)
	}
}

func TestGrpcClient_File(t *testing.T) {
	// Create a mock file service
	fs := file.NewService(nil)
	client := &grpcClient{
		fileService: fs,
	}

	// Test that File() returns the file service
	result := client.File()
	if result != fs {
		t.Error("Expected File() to return the file service")
	}
}
