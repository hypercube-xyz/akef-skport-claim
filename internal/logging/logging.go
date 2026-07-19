package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxLogSize   = 5 << 20
	logRetention = 45 * 24 * time.Hour
)

func Interactive(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func Scheduled(cacheDir string, level slog.Level) (*slog.Logger, io.Closer, string, error) {
	return scheduledAt(cacheDir, level, time.Now())
}

func scheduledAt(cacheDir string, level slog.Level, now time.Time) (*slog.Logger, io.Closer, string, error) {
	dir := filepath.Join(cacheDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, "", fmt.Errorf("create log directory: %w", err)
	}
	cleanupErrors, err := removeExpired(dir, now)
	if err != nil {
		return nil, nil, "", err
	}
	path := filepath.Join(dir, "scheduled-"+now.Format("2006-01-02")+".log")
	if err := rotate(path); err != nil {
		return nil, nil, "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open scheduled log: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: level}))
	for _, cleanupErr := range cleanupErrors {
		logger.Warn("failed to remove expired log", "error", cleanupErr)
	}
	return logger, file, path, nil
}

func removeExpired(dir string, now time.Time) ([]error, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read log directory: %w", err)
	}
	cutoff := now.Add(-logRetention)
	var cleanupErrors []error
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(name, "scheduled-") || !(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.1")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect %s: %w", name, err))
			continue
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", name, err))
		}
	}
	return cleanupErrors, nil
}

func rotate(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect scheduled log: %w", err)
	}
	if info.Size() <= maxLogSize {
		return nil
	}
	backup := path + ".1"
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous scheduled log backup: %w", err)
	}
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("rotate scheduled log: %w", err)
	}
	return nil
}

func ParseLevel(value string) slog.Level {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
