package file

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/openconfig/gnoi/common"
	"github.com/openconfig/gnoi/file"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MockHTTPClient for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestService_TransferToRemote_Success(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	service := NewService(tempDir)

	// Mock HTTP client
	mockContent := "test file content"
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(mockContent)),
			}, nil
		},
	}
	service.SetHTTPClient(mockClient)

	// Execute request
	req := &file.TransferToRemoteRequest{
		LocalPath: "/tmp/test/file.bin",
		RemoteDownload: &common.RemoteDownload{
			Path:     "http://example.com/file.bin",
			Protocol: common.RemoteDownload_HTTP,
		},
	}

	resp, err := service.TransferToRemote(context.Background(), req)
	if err != nil {
		t.Fatalf("TransferToRemote failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	// Verify file was created
	expectedPath := "/tmp/test/file.bin"
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != mockContent {
		t.Errorf("File content mismatch. Got %q, want %q", string(content), mockContent)
	}
}

func TestService_TransferToRemote_HTTPError(t *testing.T) {
	tempDir := t.TempDir()
	service := NewService(tempDir)

	// Mock HTTP client returning 404
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       io.NopCloser(bytes.NewBuffer(nil)),
			}, nil
		},
	}
	service.SetHTTPClient(mockClient)

	req := &file.TransferToRemoteRequest{
		LocalPath: "/tmp/test/file.bin",
		RemoteDownload: &common.RemoteDownload{
			Path:     "http://example.com/missing.bin",
			Protocol: common.RemoteDownload_HTTP,
		},
	}

	_, err := service.TransferToRemote(context.Background(), req)
	if err == nil {
		t.Fatal("Expected error for HTTP 404")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got %v", err)
	}

	if st.Code() != codes.Internal {
		t.Errorf("Expected Internal error code, got %v", st.Code())
	}

	if !strings.Contains(st.Message(), "404") {
		t.Errorf("Error message should mention 404 status: %v", st.Message())
	}
}

func TestService_TransferToRemote_ValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	service := NewService(tempDir)

	tests := []struct {
		name        string
		req         *file.TransferToRemoteRequest
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name: "missing remote_download",
			req: &file.TransferToRemoteRequest{
				LocalPath: "/tmp/test.bin",
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: "remote_download is required",
		},
		{
			name: "unsupported protocol",
			req: &file.TransferToRemoteRequest{
				LocalPath: "/tmp/test.bin",
				RemoteDownload: &common.RemoteDownload{
					Path:     "ftp://example.com/file.bin",
					Protocol: common.RemoteDownload_SFTP,
				},
			},
			wantCode:    codes.Unimplemented,
			wantMessage: "only HTTP protocol is supported",
		},
		{
			name: "missing remote path",
			req: &file.TransferToRemoteRequest{
				LocalPath: "/tmp/test.bin",
				RemoteDownload: &common.RemoteDownload{
					Path:     "",
					Protocol: common.RemoteDownload_HTTP,
				},
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: "remote URL path is required",
		},
		{
			name: "missing local path",
			req: &file.TransferToRemoteRequest{
				LocalPath: "",
				RemoteDownload: &common.RemoteDownload{
					Path:     "http://example.com/file.bin",
					Protocol: common.RemoteDownload_HTTP,
				},
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: "local path is required",
		},
		{
			name: "path traversal attempt",
			req: &file.TransferToRemoteRequest{
				LocalPath: "../../../etc/passwd",
				RemoteDownload: &common.RemoteDownload{
					Path:     "http://example.com/file.bin",
					Protocol: common.RemoteDownload_HTTP,
				},
			},
			wantCode:    codes.PermissionDenied,
			wantMessage: "must be absolute",
		},
		{
			name: "invalid directory",
			req: &file.TransferToRemoteRequest{
				LocalPath: "/etc/passwd",
				RemoteDownload: &common.RemoteDownload{
					Path:     "http://example.com/file.bin",
					Protocol: common.RemoteDownload_HTTP,
				},
			},
			wantCode:    codes.PermissionDenied,
			wantMessage: "must start with /tmp/ or /var/tmp/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.TransferToRemote(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Expected error")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got %v", err)
			}

			if st.Code() != tt.wantCode {
				t.Errorf("Got code %v, want %v", st.Code(), tt.wantCode)
			}

			if !strings.Contains(st.Message(), tt.wantMessage) {
				t.Errorf("Message %q doesn't contain %q", st.Message(), tt.wantMessage)
			}
		})
	}
}

func TestService_TransferToRemote_AbsolutePath(t *testing.T) {
	service := NewService("/tmp")

	// Mock successful download
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("content")),
			}, nil
		},
	}
	service.SetHTTPClient(mockClient)

	// Request with absolute path in /tmp/
	targetPath := "/tmp/downloads/file.bin"
	req := &file.TransferToRemoteRequest{
		LocalPath: targetPath,
		RemoteDownload: &common.RemoteDownload{
			Path:     "http://example.com/file.bin",
			Protocol: common.RemoteDownload_HTTP,
		},
	}

	_, err := service.TransferToRemote(context.Background(), req)
	if err != nil {
		t.Fatalf("TransferToRemote failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Expected file was not created")
	}
}

func TestService_UnimplementedRPCs(t *testing.T) {
	service := NewService("/tmp")

	// Test Get
	err := service.Get(&file.GetRequest{}, nil)
	if st := status.Convert(err); st.Code() != codes.Unimplemented {
		t.Errorf("Get: expected Unimplemented, got %v", st.Code())
	}

	// Test Put
	err = service.Put(nil)
	if st := status.Convert(err); st.Code() != codes.Unimplemented {
		t.Errorf("Put: expected Unimplemented, got %v", st.Code())
	}

	// Test Stat
	_, err = service.Stat(context.Background(), &file.StatRequest{})
	if st := status.Convert(err); st.Code() != codes.Unimplemented {
		t.Errorf("Stat: expected Unimplemented, got %v", st.Code())
	}

	// Test Remove
	_, err = service.Remove(context.Background(), &file.RemoveRequest{})
	if st := status.Convert(err); st.Code() != codes.Unimplemented {
		t.Errorf("Remove: expected Unimplemented, got %v", st.Code())
	}
}
