package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

// stubNotifier records calls to SendAll.
type stubNotifier struct {
	calls int
}

func (s *stubNotifier) SendAll(_ context.Context, _ *config.Config, _ result.Run, _ *state.Store) []error {
	s.calls++
	return nil
}

func TestSendNotifications_WithSender(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	cfg := &config.Config{
		Notifications: config.Notifications{
			Aggregate: true,
			Targets: []config.NotificationTarget{
				{Name: "test", Type: "discord", Enabled: false, Events: []string{"claimed"}},
			},
		},
	}
	run := result.Run{
		Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"}},
	}
	sender := &stubNotifier{}

	sendNotifications(context.Background(), logger, dir, cfg, run, sender)

	if sender.calls != 1 {
		t.Errorf("SendAll called %d times; want 1", sender.calls)
	}
}

func TestSendNotifications_NilSender(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	cfg := &config.Config{
		Notifications: config.Notifications{
			Aggregate: true,
			Targets:   []config.NotificationTarget{},
		},
	}
	run := result.Run{
		Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"}},
	}

	// Nil sender: should not panic. Creates a real notify.New internally.
	// With no enabled targets, the lock will be acquired and released.
	sendNotifications(context.Background(), logger, dir, cfg, run, nil)
}

func TestSendNotifications_CorruptedState(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)
	cfg := &config.Config{
		Notifications: config.Notifications{Aggregate: true},
	}
	run := result.Run{
		Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "ok"}},
	}

	// Write corrupted state to exercise the error path.
	if err := os.WriteFile(dir+"/state.json", []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	sendNotifications(context.Background(), logger, dir, cfg, run, nil)
	// ponytail: should not panic. State loading errors are logged, not fatal.
}