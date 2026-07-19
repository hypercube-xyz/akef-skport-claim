package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/notify"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

func sendNotifications(ctx context.Context, logger *slog.Logger, cacheDir string, cfg *config.Config, run result.Run, sender Notifier) {
	statePath := filepath.Join(cacheDir, "state.json")
	store, err := state.Load(statePath)
	if err != nil {
		logger.Warn("failed to load notification state", "error", err)
		store = &state.Store{Notifications: map[string]time.Time{}}
	}
	if sender == nil {
		sender = notify.New(notify.Options{})
	}
	for _, err := range sender.SendAll(ctx, cfg, run, store) {
		logger.Warn("notification failed", "error", err)
	}
	if err := store.Save(statePath); err != nil {
		logger.Warn("failed to save notification state", "error", err)
	}
}
