package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := ParseLevel(tt.value); got != tt.want {
			t.Errorf("ParseLevel(%q) = %v; want %v", tt.value, got, tt.want)
		}
	}
}

func TestRotate_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.log")
	if err := rotate(path); err != nil {
		t.Errorf("rotate() on nonexistent file: %v", err)
	}
}

func TestRotate_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotate(path); err != nil {
		t.Errorf("rotate() on small file: %v", err)
	}
	// File should still exist.
	if _, err := os.Stat(path); err != nil {
		t.Error("rotate() removed small file unexpectedly")
	}
}

func TestRotate_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.log")
	large := make([]byte, maxLogSize+1)
	if err := os.WriteFile(path, large, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotate(path); err != nil {
		t.Errorf("rotate() on large file: %v", err)
	}
	// Original file should be gone (rotated).
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("rotate() should have moved large file")
	}
	// Backup should exist.
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Error("rotate() did not create backup file")
	}
}

func TestRemoveExpired(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	// Create an old log file.
	oldPath := filepath.Join(dir, "scheduled-2025-01-01.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Set mod time to old date.
	oldTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a recent log file.
	recentPath := filepath.Join(dir, "scheduled-2025-06-10.log")
	if err := os.WriteFile(recentPath, []byte("recent"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a non-log file that should be ignored.
	otherPath := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(otherPath, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleanupErrors, err := removeExpired(dir, now)
	if err != nil {
		t.Fatalf("removeExpired() error: %v", err)
	}
	if len(cleanupErrors) > 0 {
		t.Errorf("removeExpired() cleanup errors: %v", cleanupErrors)
	}

	// Old file should be removed.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("removeExpired() should have removed old log")
	}
	// Recent file should still exist.
	if _, err := os.Stat(recentPath); err != nil {
		t.Error("removeExpired() removed recent log")
	}
	// Other file should not be touched.
	if _, err := os.Stat(otherPath); err != nil {
		t.Error("removeExpired() removed non-log file")
	}
}

func TestScheduledAt(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	logger, closer, path, err := scheduledAt(dir, slog.LevelInfo, now)
	if err != nil {
		t.Fatalf("scheduledAt() error: %v", err)
	}
	if closer == nil {
		t.Fatal("scheduledAt() closer is nil")
	}
	t.Cleanup(func() { _ = closer.Close() })
	if logger == nil {
		t.Fatal("scheduledAt() logger is nil")
	}
	if path == "" {
		t.Error("scheduledAt() path is empty")
	}
	// Verify log file was created.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not found: %v", err)
	}
	// Write through the logger to verify it works.
	logger.Info("test message")
}

func TestInteractive(t *testing.T) {
	logger := Interactive(slog.LevelInfo)
	if logger == nil {
		t.Error("Interactive() returned nil")
	}
}