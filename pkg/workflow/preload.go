package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hdwhdw/sonic-change-agent/pkg/gnoi/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

const (
	defaultDownloadPath = "/tmp/sonic-image.bin"
)

// PreloadWorkflow implements the preload workflow using gnoi.file.TransferToRemote
type PreloadWorkflow struct {
	gnoi client.Client
}

// NewPreloadWorkflow creates a new preload workflow
func NewPreloadWorkflow(gnoiClient client.Client) *PreloadWorkflow {
	return &PreloadWorkflow{
		gnoi: gnoiClient,
	}
}

// GetName returns the workflow name
func (w *PreloadWorkflow) GetName() string {
	return "preload"
}

// Execute runs the preload workflow
func (w *PreloadWorkflow) Execute(ctx context.Context, device *unstructured.Unstructured) error {
	// Extract required fields from device spec - new CRD structure
	osVersion, found, _ := unstructured.NestedString(device.Object, "spec", "osVersion")
	if !found || osVersion == "" {
		return fmt.Errorf("osVersion not specified in device spec")
	}

	firmwareProfile, _, _ := unstructured.NestedString(device.Object, "spec", "firmwareProfile")

	// For now, we construct the image URL based on osVersion and firmwareProfile
	// This logic may need to be updated based on actual image repository structure
	imageURL := w.constructImageURL(osVersion, firmwareProfile)
	downloadPath := defaultDownloadPath

	klog.InfoS("Executing preload workflow",
		"osVersion", osVersion,
		"firmwareProfile", firmwareProfile,
		"imageURL", imageURL,
		"downloadPath", downloadPath)

	// Execute single step: Transfer file using gNOI file service
	if err := w.gnoi.File().TransferToRemote(ctx, imageURL, downloadPath); err != nil {
		return fmt.Errorf("failed to transfer file: %w", err)
	}

	klog.InfoS("Preload workflow completed successfully",
		"osVersion", osVersion,
		"downloadPath", downloadPath)

	return nil
}

// constructImageURL builds the image URL based on osVersion and firmwareProfile
func (w *PreloadWorkflow) constructImageURL(osVersion, firmwareProfile string) string {
	baseURL := os.Getenv("IMAGE_REPO_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080/images/" // fallback for testing
	}

	// Ensure trailing slash
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	filename := w.constructFilename(osVersion, firmwareProfile)
	return baseURL + filename
}

// constructFilename builds the firmware filename based on osVersion and firmwareProfile
func (w *PreloadWorkflow) constructFilename(osVersion, firmwareProfile string) string {
	platform := w.getPlatformFromProfile(firmwareProfile)

	// Special case for Broadcom Aboot
	if platform == "broadcom" && w.isAbootProfile(firmwareProfile) {
		return fmt.Sprintf("sonic-aboot-broadcom-%s.swi", osVersion)
	}

	// Standard pattern: sonic-<platform>-<version>.bin
	return fmt.Sprintf("sonic-%s-%s.bin", platform, osVersion)
}

// getPlatformFromProfile extracts the platform name from the firmwareProfile
func (w *PreloadWorkflow) getPlatformFromProfile(firmwareProfile string) string {
	profile := strings.ToLower(firmwareProfile)

	if strings.Contains(profile, "mellanox") {
		return "mellanox"
	}
	if strings.Contains(profile, "broadcom") {
		return "broadcom"
	}
	if strings.Contains(profile, "cisco") {
		return "cisco"
	}
	if strings.Contains(profile, "arista") {
		return "arista"
	}

	// Default fallback
	return "mellanox"
}

// isAbootProfile checks if the firmwareProfile indicates Aboot bootloader
func (w *PreloadWorkflow) isAbootProfile(firmwareProfile string) bool {
	return strings.Contains(strings.ToLower(firmwareProfile), "aboot")
}
