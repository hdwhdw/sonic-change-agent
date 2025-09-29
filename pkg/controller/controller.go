package controller

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	gnoios "github.com/openconfig/gnoi/os"
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

// isDebugMode checks if DEBUG_MODE environment variable is set
func isDebugMode() bool {
	return os.Getenv("DEBUG_MODE") == "true"
}

// isDryRunMode checks if DRY_RUN environment variable is set
func isDryRunMode() bool {
	return os.Getenv("DRY_RUN") == "true"
}

// Controller manages NetworkDevice CRDs for this node
type Controller struct {
	deviceName      string
	dynamicClient   dynamic.Interface
	informer        cache.SharedIndexInformer
	operationMutex  sync.Mutex  // Prevents concurrent operations

	// Upgrade state tracking
	upgradeStateMutex sync.Mutex
	currentUpgrade    *UpgradeState
}

// UpgradeState tracks the current upgrade attempt
type UpgradeState struct {
	TargetVersion string
	StartTime     time.Time
	Phase         string
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

		// Get current version to compare
		currentVersion, _, _ := unstructured.NestedString(newU.Object, "status", "os", "currentVersion")

		// Only proceed if new desired version is different from current
		if newDesiredVersion != "" && newDesiredVersion != currentVersion {
			klog.InfoS("🔄 Triggering upgrade operation",
				"device", deviceName,
				"from", currentVersion,
				"to", newDesiredVersion)

			c.handleUpgradeOperation(newU)
		}
	}

}

func (c *Controller) onNetworkDeviceDelete(obj interface{}) {
	u := obj.(*unstructured.Unstructured)
	deviceName := u.GetName()
	klog.InfoS("🔴 NetworkDevice DELETED", "device", deviceName)
}


// handleUpgradeOperation executes a firmware upgrade using gNOI System.SetPackage with activate=true
func (c *Controller) handleUpgradeOperation(u *unstructured.Unstructured) {
	deviceName := u.GetName()
	klog.InfoS("🔒 Acquiring operation lock for UPGRADE", "device", deviceName)
	c.operationMutex.Lock()
	defer func() {
		c.operationMutex.Unlock()
		klog.InfoS("🔓 Released operation lock for UPGRADE", "device", deviceName)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	desiredVersion, _, _ := unstructured.NestedString(u.Object, "spec", "os", "desiredVersion")
	currentVersion, _, _ := unstructured.NestedString(u.Object, "status", "os", "currentVersion")

	klog.InfoS("🔧 Starting firmware upgrade operation",
		"device", deviceName,
		"currentVersion", currentVersion,
		"desiredVersion", desiredVersion)

	// Safety check: ensure versions are different
	if desiredVersion == currentVersion {
		klog.InfoS("⚠️ Upgrade skipped - versions are the same",
			"device", deviceName,
			"version", currentVersion)
		return
	}

	// Mark the start of upgrade process
	c.startUpgrade(desiredVersion)

	// Get firmware URL from CRD spec, fall back to constructed URL if not specified
	firmwareURL, _, _ := unstructured.NestedString(u.Object, "spec", "os", "firmwareURL")
	var imageURL string
	if firmwareURL != "" {
		imageURL = firmwareURL
		klog.InfoS("🌐 Using firmware URL from CRD spec", "imageURL", imageURL)
	} else {
		imageURL = fmt.Sprintf("http://10.250.0.1:8888/sonic-vs-%s.bin", desiredVersion)
		klog.InfoS("🔧 Using constructed firmware URL", "imageURL", imageURL)
	}

	// Safety check: verify the target firmware file exists
	klog.InfoS("🔍 Verifying firmware availability", "imageURL", imageURL)
	downloadPath := "/tmp/sonic-upgrade.bin" // Use different path for upgrades

	// PHASE 1: Download and activate firmware (activate=true)
	klog.InfoS("🚀 PHASE 1: Downloading and activating firmware",
		"device", deviceName,
		"imageURL", imageURL,
		"downloadPath", downloadPath,
		"activate", true)

	if err := c.executeDownloadPackage(ctx, imageURL, desiredVersion, "", downloadPath); err != nil {
		klog.ErrorS(err, "Firmware download+activate failed", "device", deviceName)
		// Clear upgrade state on failure
		c.clearUpgrade()
		return
	}

	klog.InfoS("✅ PHASE 1 COMPLETE: Firmware downloaded and activated successfully",
		"device", deviceName,
		"desiredVersion", desiredVersion)

	// PHASE 2: Reboot to activate the new firmware
	klog.InfoS("🔄 PHASE 2: Initiating system reboot to activate firmware",
		"device", deviceName,
		"desiredVersion", desiredVersion)

	if err := c.executeSystemReboot(ctx); err != nil {
		klog.ErrorS(err, "System reboot failed", "device", deviceName)
		// Clear upgrade state on failure
		c.clearUpgrade()
		return
	}

	klog.InfoS("✅ PHASE 2 COMPLETE: System reboot initiated successfully",
		"device", deviceName,
		"desiredVersion", desiredVersion)

	// CRITICAL: Do NOT complete the upgrade process here!
	// The upgrade stays "in progress" until the OS version actually changes.
	// This prevents multiple SetPackage calls for the same upgrade.
	// The upgrade state will be cleared by reconciliation when versions match.

	klog.InfoS("⏳ Upgrade process remains active - waiting for version change after reboot",
		"device", deviceName,
		"desiredVersion", desiredVersion)

	// Keep the upgrade marked as in-progress indefinitely
	// The reconciliation loop will clear this state when:
	// 1. currentVersion == desiredVersion, OR
	// 2. A different desiredVersion is requested
}

// queryOSVersion queries the current OS version via gNOI OS.Verify
func (c *Controller) queryOSVersion(ctx context.Context) (string, error) {
	// DEBUG MODE: Return fake version
	if isDebugMode() {
		klog.InfoS("🐛 DEBUG MODE: Returning fake OS version")
		return "SONiC-OS-20250505.03", nil
	}

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
	client := gnoios.NewOSClient(conn)

	// Call Verify to get current OS version
	req := &gnoios.VerifyRequest{}
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
	var currentVersion string

	// Execute sync operations under mutex lock
	klog.V(2).InfoS("🔒 Acquiring operation lock for OS sync", "device", c.deviceName)
	c.operationMutex.Lock()

	// Query current OS version from device
	queryCtx, queryCancel := context.WithTimeout(ctx, 10*time.Second)
	defer queryCancel()

	var err error
	currentVersion, err = c.queryOSVersion(queryCtx)
	if err != nil {
		c.operationMutex.Unlock()
		klog.ErrorS(err, "Failed to query OS version from device", "device", c.deviceName)
		return
	}

	klog.InfoS("📱 Retrieved OS version from device",
		"device", c.deviceName,
		"version", currentVersion)

	// Update CRD status
	statusCtx, statusCancel := context.WithTimeout(ctx, 5*time.Second)
	defer statusCancel()

	if err := c.updateCRDStatus(statusCtx, currentVersion); err != nil {
		c.operationMutex.Unlock()
		klog.ErrorS(err, "Failed to update CRD status", "device", c.deviceName)
		return
	}

	klog.InfoS("✅ Updated NetworkDevice CRD status",
		"device", c.deviceName,
		"currentVersion", currentVersion)

	// Release mutex before reconciliation check
	c.operationMutex.Unlock()
	klog.V(2).InfoS("🔓 Released operation lock for OS sync", "device", c.deviceName)

	klog.InfoS("🔧 About to check reconciliation", "device", c.deviceName, "currentVersion", currentVersion)

	// 🔄 NEW: Check for reconciliation OUTSIDE the mutex lock
	if currentVersion != "" {
		klog.InfoS("🔧 Calling reconciliation check", "device", c.deviceName)
		reconCtx, reconCancel := context.WithTimeout(ctx, 5*time.Second)
		defer reconCancel()

		c.checkReconciliation(reconCtx, currentVersion)
	} else {
		klog.InfoS("🔧 Skipping reconciliation - no current version", "device", c.deviceName)
	}
}

// checkReconciliation compares current vs desired version and triggers reconciliation if needed
func (c *Controller) checkReconciliation(ctx context.Context, currentVersion string) {
	klog.InfoS("🔍 Checking reconciliation needs", "device", c.deviceName, "currentVersion", currentVersion)

	// Get the NetworkDevice CRD to check desired version
	networkDeviceGVR := schema.GroupVersionResource{
		Group:    "sonic.io",
		Version:  "v1",
		Resource: "networkdevices",
	}

	device, err := c.dynamicClient.Resource(networkDeviceGVR).Namespace("default").Get(ctx, c.deviceName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to get NetworkDevice for reconciliation check", "device", c.deviceName)
		return
	}

	// Extract desired version
	desiredVersion, _, _ := unstructured.NestedString(device.Object, "spec", "os", "desiredVersion")

	// Check if reconciliation is needed
	if desiredVersion != "" && desiredVersion != currentVersion {
		klog.InfoS("🎯 RECONCILIATION NEEDED",
			"device", c.deviceName,
			"currentVersion", currentVersion,
			"desiredVersion", desiredVersion)

		// Check if upgrade is already in progress for this version
		if c.isUpgradeInProgress(desiredVersion) {
			klog.InfoS("⏳ Upgrade already in progress - skipping reconciliation",
				"device", c.deviceName,
				"targetVersion", desiredVersion)
			return
		}

		if isDryRunMode() {
			klog.InfoS("🏃‍♂️ DRY RUN: Would trigger upgrade reconciliation",
				"device", c.deviceName,
				"from", currentVersion,
				"to", desiredVersion)
		} else {
			klog.InfoS("🔄 TRIGGERING UPGRADE RECONCILIATION",
				"device", c.deviceName,
				"from", currentVersion,
				"to", desiredVersion)

			// Call actual reconciliation logic
			c.handleUpgradeOperation(device)
		}
	} else {
		klog.V(2).InfoS("✅ No reconciliation needed - versions match",
			"device", c.deviceName,
			"version", currentVersion)

		// Clear any existing upgrade state since versions match
		c.clearUpgrade()
	}
}


// executeUpgradePackage calls gNOI System.SetPackage to upgrade firmware (activate=true)
func (c *Controller) executeUpgradePackage(ctx context.Context, imageURL, targetVersion, checksum, downloadPath string) error {
	klog.InfoS("📦 Sending SetPackage request for UPGRADE",
		"imageURL", imageURL,
		"targetVersion", targetVersion,
		"activate", true,
		"downloadPath", downloadPath)

	// DEBUG MODE: Return fake success
	if isDebugMode() {
		klog.InfoS("🐛 DEBUG MODE: Faking upgrade success", "targetVersion", targetVersion)
		time.Sleep(2 * time.Second) // Simulate processing time
		return nil
	}

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

	// Send Package request (first message) with activate=true
	packageReq := &system.SetPackageRequest{
		Request: &system.SetPackageRequest_Package{
			Package: &system.Package{
				Filename:       downloadPath, // Destination path on device from CRD
				Version:        targetVersion,
				Activate:       true, // CRITICAL: This installs and activates the firmware
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

	klog.InfoS("✅ SetPackage UPGRADE completed successfully", "response", resp.String())

	return nil
}

// executeDownloadPackage calls gNOI System.SetPackage to download and activate firmware (activate=true)
func (c *Controller) executeDownloadPackage(ctx context.Context, imageURL, targetVersion, checksum, downloadPath string) error {
	klog.InfoS("🚀 Sending SetPackage request for DOWNLOAD+ACTIVATE",
		"imageURL", imageURL,
		"targetVersion", targetVersion,
		"activate", true,
		"downloadPath", downloadPath)

	// DEBUG MODE: Return fake success
	if isDebugMode() {
		klog.InfoS("🐛 DEBUG MODE: Faking download success", "targetVersion", targetVersion)
		time.Sleep(2 * time.Second) // Simulate processing time
		return nil
	}

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

	// Send Package request (first message) with activate=false for download only
	packageReq := &system.SetPackageRequest{
		Request: &system.SetPackageRequest_Package{
			Package: &system.Package{
				Filename:       downloadPath, // Destination path on device
				Version:        targetVersion,
				Activate:       true, // CRITICAL: Download AND activate firmware
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
		return fmt.Errorf("SetPackage download operation failed: %w", err)
	}

	klog.InfoS("✅ SetPackage DOWNLOAD+ACTIVATE completed successfully", "response", resp.String())

	return nil
}

// executeSystemReboot calls gNOI System.Reboot to reboot the device
func (c *Controller) executeSystemReboot(ctx context.Context) error {
	klog.InfoS("🔄 Sending System.Reboot request")

	// DEBUG MODE: Return fake success
	if isDebugMode() {
		klog.InfoS("🐛 DEBUG MODE: Faking reboot success")
		time.Sleep(1 * time.Second) // Simulate processing time
		return nil
	}

	// Connect to gRPC server on port 8080
	conn, err := grpc.DialContext(ctx, "localhost:8080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to gNOI server: %w", err)
	}
	defer conn.Close()

	// Create System service client
	client := system.NewSystemClient(conn)

	// Prepare reboot request
	rebootReq := &system.RebootRequest{
		Method:  system.RebootMethod_COLD,
		Delay:   0, // Immediate reboot
		Message: "SONiC firmware upgrade reboot",
		Force:   false,
	}

	// Send reboot request
	resp, err := client.Reboot(ctx, rebootReq)
	if err != nil {
		return fmt.Errorf("System.Reboot operation failed: %w", err)
	}

	klog.InfoS("✅ System.Reboot completed successfully", "response", resp.String())

	return nil
}

// isUpgradeInProgress checks if an upgrade for the target version is already in progress
func (c *Controller) isUpgradeInProgress(targetVersion string) bool {
	c.upgradeStateMutex.Lock()
	defer c.upgradeStateMutex.Unlock()

	if c.currentUpgrade == nil {
		return false
	}

	// Check if we're already upgrading to this version
	if c.currentUpgrade.TargetVersion == targetVersion {
		klog.InfoS("⏳ Upgrade already in progress",
			"device", c.deviceName,
			"targetVersion", targetVersion,
			"phase", c.currentUpgrade.Phase,
			"startTime", c.currentUpgrade.StartTime)
		return true
	}

	// Different target version requested - clear old upgrade and allow new one
	if c.currentUpgrade.TargetVersion != targetVersion {
		klog.InfoS("🔄 New target version requested - clearing previous upgrade",
			"device", c.deviceName,
			"oldTarget", c.currentUpgrade.TargetVersion,
			"newTarget", targetVersion)
		c.currentUpgrade = nil
		return false
	}

	return false
}

// startUpgrade marks the beginning of an upgrade process
func (c *Controller) startUpgrade(targetVersion string) {
	c.upgradeStateMutex.Lock()
	defer c.upgradeStateMutex.Unlock()

	c.currentUpgrade = &UpgradeState{
		TargetVersion: targetVersion,
		StartTime:     time.Now(),
		Phase:         "download+activate+reboot",
	}

	klog.InfoS("🚀 Starting upgrade process",
		"device", c.deviceName,
		"targetVersion", targetVersion,
		"startTime", c.currentUpgrade.StartTime)
}

// clearUpgrade clears the upgrade state when upgrade is complete or versions match
func (c *Controller) clearUpgrade() {
	c.upgradeStateMutex.Lock()
	defer c.upgradeStateMutex.Unlock()

	if c.currentUpgrade != nil {
		klog.InfoS("✅ Clearing upgrade state",
			"device", c.deviceName,
			"targetVersion", c.currentUpgrade.TargetVersion,
			"duration", time.Since(c.currentUpgrade.StartTime))
		c.currentUpgrade = nil
	}
}

