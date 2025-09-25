package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/openconfig/gnoi/os"
	"github.com/openconfig/gnoi/system"
	"github.com/openconfig/gnoi/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// Controller manages NetworkDevice CRDs for this node
type Controller struct {
	deviceName    string
	dynamicClient dynamic.Interface
	informer      cache.SharedIndexInformer
}

// NewController creates a new CRD controller
func NewController(deviceName string) (*Controller, error) {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// When using hostNetwork, we need to override the host to use the real API server
	// instead of the cluster service IP (which is not reachable from host network)
	config.Host = "https://10.52.0.72:6443"

	// Create dynamic client for CRDs
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	controller := &Controller{
		deviceName:    deviceName,
		dynamicClient: dynamicClient,
	}

	// Set up informer for NetworkDevice CRDs
	if err := controller.setupInformer(); err != nil {
		return nil, fmt.Errorf("failed to setup informer: %w", err)
	}

	return controller, nil
}

// setupInformer creates the CRD informer with field selectors
func (c *Controller) setupInformer() error {
	// Define the NetworkDevice GVR (Group/Version/Resource)
	networkDeviceGVR := schema.GroupVersionResource{
		Group:    "sonic.io",
		Version:  "v1",
		Resource: "networkdevices",
	}

	// Create a field selector to watch only our device
	fieldSelector := fields.OneTermEqualSelector("metadata.name", c.deviceName)

	// Create list/watch functions
	listWatchFunc := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector.String()
			return c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").List(context.TODO(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector.String()
			return c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").Watch(context.TODO(), options)
		},
	}

	// Create the informer
	c.informer = cache.NewSharedIndexInformer(
		listWatchFunc,
		&unstructured.Unstructured{}, // Object type
		time.Minute*5,                // Resync period
		cache.Indexers{},             // No custom indexers needed
	)

	// Add event handlers
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onNetworkDeviceAdd,
		UpdateFunc: c.onNetworkDeviceUpdate,
		DeleteFunc: c.onNetworkDeviceDelete,
	})

	return nil
}

// Run starts the controller
func (c *Controller) Run(ctx context.Context) error {
	klog.InfoS("Starting CRD controller", "device", c.deviceName)

	// Start the informer
	go c.informer.Run(ctx.Done())

	// Wait for cache sync
	klog.InfoS("Waiting for cache sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("failed to sync cache")
	}
	klog.InfoS("Cache synced successfully")

	// Keep running until context is cancelled
	<-ctx.Done()
	klog.InfoS("CRD controller stopped")
	return nil
}

// Event handlers for NetworkDevice CRD changes

func (c *Controller) onNetworkDeviceAdd(obj interface{}) {
	u := obj.(*unstructured.Unstructured)
	deviceName := u.GetName()

	// Extract fields using unstructured access
	desiredVersion, _, _ := unstructured.NestedString(u.Object, "spec", "os", "desiredVersion")
	currentVersion, _, _ := unstructured.NestedString(u.Object, "status", "os", "currentVersion")

	klog.InfoS("🟢 NetworkDevice ADDED",
		"device", deviceName,
		"desiredVersion", desiredVersion,
		"currentVersion", currentVersion)

	// Check if there's a preload operation
	requestID, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "requestId")
	if requestID != "" {
		targetVersion, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "targetVersion")
		phase, _, _ := unstructured.NestedString(u.Object, "status", "preload", "phase")

		klog.InfoS("📦 Preload operation detected",
			"device", deviceName,
			"requestId", requestID,
			"targetVersion", targetVersion,
			"phase", phase)
	}
}

func (c *Controller) onNetworkDeviceUpdate(oldObj, newObj interface{}) {
	oldU := oldObj.(*unstructured.Unstructured)
	newU := newObj.(*unstructured.Unstructured)

	deviceName := newU.GetName()
	generation := newU.GetGeneration()

	klog.InfoS("🟡 NetworkDevice UPDATED",
		"device", deviceName,
		"generation", generation)

	// Check for version changes
	oldDesiredVersion, _, _ := unstructured.NestedString(oldU.Object, "spec", "os", "desiredVersion")
	newDesiredVersion, _, _ := unstructured.NestedString(newU.Object, "spec", "os", "desiredVersion")

	if oldDesiredVersion != newDesiredVersion {
		klog.InfoS("📋 Desired version changed",
			"device", deviceName,
			"old", oldDesiredVersion,
			"new", newDesiredVersion)
	}

	// Check for new preload operations
	oldRequestID, _, _ := unstructured.NestedString(oldU.Object, "spec", "preload", "requestId")
	newRequestID, _, _ := unstructured.NestedString(newU.Object, "spec", "preload", "requestId")

	if oldRequestID != newRequestID {
		targetVersion, _, _ := unstructured.NestedString(newU.Object, "spec", "preload", "targetVersion")
		imageURL, _, _ := unstructured.NestedString(newU.Object, "spec", "preload", "imageURL")

		klog.InfoS("🚀 New preload operation requested",
			"device", deviceName,
			"requestId", newRequestID,
			"targetVersion", targetVersion,
			"imageURL", imageURL)

		// This is where we would start the actual preload process
		c.handlePreloadOperation(newU)
	}
}

func (c *Controller) onNetworkDeviceDelete(obj interface{}) {
	u := obj.(*unstructured.Unstructured)
	deviceName := u.GetName()
	klog.InfoS("🔴 NetworkDevice DELETED", "device", deviceName)
}

// handlePreloadOperation executes a real firmware preload using gNOI System.SetPackage
func (c *Controller) handlePreloadOperation(u *unstructured.Unstructured) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	deviceName := u.GetName()
	requestID, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "requestId")
	targetVersion, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "targetVersion")
	imageURL, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "imageURL")
	checksum, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "checksum", "md5")
	downloadPath, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "downloadPath")
	if downloadPath == "" {
		downloadPath = "/tmp/sonic-firmware.bin" // Default fallback
	}

	klog.InfoS("🔧 Starting firmware preload operation",
		"device", deviceName,
		"requestId", requestID,
		"targetVersion", targetVersion,
		"imageURL", imageURL)

	// Update status to InProgress
	if err := c.updatePreloadStatus(ctx, "InProgress", 0, "Starting firmware download and preload"); err != nil {
		klog.ErrorS(err, "Failed to update preload status to InProgress")
	}

	// Execute the actual preload
	if err := c.executeSetPackage(ctx, imageURL, targetVersion, checksum, downloadPath); err != nil {
		klog.ErrorS(err, "Firmware preload failed", "device", deviceName, "requestId", requestID)
		c.updatePreloadStatus(ctx, "Failed", 0, fmt.Sprintf("Preload failed: %v", err))
		return
	}

	// Update status to Succeeded
	if err := c.updatePreloadStatus(ctx, "Succeeded", 100, "Firmware successfully preloaded"); err != nil {
		klog.ErrorS(err, "Failed to update preload status to Succeeded")
	}

	klog.InfoS("✅ Firmware preload completed successfully",
		"device", deviceName,
		"requestId", requestID,
		"targetVersion", targetVersion)
}

// queryOSVersion queries the current OS version via gNOI OS.Verify
func (c *Controller) queryOSVersion(ctx context.Context) (string, error) {
	endpoint := fmt.Sprintf("%s:8080", c.deviceName)

	// Create gRPC connection
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to connect to gNOI server %s: %w", endpoint, err)
	}
	defer conn.Close()

	// Create OS service client
	client := os.NewOSClient(conn)

	// Call Verify to get current OS version
	req := &os.VerifyRequest{}
	resp, err := client.Verify(ctx, req)
	if err != nil {
		return "", fmt.Errorf("gNOI OS.Verify failed: %w", err)
	}

	return resp.Version, nil
}

// updateCRDStatus updates the NetworkDevice CRD status with current OS version
func (c *Controller) updateCRDStatus(ctx context.Context, currentVersion string) error {
	networkDeviceGVR := schema.GroupVersionResource{
		Group:    "sonic.io",
		Version:  "v1",
		Resource: "networkdevices",
	}

	// Get current NetworkDevice resource
	device, err := c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").Get(ctx, c.deviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get NetworkDevice %s: %w", c.deviceName, err)
	}

	// Update status.os.currentVersion
	if device.Object["status"] == nil {
		device.Object["status"] = make(map[string]interface{})
	}
	status := device.Object["status"].(map[string]interface{})

	if status["os"] == nil {
		status["os"] = make(map[string]interface{})
	}
	osStatus := status["os"].(map[string]interface{})
	osStatus["currentVersion"] = currentVersion

	// Update the status subresource
	_, err = c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").UpdateStatus(ctx, device, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update NetworkDevice %s status: %w", c.deviceName, err)
	}

	return nil
}

// StartOSVersionSync starts a periodic sync of OS version from device to CRD status
func (c *Controller) StartOSVersionSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do initial sync immediately
	c.syncOSVersion(ctx)

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("OS version sync stopped")
			return
		case <-ticker.C:
			c.syncOSVersion(ctx)
		}
	}
}

// syncOSVersion performs a single OS version sync
func (c *Controller) syncOSVersion(ctx context.Context) {
	klog.V(2).InfoS("🔍 Syncing OS version from device", "device", c.deviceName)

	// Query current OS version from device
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	currentVersion, err := c.queryOSVersion(ctx)
	if err != nil {
		klog.ErrorS(err, "Failed to query OS version from device", "device", c.deviceName)
		return
	}

	klog.InfoS("📱 Retrieved OS version from device",
		"device", c.deviceName,
		"version", currentVersion)

	// Update CRD status
	if err := c.updateCRDStatus(ctx, currentVersion); err != nil {
		klog.ErrorS(err, "Failed to update CRD status", "device", c.deviceName)
		return
	}

	klog.InfoS("✅ Updated NetworkDevice CRD status",
		"device", c.deviceName,
		"currentVersion", currentVersion)
}

// executeSetPackage calls gNOI System.SetPackage to preload firmware (activate=false)
func (c *Controller) executeSetPackage(ctx context.Context, imageURL, targetVersion, checksum, downloadPath string) error {
	endpoint := fmt.Sprintf("%s:8080", c.deviceName)

	// Create gRPC connection
	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to gNOI server %s: %w", endpoint, err)
	}
	defer conn.Close()

	// Create System service client
	client := system.NewSystemClient(conn)

	// Create streaming client
	stream, err := client.SetPackage(ctx)
	if err != nil {
		return fmt.Errorf("failed to create SetPackage stream: %w", err)
	}

	klog.InfoS("📦 Sending SetPackage request",
		"imageURL", imageURL,
		"targetVersion", targetVersion,
		"activate", false)

	// Send Package request (first message)
	packageReq := &system.SetPackageRequest{
		Request: &system.SetPackageRequest_Package{
			Package: &system.Package{
				Filename:       downloadPath, // Destination path on device from CRD
				Version:        targetVersion,
				Activate:       false, // Critical: preload only, don't activate
				RemoteDownload: &common.RemoteDownload{
					Path:     imageURL, // Source URL to download from
					Protocol: common.RemoteDownload_HTTP,
				},
			},
		},
	}

	if err := stream.Send(packageReq); err != nil {
		return fmt.Errorf("failed to send package request: %w", err)
	}

	// Close send side and receive response
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("SetPackage operation failed: %w", err)
	}

	klog.InfoS("✅ SetPackage completed successfully",
		"response", resp.String())

	return nil
}

// updatePreloadStatus updates the preload status in the CRD
func (c *Controller) updatePreloadStatus(ctx context.Context, phase string, progress int, message string) error {
	networkDeviceGVR := schema.GroupVersionResource{
		Group:    "sonic.io",
		Version:  "v1",
		Resource: "networkdevices",
	}

	// Get current NetworkDevice resource
	device, err := c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").Get(ctx, c.deviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get NetworkDevice %s: %w", c.deviceName, err)
	}

	// Update status.preload
	if device.Object["status"] == nil {
		device.Object["status"] = make(map[string]interface{})
	}
	status := device.Object["status"].(map[string]interface{})

	if status["preload"] == nil {
		status["preload"] = make(map[string]interface{})
	}
	preloadStatus := status["preload"].(map[string]interface{})

	preloadStatus["phase"] = phase
	preloadStatus["progress"] = progress
	preloadStatus["message"] = message

	// Update the status subresource
	_, err = c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").UpdateStatus(ctx, device, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update NetworkDevice %s preload status: %w", c.deviceName, err)
	}

	klog.InfoS("📊 Updated preload status",
		"device", c.deviceName,
		"phase", phase,
		"progress", progress,
		"message", message)

	return nil
}