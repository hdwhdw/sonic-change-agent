package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/hdwhdw/sonic-change-agent/pkg/config"
	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/server"
	"k8s.io/klog/v2"
)

func main() {
	// Command line flags
	var (
		fallbackAddress = flag.String("address", "localhost:8080", "Fallback gRPC server address if Redis config unavailable")
		hostRootFS      = flag.String("host-root-fs", "/tmp/gnoi", "Mount point of host root filesystem (typically /mnt/host)")
	)

	// Initialize klog
	klog.InitFlags(nil)
	flag.Parse()

	// Try to read configuration from Redis first
	var serverAddress string
	gnoiConfig, err := config.GetGNOIConfigFromRedis()
	if err != nil {
		klog.InfoS("Failed to read gNOI config from Redis, using fallback",
			"error", err,
			"fallbackAddress", *fallbackAddress)
		serverAddress = *fallbackAddress
	} else {
		serverAddress = gnoiConfig.GetGNOIEndpoint()
		klog.InfoS("Read gNOI configuration from Redis",
			"address", serverAddress,
			"port", gnoiConfig.Port,
			"useTLS", gnoiConfig.UseTLS)
	}

	// Log startup
	klog.InfoS("Starting gNOI light server",
		"version", "0.1.0",
		"address", serverAddress,
		"hostRootFS", *hostRootFS)

	// Create server
	cfg := server.Config{
		Address:    serverAddress,
		HostRootFS: *hostRootFS,
	}
	srv := server.NewServer(cfg)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		klog.InfoS("Received signal", "signal", sig)
		cancel()
	}()

	// Start server
	if err := srv.Start(ctx); err != nil {
		klog.ErrorS(err, "Server failed")
		os.Exit(1)
	}

	klog.InfoS("Server stopped")
}
