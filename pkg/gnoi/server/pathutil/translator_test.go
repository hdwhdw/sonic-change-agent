package pathutil

import (
	"testing"
)

func TestNewTranslator(t *testing.T) {
	hostRootFS := "/mnt/host"
	translator := NewTranslator(hostRootFS)

	if translator.hostRootFS != hostRootFS {
		t.Errorf("Expected hostRootFS %s, got %s", hostRootFS, translator.hostRootFS)
	}

	if translator.GetHostRootFS() != hostRootFS {
		t.Errorf("Expected GetHostRootFS() to return %s, got %s", hostRootFS, translator.GetHostRootFS())
	}
}

func TestTranslateToHost(t *testing.T) {
	tests := []struct {
		name          string
		hostRootFS    string
		containerPath string
		expectedPath  string
	}{
		{
			name:          "simple absolute path",
			hostRootFS:    "/mnt/host",
			containerPath: "/tmp/test.bin",
			expectedPath:  "/mnt/host/tmp/test.bin",
		},
		{
			name:          "path with subdirectories",
			hostRootFS:    "/mnt/host",
			containerPath: "/var/tmp/downloads/file.bin",
			expectedPath:  "/mnt/host/var/tmp/downloads/file.bin",
		},
		{
			name:          "path that needs cleaning",
			hostRootFS:    "/mnt/host",
			containerPath: "/tmp//test/../final.bin",
			expectedPath:  "/mnt/host/tmp/final.bin",
		},
		{
			name:          "root path",
			hostRootFS:    "/mnt/host",
			containerPath: "/",
			expectedPath:  "/mnt/host",
		},
		{
			name:          "different host mount point",
			hostRootFS:    "/host-fs",
			containerPath: "/tmp/test.bin",
			expectedPath:  "/host-fs/tmp/test.bin",
		},
		{
			name:          "empty host root",
			hostRootFS:    "",
			containerPath: "/tmp/test.bin",
			expectedPath:  "/tmp/test.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.hostRootFS)
			result := translator.TranslateToHost(tt.containerPath)

			if result != tt.expectedPath {
				t.Errorf("Expected %s, got %s", tt.expectedPath, result)
			}
		})
	}
}

func TestTranslateToHost_ConsistentCleaning(t *testing.T) {
	translator := NewTranslator("/mnt/host")

	// Test that multiple calls with equivalent paths return the same result
	paths := []string{
		"/tmp/test.bin",
		"/tmp//test.bin",
		"/tmp/./test.bin",
		"/tmp/subdir/../test.bin",
	}

	expectedResult := "/mnt/host/tmp/test.bin"

	for i, path := range paths {
		result := translator.TranslateToHost(path)
		if result != expectedResult {
			t.Errorf("Path %d (%s): expected %s, got %s", i, path, expectedResult, result)
		}
	}
}
