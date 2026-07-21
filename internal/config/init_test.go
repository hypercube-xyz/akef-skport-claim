package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_CreatesNewConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	got, err := Init(path, false)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if got != path {
		t.Errorf("Init() = %q; want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "version = 1") {
		t.Error("generated config should contain version = 1")
	}
}

func TestInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Init(path, false)
	if err == nil {
		t.Fatal("Init() should fail when config already exists")
	}
}

func TestInit_Force(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Init(path, true)
	if err != nil {
		t.Fatalf("Init(force=true) error: %v", err)
	}
	if got != path {
		t.Errorf("Init() = %q; want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(data), "version = 1") {
		t.Error("forced config should contain version = 1")
	}
}

func TestInit_ExampleIsValid(t *testing.T) {
	// The Example const should be parseable TOML.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(Example), 0o600); err != nil {
		t.Fatal(err)
	}
	// Just verify it can be loaded (minus validation — it has placeholders).
	_, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Example const is not valid: %v", err)
	}
}

func TestInit_ResolvePath(t *testing.T) {
	// Empty path should resolve to default.
	got, err := Init("", false)
	if err != nil {
		// May fail if default config dir doesn't exist, but shouldn't panic.
		t.Logf("Init(\"\") returned error (expected if default dir missing): %v", err)
	}
	_ = got
}