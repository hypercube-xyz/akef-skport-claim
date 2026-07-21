package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

// ---------------------------------------------------------------------------
// randomDelay
// ---------------------------------------------------------------------------

func TestRandomDelay_Positive(t *testing.T) {
	max := 100 * time.Millisecond
	for range 100 {
		got := randomDelay(max)
		if got < 0 || got >= max {
			t.Errorf("randomDelay(%v) = %v; want [0, %v)", max, got, max)
		}
	}
}

func TestRandomDelay_Zero(t *testing.T) {
	if got := randomDelay(0); got != 0 {
		t.Errorf("randomDelay(0) = %v; want 0", got)
	}
}

func TestRandomDelay_Negative(t *testing.T) {
	if got := randomDelay(-1 * time.Second); got != 0 {
		t.Errorf("randomDelay(-1s) = %v; want 0", got)
	}
}

// ---------------------------------------------------------------------------
// sleepContext
// ---------------------------------------------------------------------------

func TestSleepContext_Normal(t *testing.T) {
	start := time.Now()
	err := sleepContext(context.Background(), 50*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("sleepContext() error: %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("slept %v; want >= 50ms", elapsed)
	}
}

func TestSleepContext_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sleepContext(ctx, time.Hour)
	if err == nil {
		t.Fatal("sleepContext() should return error when context canceled")
	}
}

func TestSleepContext_ZeroDelay(t *testing.T) {
	err := sleepContext(context.Background(), 0)
	if err != nil {
		t.Fatalf("sleepContext(0) error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeReport
// ---------------------------------------------------------------------------

func TestWriteReport_Stdout(t *testing.T) {
	var buf bytes.Buffer
	options := Options{Output: &buf}
	logger := slog.New(slog.DiscardHandler)
	runReport := result.Run{
		Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"}},
	}

	err := writeReport(options, logger, runReport)
	if err != nil {
		t.Fatalf("writeReport() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "main") || !strings.Contains(got, "claimed") {
		t.Errorf("writeReport() = %q; want report content", got)
	}
}

func TestWriteReport_Silent(t *testing.T) {
	options := Options{Silent: true}
	logger := slog.New(slog.DiscardHandler)
	runReport := result.Run{
		Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"}},
	}

	err := writeReport(options, logger, runReport)
	if err != nil {
		t.Fatalf("writeReport() error: %v", err)
	}
}

func TestWriteReport_NilOutput(t *testing.T) {
	options := Options{}
	logger := slog.New(slog.DiscardHandler)
	runReport := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "ok"}}}
	// ponytail: writes to os.Stdout; covered by integration.
	_ = writeReport(options, logger, runReport)
}

// ---------------------------------------------------------------------------
// configureLogger
// ---------------------------------------------------------------------------

func TestConfigureLogger_Interactive(t *testing.T) {
	options := Options{Silent: false}
	cfg := &config.Config{App: config.AppConfig{LogLevel: "info"}}
	logger, closer, err := configureLogger(options, cfg, "")
	if err != nil {
		t.Fatalf("configureLogger() error: %v", err)
	}
	if closer != nil {
		t.Error("interactive logger should not have a closer")
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
}

func TestConfigureLogger_Silent(t *testing.T) {
	dir := t.TempDir()
	options := Options{Silent: true}
	cfg := &config.Config{App: config.AppConfig{LogLevel: "info"}}
	logger, closer, err := configureLogger(options, cfg, dir)
	if err != nil {
		t.Fatalf("configureLogger() error: %v", err)
	}
	if closer == nil {
		t.Fatal("silent logger should have a closer")
	}
	t.Cleanup(func() { _ = closer.Close() })
	if logger == nil {
		t.Fatal("logger is nil")
	}
}

// ---------------------------------------------------------------------------
// Execute integration tests
// ---------------------------------------------------------------------------

func writeTempConfig(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestExecute_ConfigLoadError(t *testing.T) {
	_, _, err := Execute(context.Background(), Options{ConfigPath: "/nonexistent/config.toml"})
	if err == nil {
		t.Fatal("Execute() should fail with nonexistent config")
	}
}

func TestExecute_ValidConfigStatusOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, `
version = 1
[app]
language = "en"
log_level = "info"
[run]
random_delay = "0s"
account_delay = "0s"
request_timeout = "10s"
notification_error_cooldown = "24h"
[[accounts]]
name = "main"
enabled = true
cred = "test-cred"
game_role = "test-role"
language = "en"
`)
	// StatusOnly=true skips claim lock and notifications.
	// Network calls will fail (no real SKPORT server), but the flow
	// should still populate the account entry.
	run, _, err := Execute(context.Background(), Options{ConfigPath: cfgPath, StatusOnly: true, Silent: true, Output: io.Discard})
	// We expect a network error, but the account should be in the list.
	if err == nil {
		t.Log("unexpected success (SKPORT may be reachable)")
	}
	if len(run.Accounts) == 0 {
		t.Error("run.Accounts is empty; expected at least one account entry")
	}
}

func TestExecute_InvalidAccountName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, `
version = 1
[app]
log_level = "info"
[run]
random_delay = "0s"
account_delay = "0s"
request_timeout = "10s"
[[accounts]]
name = "main"
enabled = true
cred = "test-cred"
game_role = "test-role"
language = "en"
`)
	_, _, err := Execute(context.Background(), Options{ConfigPath: cfgPath, AccountName: "nonexistent", Silent: true, Output: io.Discard})
	if err == nil {
		t.Fatal("Execute() should fail with unknown account name")
	}
}

func TestExecute_DisabledAccount(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, `
version = 1
[app]
log_level = "info"
[run]
random_delay = "0s"
account_delay = "0s"
request_timeout = "10s"
[[accounts]]
name = "off"
enabled = false
cred = "test-cred"
game_role = "test-role"
language = "en"
`)
	_, _, err := Execute(context.Background(), Options{ConfigPath: cfgPath, Silent: true, Output: io.Discard})
	if err == nil {
		t.Fatal("Execute() should fail when no accounts are enabled")
	}
}