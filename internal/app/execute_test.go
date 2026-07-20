package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestExecutionSetupAndDelayFailures(t *testing.T) {
	isolateUserDirs(t)
	missing := filepath.Join(t.TempDir(), "missing.toml")
	if _, code, err := Execute(context.Background(), Options{ConfigPath: missing}); err == nil || code != report.ExitConfig {
		t.Fatalf("missing config code=%d err=%v", code, err)
	}
	path := writeAppConfig(t, "0s")
	if _, code, err := Execute(context.Background(), Options{ConfigPath: path, AccountName: "missing"}); err == nil || code != report.ExitConfig {
		t.Fatalf("missing account code=%d err=%v", code, err)
	}
	delayed := strings.Replace(string(mustReadFile(t, path)), `random_delay = "0s"`, `random_delay = "1s"`, 1)
	delayedPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(delayedPath, []byte(delayed), 0o600); err != nil {
		t.Fatal(err)
	}
	slept := false
	_, code, err := Execute(context.Background(), Options{
		ConfigPath: delayedPath, Output: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep:  func(context.Context, time.Duration) error { slept = true; return context.Canceled },
	})
	if !slept || !errors.Is(err, context.Canceled) || code != report.ExitTransient {
		t.Fatalf("random delay slept=%v code=%d err=%v", slept, code, err)
	}
}

func TestExecutionRejectsUnusableCacheAndScheduledLogPaths(t *testing.T) {
	path := writeAppConfig(t, "0s")
	blocker := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", blocker)
	t.Setenv("LOCALAPPDATA", blocker)
	t.Setenv("HOME", blocker)
	if _, code, err := Execute(context.Background(), Options{ConfigPath: path}); err == nil || code != report.ExitInternal {
		t.Fatalf("unusable cache code=%d err=%v", code, err)
	}
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "logs"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if logger, closer, err := configureLogger(Options{Silent: true}, &config.Config{}, cache); err == nil || logger != nil || closer != nil {
		t.Fatalf("blocked scheduled log logger=%v closer=%v err=%v", logger, closer, err)
	}
}

func TestLoggerReportAndSleepHelpers(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{LogLevel: "debug"}}
	explicit := slog.New(slog.NewTextHandler(io.Discard, nil))
	logger, closer, err := configureLogger(Options{Logger: explicit}, cfg, t.TempDir())
	if err != nil || logger != explicit || closer != nil {
		t.Fatalf("explicit logger=%v closer=%v err=%v", logger, closer, err)
	}
	logger, closer, err = configureLogger(Options{}, cfg, t.TempDir())
	if err != nil || logger == nil || closer != nil {
		t.Fatalf("interactive logger=%v closer=%v err=%v", logger, closer, err)
	}
	logger, closer, err = configureLogger(Options{Silent: true}, cfg, t.TempDir())
	if err != nil || logger == nil || closer == nil {
		t.Fatalf("scheduled logger=%v closer=%v err=%v", logger, closer, err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	run := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.Claimed}}}
	if err := writeReport(Options{Output: &output}, explicit, run); err != nil || !strings.Contains(output.String(), "main") {
		t.Fatalf("report output=%q err=%v", output.String(), err)
	}
	if err := writeReport(Options{Silent: true}, explicit, run); err != nil {
		t.Fatalf("silent report=%v", err)
	}
	if got := randomDelay(0); got != 0 {
		t.Fatalf("randomDelay(0)=%s", got)
	}
	if err := sleepContext(context.Background(), time.Nanosecond); err != nil {
		t.Fatalf("completed sleep=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sleep=%v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test-controlled path.
	if err != nil {
		t.Fatal(err)
	}
	return data
}
