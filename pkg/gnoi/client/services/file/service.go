package file

import (
	"context"
	"fmt"
	"os"

	"github.com/openconfig/gnoi/common"
	"github.com/openconfig/gnoi/file"
	"k8s.io/klog/v2"
)

// Service implements the File service for gNOI operations
type Service struct {
	client file.FileClient
}

// NewService creates a new File service
func NewService(client file.FileClient) *Service {
	return &Service{
		client: client,
	}
}

// TransferToRemote transfers a file from URL to remote path using gnoi.file service
func (s *Service) TransferToRemote(ctx context.Context, sourceURL, remotePath string) error {
	klog.InfoS("Starting file transfer via gNOI file service",
		"sourceURL", sourceURL,
		"remotePath", remotePath)

	// Check if DRY_RUN mode
	if os.Getenv("DRY_RUN") == "true" {
		klog.InfoS("DRY_RUN: Would transfer file via gNOI file.TransferToRemote",
			"sourceURL", sourceURL,
			"remotePath", remotePath)
		return nil
	}

	// Create TransferToRemote request
	req := &file.TransferToRemoteRequest{
		LocalPath: remotePath,
		RemoteDownload: &common.RemoteDownload{
			Path:     sourceURL,
			Protocol: common.RemoteDownload_HTTP,
		},
	}

	// Execute the transfer
	resp, err := s.client.TransferToRemote(ctx, req)
	if err != nil {
		return fmt.Errorf("file transfer failed: %w", err)
	}

	klog.InfoS("File transfer completed successfully",
		"response", resp.String(),
		"remotePath", remotePath)

	return nil
}
