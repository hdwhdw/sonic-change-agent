package controller

import (
	"context"
	"fmt"
	"time"

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

// handlePreloadOperation simulates handling a preload operation
func (c *Controller) handlePreloadOperation(u *unstructured.Unstructured) {
	deviceName := u.GetName()
	requestID, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "requestId")
	targetVersion, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "targetVersion")

	klog.InfoS("🔧 Processing preload operation",
		"device", deviceName,
		"requestId", requestID,
		"targetVersion", targetVersion)

	// In the future, this would:
	// 1. Update status to "InProgress"
	// 2. Call gNOI System.SetPackage RPC
	// 3. Stream progress updates
	// 4. Update status to "Succeeded" or "Failed"

	imageURL, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "imageURL")
	checksum, _, _ := unstructured.NestedString(u.Object, "spec", "preload", "checksum", "md5")

	klog.InfoS("✅ Preload operation would be executed here",
		"device", deviceName,
		"imageURL", imageURL,
		"checksum", checksum)
}