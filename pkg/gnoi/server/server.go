package server

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/server/pathutil"
	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/server/services/file"
	gnoi_file "github.com/openconfig/gnoi/file"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"k8s.io/klog/v2"
)

// Server represents the gNOI server
type Server struct {
	address    string
	grpcServer *grpc.Server
	listener   net.Listener
	fileServer *file.Service
	mu         sync.RWMutex
}

// Config holds server configuration
type Config struct {
	Address    string
	HostRootFS string // Mount point of host root filesystem
}

// NewServer creates a new gNOI server
func NewServer(cfg Config) *Server {
	// Default address
	if cfg.Address == "" {
		cfg.Address = "localhost:8080"
	}

	// Default host root filesystem mount point
	if cfg.HostRootFS == "" {
		cfg.HostRootFS = "/tmp/gnoi"
	}

	// Create path translator
	pathTranslator := pathutil.NewTranslator(cfg.HostRootFS)

	return &Server{
		address:    cfg.Address,
		fileServer: file.NewService(pathTranslator),
	}
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	klog.InfoS("Starting gNOI server", "address", s.address)

	// Create listener
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.address, err)
	}

	// Protect listener assignment
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	// Create gRPC server with insecure credentials
	s.grpcServer = grpc.NewServer(
		grpc.Creds(insecure.NewCredentials()),
	)

	// Register services
	gnoi_file.RegisterFileServer(s.grpcServer, s.fileServer)

	// Enable reflection for debugging with grpcurl
	reflection.Register(s.grpcServer)

	// Start serving in goroutine
	errCh := make(chan error, 1)
	go func() {
		klog.InfoS("gNOI server listening", "address", s.address)
		if err := s.grpcServer.Serve(listener); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		klog.InfoS("Shutting down gNOI server")
		s.Stop()
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("gRPC server error: %w", err)
		}
		return nil
	}
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// GetAddress returns the server's listening address
func (s *Server) GetAddress() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.address
}

// GetFileService returns the file service (for testing)
func (s *Server) GetFileService() *file.Service {
	return s.fileServer
}
