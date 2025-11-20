package file

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hdwhdw/sonic-change-agent/pkg/security/pathvalidator"
	"github.com/openconfig/gnoi/common"
	"github.com/openconfig/gnoi/file"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

// HTTPClient interface for mocking in tests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Service implements the gNOI File service
type Service struct {
	file.UnimplementedFileServer
	httpClient HTTPClient
	baseDir    string // Base directory for file operations
}

// NewService creates a new File service
func NewService(baseDir string) *Service {
	return &Service{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		baseDir: baseDir,
	}
}

// SetHTTPClient sets a custom HTTP client (for testing)
func (s *Service) SetHTTPClient(client HTTPClient) {
	s.httpClient = client
}

// TransferToRemote implements the gNOI File.TransferToRemote RPC
func (s *Service) TransferToRemote(ctx context.Context, req *file.TransferToRemoteRequest) (*file.TransferToRemoteResponse, error) {
	klog.InfoS("Received TransferToRemote request",
		"localPath", req.LocalPath,
		"remoteURL", req.RemoteDownload.GetPath())

	// Validate request
	if req.RemoteDownload == nil {
		return nil, status.Error(codes.InvalidArgument, "remote_download is required")
	}

	if req.RemoteDownload.Protocol != common.RemoteDownload_HTTP {
		return nil, status.Errorf(codes.Unimplemented, "only HTTP protocol is supported, got %v", req.RemoteDownload.Protocol)
	}

	remoteURL := req.RemoteDownload.GetPath()
	if remoteURL == "" {
		return nil, status.Error(codes.InvalidArgument, "remote URL path is required")
	}

	localPath := req.LocalPath
	if localPath == "" {
		return nil, status.Error(codes.InvalidArgument, "local path is required")
	}

	// Validate download path for security
	if err := pathvalidator.ValidatePathForDownload(localPath); err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "invalid download path: %v", err)
	}

	cleanPath := filepath.Clean(localPath)

	// Download file
	if err := s.downloadFile(ctx, remoteURL, cleanPath); err != nil {
		klog.ErrorS(err, "Failed to download file",
			"remoteURL", remoteURL,
			"localPath", cleanPath)
		return nil, status.Errorf(codes.Internal, "download failed: %v", err)
	}

	klog.InfoS("File transfer completed successfully",
		"remoteURL", remoteURL,
		"localPath", cleanPath)

	return &file.TransferToRemoteResponse{}, nil
}

// downloadFile downloads a file from URL to local path
func (s *Service) downloadFile(ctx context.Context, url, destPath string) error {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	// Create destination directory
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", destDir, err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer destFile.Close()

	// Copy data
	written, err := io.Copy(destFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	klog.InfoS("Downloaded file successfully",
		"url", url,
		"path", destPath,
		"size", written)

	return nil
}

// Get implements the gNOI File.Get RPC (not implemented)
func (s *Service) Get(req *file.GetRequest, stream file.File_GetServer) error {
	return status.Error(codes.Unimplemented, "Get is not implemented")
}

// Put implements the gNOI File.Put RPC (not implemented)
func (s *Service) Put(stream file.File_PutServer) error {
	return status.Error(codes.Unimplemented, "Put is not implemented")
}

// Stat implements the gNOI File.Stat RPC (not implemented)
func (s *Service) Stat(ctx context.Context, req *file.StatRequest) (*file.StatResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Stat is not implemented")
}

// Remove implements the gNOI File.Remove RPC (not implemented)
func (s *Service) Remove(ctx context.Context, req *file.RemoveRequest) (*file.RemoveResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Remove is not implemented")
}
