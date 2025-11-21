package pathvalidator

import (
	"strings"
	"testing"
)

func TestValidatePathForDownload(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		expectError   bool
		errorContains string
	}{
		// Valid paths
		{"valid tmp file", "/tmp/test.json", false, ""},
		{"valid var tmp file", "/var/tmp/image.bin", false, ""},
		{"valid tmp subdirectory", "/tmp/downloads/file.bin", false, ""},
		{"valid var tmp subdirectory", "/var/tmp/cache/data.bin", false, ""},
		{"valid tmp deep subdirectory", "/tmp/a/b/c/file.bin", false, ""},

		// Invalid: empty or relative paths
		{"empty path", "", true, "cannot be empty"},
		{"relative path", "test.json", true, "must be absolute"},
		{"relative with subdirectory", "downloads/file.bin", true, "must be absolute"},
		{"relative with dot", "./test.json", true, "must be absolute"},

		// Invalid: wrong directories
		{"etc directory", "/etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"home directory", "/home/user/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"root directory", "/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"usr directory", "/usr/bin/file", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"var directory", "/var/log/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"opt directory", "/opt/app/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},

		// Invalid: path traversal attempts (cleaned paths will fail prefix check)
		{"traversal from tmp", "/tmp/../etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"traversal from var tmp", "/var/tmp/../etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"complex traversal", "/tmp/dir/../../../etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"dot traversal", "/tmp/./../../etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"nested traversal", "/tmp/a/b/../../../etc/passwd", true, "must be inside /tmp/ or /var/tmp/ directories"},

		// Edge cases
		{"tmp root not allowed", "/tmp", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"var tmp root not allowed", "/var/tmp", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"similar path tmpdir", "/tmpdir/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"similar path vartmp", "/vartmp/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},
		{"tmp prefix but wrong", "/tmp-backup/file.txt", true, "must be inside /tmp/ or /var/tmp/ directories"},

		// More edge cases - these should now be valid after cleaning
		{"double slash", "/tmp//file.txt", false, ""}, // Cleaned to /tmp/file.txt
		{"trailing slash", "/tmp/dir/", false, ""},    // Should be valid
		{"dot in path", "/tmp/./file.txt", false, ""}, // Cleaned to /tmp/file.txt
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathForDownload(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none for path: %s", tt.path)
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v for path: %s", err, tt.path)
				}
			}
		})
	}
}

func TestValidatePathForDownload_SecurityScenarios(t *testing.T) {
	// Test various security attack scenarios
	attackPaths := []string{
		// Classic traversal - should be caught by prefix check after cleaning
		"/tmp/../../../etc/passwd",
		"/var/tmp/../etc/shadow",
		"/tmp/../etc/passwd",         // Direct escape from /tmp
		"/tmp/link/../../etc/passwd", // Escape through subdirectory

		// Mixed case - not /tmp/ or /var/tmp/
		"/TMP/file.txt",
		"/VAR/TMP/file.txt",

		// Wrong directories
		"/etc/passwd",
		"/home/user/file.txt",

		// Null byte injection
		"/tmp/file\x00.txt",
		"/tmp/normal.txt\x00/etc/passwd",
	}

	// These should be allowed (no longer attack scenarios)
	allowedPaths := []string{
		// URL-encoded (if this gets to us decoded, it's fine)
		"/tmp/normal-file.txt",

		// Very long paths (should be allowed if in /tmp/)
		"/tmp/" + strings.Repeat("a", 100) + "/file.txt",

		// This stays within /tmp/ - creates /tmp/etc/passwd which is valid
		"/tmp/link/../etc/passwd",
	}

	for _, path := range attackPaths {
		t.Run("security_test_attack_"+path, func(t *testing.T) {
			err := ValidatePathForDownload(path)
			if err == nil {
				t.Errorf("security test failed: path %q should be rejected", path)
			}
		})
	}

	for _, path := range allowedPaths {
		t.Run("security_test_allowed_"+path, func(t *testing.T) {
			err := ValidatePathForDownload(path)
			if err != nil {
				t.Errorf("allowed path should be valid: path %q got error %v", path, err)
			}
		})
	}
}
