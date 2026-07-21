//go:build unix

package config

import (
	"os"
	"testing"
)

func TestCheckPermissions_RegularFile(t *testing.T) {
	path := t.TempDir() + "/good.conf"
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckPermissions(path); err != nil {
		t.Errorf("CheckPermissions(0600) error: %v", err)
	}
}

func TestCheckPermissions_WorldReadable(t *testing.T) {
	path := t.TempDir() + "/bad.conf"
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckPermissions(path); err == nil {
		t.Error("CheckPermissions(0644) should fail")
	}
}

func TestCheckPermissions_NotExist(t *testing.T) {
	if err := CheckPermissions("/nonexistent/path.conf"); err == nil {
		t.Error("CheckPermissions() should fail for missing file")
	}
}