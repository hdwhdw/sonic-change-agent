package server

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestServer_StartStop(t *testing.T) {
	cfg := Config{
		Address: "localhost:0", // Use port 0 for automatic assignment
		BaseDir: t.TempDir(),
	}

	server := NewServer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	errCh := make(chan error)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Verify we can connect
	addr := server.GetAddress()
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	conn.Close()

	// Stop server
	cancel()

	// Wait for server to stop
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Server returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestServer_ListenError(t *testing.T) {
	// Listen on a port first
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create test listener: %v", err)
	}
	addr := listener.Addr().String()
	defer listener.Close()

	// Try to start server on same port
	cfg := Config{
		Address: addr,
		BaseDir: t.TempDir(),
	}

	server := NewServer(cfg)
	ctx := context.Background()

	err = server.Start(ctx)
	if err == nil {
		t.Fatal("Expected error when port is already in use")
	}
}

func TestServer_DefaultConfig(t *testing.T) {
	server := NewServer(Config{})

	if server.address != "localhost:8080" {
		t.Errorf("Expected default address localhost:8080, got %s", server.address)
	}

	if server.fileServer == nil {
		t.Error("File service should not be nil")
	}
}

func TestServer_GetFileService(t *testing.T) {
	server := NewServer(Config{
		BaseDir: "/test/dir",
	})

	fs := server.GetFileService()
	if fs == nil {
		t.Fatal("GetFileService returned nil")
	}
}
