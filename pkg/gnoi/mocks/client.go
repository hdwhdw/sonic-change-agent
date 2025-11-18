package mocks

import (
	"sync"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi"
)

// Client is a mock implementation of Client for testing
type Client struct {
	mu sync.Mutex

	// Services
	fileService *FileService

	// Mock behaviors
	CloseFunc func() error

	// Call tracking
	CloseCalls int
}

// NewClient creates a new mock client with default behaviors
func NewClient() *Client {
	mockFileService := NewFileService()

	return &Client{
		fileService: mockFileService,
		CloseFunc: func() error {
			return nil
		},
	}
}

// File returns the mock File service
func (m *Client) File() gnoi.FileService {
	return m.fileService
}

// Close closes the mock connection
func (m *Client) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CloseCalls++
	return m.CloseFunc()
}

// GetFileService returns the mock file service for test assertions
func (m *Client) GetFileService() *FileService {
	return m.fileService
}

// ResetCalls resets all call tracking
func (m *Client) ResetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fileService.ResetCalls()
	m.CloseCalls = 0
}

// Ensure Client implements gnoi.Client interface
var _ gnoi.Client = (*Client)(nil)
