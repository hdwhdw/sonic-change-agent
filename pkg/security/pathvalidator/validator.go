package pathvalidator

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePathForDownload validates a file path for download operations
// Only allows absolute paths to /tmp/ or /var/tmp/ with no path traversal
func ValidatePathForDownload(path string) error {
	if path == "" {
		return fmt.Errorf("download path cannot be empty")
	}

	// Must be absolute path
	if !filepath.IsAbs(path) {
		return fmt.Errorf("download path must be absolute, got: %s", path)
	}

	// Clean the path and check allowed prefixes
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, "/tmp/") && !strings.HasPrefix(cleanPath, "/var/tmp/") {
		return fmt.Errorf("download path must start with /tmp/ or /var/tmp/, got: %s", cleanPath)
	}

	return nil
}

// Future functions for other components:
// func ValidatePathForConfig(path string) error { ... }
// func ValidatePathForWorkflow(path string) error { ... }
// func ValidatePathForRemoval(path string) error { ... }
