package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestNotificationStateRecoversFromInvalidJSONAndCancellation(t *testing.T) {
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "state.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := &config.Config{}
	sendNotifications(context.Background(), logger, cache, cfg, result.Run{}, nil)
	if !strings.Contains(logs.String(), "failed to load notification state") {
		t.Fatalf("state recovery was not logged: %s", logs.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logs.Reset()
	sendNotifications(ctx, logger, cache, cfg, result.Run{}, nil)
	if !strings.Contains(logs.String(), "failed to acquire notification state lock") {
		t.Fatalf("lock cancellation was not logged: %s", logs.String())
	}
}

func TestNotificationStateSaveFailureIsContained(t *testing.T) {
	cache := t.TempDir()
	statePath := filepath.Join(cache, "state.json")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statePath, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	sendNotifications(context.Background(), logger, cache, &config.Config{}, result.Run{}, nil)
	if !strings.Contains(logs.String(), "failed to save notification state") {
		t.Fatalf("save failure was not logged: %s", logs.String())
	}
}
