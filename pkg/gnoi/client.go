package gnoi

import (
	"context"
	"fmt"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/services/file"
	gnoifile "github.com/openconfig/gnoi/file"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// FileService defines the interface for gNOI File operations
type FileService interface {
	// TransferToRemote transfers a file from URL to remote path
	TransferToRemote(ctx context.Context, sourceURL, remotePath string) error
}

// Client is the main gNOI client interface
type Client interface {
	// File returns the File service
	File() FileService
	// Close closes the gRPC connection
	Close() error
}

// grpcClient implements Client using real gRPC calls
type grpcClient struct {
	conn        *grpc.ClientConn
	fileService FileService
}

// NewClient creates a new gNOI client
func NewClient(endpoint string) (Client, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to dial gNOI server: %w", err)
	}

	// Create file client and service
	fileClient := gnoifile.NewFileClient(conn)
	fileService := file.NewService(fileClient)

	return &grpcClient{
		conn:        conn,
		fileService: fileService,
	}, nil
}

// File returns the File service
func (c *grpcClient) File() FileService {
	return c.fileService
}

// Close closes the gRPC connection
func (c *grpcClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ensure grpcClient implements Client interface
var _ Client = (*grpcClient)(nil)
