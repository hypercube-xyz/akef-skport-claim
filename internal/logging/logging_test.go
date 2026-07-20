package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduledUsesDailyLogAndRotatesOversizedFile(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.Local)
	path := filepath.Join(dir, "scheduled-2026-07-19.log")
	if err := os.WriteFile(path, make([]byte, maxLogSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("old backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	logger, closer, gotPath, err := scheduledAt(cache, slog.LevelInfo, now)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("new entry")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if gotPath != path {
		t.Fatalf("unexpected log path: %s", gotPath)
	}
	backup, err := os.Stat(path + ".1")
	if err != nil || backup.Size() != maxLogSize+1 {
		t.Fatalf("backup was not replaced: info=%v err=%v", backup, err)
	}
	current, err := os.ReadFile(path) // #nosec G304 -- path is inside t.TempDir.
	if err != nil || len(current) == 0 {
		t.Fatalf("new log was not written: %q err=%v", current, err)
	}
}

func TestScheduledRemovesOnlyLogsOlderThan45Days(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC)
	oldPath := filepath.Join(dir, "scheduled-2026-05-01.log")
	recentPath := filepath.Join(dir, "scheduled-2026-06-06.log")
	unrelatedPath := filepath.Join(dir, "keep.txt")
	for _, path := range []string{oldPath, recentPath, unrelatedPath} {
		if err := os.WriteFile(path, []byte("log"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldPath, now.Add(-46*24*time.Hour), now.Add(-46*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentPath, now.Add(-44*24*time.Hour), now.Add(-44*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	logger, closer, _, err := scheduledAt(cache, slog.LevelInfo, now)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("retention test")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired log remains: %v", err)
	}
	for _, path := range []string{recentPath, unrelatedPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained file %s is missing: %v", path, err)
		}
	}
}

func TestParseLevel(t *testing.T) {
	for value, want := range map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError, "unknown": slog.LevelInfo} {
		if got := ParseLevel(value); got != want {
			t.Fatalf("ParseLevel(%q)=%v want %v", value, got, want)
		}
	}
}

func TestLoggerConstructorsAndFilesystemFailures(t *testing.T) {
	if logger := Interactive(slog.LevelWarn); logger == nil {
		t.Fatal("Interactive returned nil")
	}
	cache := t.TempDir()
	logger, closer, path, err := Scheduled(cache, slog.LevelInfo)
	if err != nil || logger == nil || closer == nil || path == "" {
		t.Fatalf("Scheduled() logger=%v closer=%v path=%q err=%v", logger, closer, path, err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := scheduledAt(blocker, slog.LevelInfo, time.Now()); err == nil {
		t.Fatal("scheduled logger below a regular file should fail")
	}
	if _, err := removeExpired(filepath.Join(t.TempDir(), "missing"), time.Now()); err == nil {
		t.Fatal("missing log directory should fail")
	}
}

func TestRotateSmallAndInvalidPaths(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.log")
	if err := rotate(missing); err != nil {
		t.Fatalf("missing log rotation=%v", err)
	}
	small := filepath.Join(t.TempDir(), "small.log")
	if err := os.WriteFile(small, []byte("small"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotate(small); err != nil {
		t.Fatalf("small log rotation=%v", err)
	}
}

func TestScheduledRejectsDirectoryLogAndRotationBackup(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.Local)
	cache := t.TempDir()
	logDir := filepath.Join(cache, "logs")
	if err := os.MkdirAll(filepath.Join(logDir, "scheduled-2026-07-20.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := scheduledAt(cache, slog.LevelInfo, now); err == nil {
		t.Fatal("directory at the daily log path should fail")
	}

	path := filepath.Join(t.TempDir(), "oversized.log")
	if err := os.WriteFile(path, make([]byte, maxLogSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".1", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path+".1", "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotate(path); err == nil {
		t.Fatal("non-empty backup directory should prevent rotation")
	}
}
