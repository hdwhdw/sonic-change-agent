package pathutil

import "path/filepath"

// Translator handles path translation between container and host filesystem contexts
type Translator struct {
	hostRootFS string // Mount point where host root filesystem is accessible
}

// NewTranslator creates a new path translator
func NewTranslator(hostRootFS string) *Translator {
	return &Translator{
		hostRootFS: hostRootFS,
	}
}

// TranslateToHost translates a container path to the corresponding host filesystem path
// This is needed when the host root filesystem is mounted at a different location in the container
func (t *Translator) TranslateToHost(containerPath string) string {
	cleanPath := filepath.Clean(containerPath)
	return filepath.Join(t.hostRootFS, cleanPath)
}

// GetHostRootFS returns the host root filesystem mount point
func (t *Translator) GetHostRootFS() string {
	return t.hostRootFS
}
