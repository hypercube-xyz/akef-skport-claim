//go:build unix

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPermissionsRejectsGroupOrWorldAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("version = 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckPermissions(path); err == nil || !strings.Contains(err.Error(), "expose secrets") {
		t.Fatalf("insecure permissions error=%v", err)
	}
}
