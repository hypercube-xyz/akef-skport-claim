package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/notify"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

const notificationLockWait = 10 * time.Minute

func sendNotifications(ctx context.Context, logger *slog.Logger, cacheDir string, cfg *config.Config, run result.Run, sender Notifier) {
	// Deduplication is a read-modify-write transaction. A separate lock keeps
	// notification latency from blocking attendance checks and claims.
	stateLock, err := lock.Wait(ctx, filepath.Join(cacheDir, "notification.lock"), notificationLockWait, lockPollInterval)
	if err != nil {
		logger.Warn("failed to acquire notification state lock", "error", err)
		return
	}
	defer func() {
		if err := stateLock.Close(); err != nil {
			logger.Warn("failed to release notification state lock", "error", err)
		}
	}()

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
