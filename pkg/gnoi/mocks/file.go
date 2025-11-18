package mocks

import (
	"context"
	"fmt"
	"sync"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi"
)

// FileService is a mock implementation of FileService
type FileService struct {
	mu sync.Mutex

	// Mock behavior
	TransferToRemoteFunc func(ctx context.Context, sourceURL, remotePath string) error

	// Call tracking
	TransferToRemoteCalls []TransferToRemoteCall
}

type TransferToRemoteCall struct {
	SourceURL  string
	RemotePath string
}

// NewFileService creates a new mock file service with default behaviors
func NewFileService() *FileService {
	return &FileService{
		TransferToRemoteFunc: func(ctx context.Context, sourceURL, remotePath string) error {
			return nil
		},
	}
}

// TransferToRemote implements FileService.TransferToRemote
func (f *FileService) TransferToRemote(ctx context.Context, sourceURL, remotePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.TransferToRemoteCalls = append(f.TransferToRemoteCalls, TransferToRemoteCall{
		SourceURL:  sourceURL,
		RemotePath: remotePath,
	})

	return f.TransferToRemoteFunc(ctx, sourceURL, remotePath)
}

// GetTransferToRemoteCallCount returns the number of TransferToRemote calls
func (f *FileService) GetTransferToRemoteCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.TransferToRemoteCalls)
}

// GetLastTransferToRemoteCall returns the last TransferToRemote call
func (f *FileService) GetLastTransferToRemoteCall() (TransferToRemoteCall, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.TransferToRemoteCalls) == 0 {
		return TransferToRemoteCall{}, fmt.Errorf("no TransferToRemote calls recorded")
	}
	return f.TransferToRemoteCalls[len(f.TransferToRemoteCalls)-1], nil
}

// ResetCalls resets all call tracking
func (f *FileService) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.TransferToRemoteCalls = nil
}

// Ensure FileService implements gnoi.FileService interface
var _ gnoi.FileService = (*FileService)(nil)
