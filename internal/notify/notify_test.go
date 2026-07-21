package notify

import (
	"context"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

func TestEventFor(t *testing.T) {
	tests := []struct {
		outcome result.Outcome
		want    string
	}{
		{result.Claimed, "claimed"},
		{result.AlreadyClaimed, "already_claimed"},
		{result.Unavailable, "unavailable"},
		{result.AuthExpired, "auth_expired"},
		{result.TransientError, "error"},
		{result.ClaimError, "error"},
		{result.AmbiguousClaim, "error"},
		{result.InternalError, "error"},
		{result.Skipped, "skipped"},
	}
	for _, tt := range tests {
		if got := eventFor(tt.outcome); got != tt.want {
			t.Errorf("eventFor(%q) = %q; want %q", tt.outcome, got, tt.want)
		}
	}
}

func TestSelectEvents(t *testing.T) {
	events := []string{"claimed", "error"}
	results := []result.Account{
		{Name: "main", Outcome: result.Claimed},
		{Name: "alt", Outcome: result.TransientError},
	}
	matched, hasNonDedup, keys := selectEvents("test-target", events, results)
	if !matched {
		t.Error("selectEvents() matched = false; want true")
	}
	if !hasNonDedup {
		t.Error("selectEvents() hasNonDedup = false; want true (claimed event)")
	}
	if len(keys) != 1 {
		t.Errorf("selectEvents() keys = %v; want 1 dedup key (error)", keys)
	}
}

func TestSelectEvents_NoMatch(t *testing.T) {
	events := []string{"claimed"}
	results := []result.Account{{Name: "main", Outcome: result.AlreadyClaimed}}
	matched, _, _ := selectEvents("test", events, results)
	if matched {
		t.Error("selectEvents() matched = true; want false")
	}
}

func TestSelectEvents_AllDedup(t *testing.T) {
	events := []string{"auth_expired", "error"}
	results := []result.Account{
		{Name: "main", Outcome: result.AuthExpired},
		{Name: "alt", Outcome: result.TransientError},
	}
	matched, hasNonDedup, keys := selectEvents("target", events, results)
	if !matched {
		t.Error("selectEvents() matched = false; want true")
	}
	if hasNonDedup {
		t.Error("selectEvents() hasNonDedup = true; want false (all dedup)")
	}
	if len(keys) != 2 {
		t.Errorf("selectEvents() keys = %v; want 2 dedup keys", keys)
	}
}

func TestDedupKey(t *testing.T) {
	a := dedupKey("discord", "main", "auth_expired")
	b := dedupKey("discord", "main", "auth_expired")
	if a != b {
		t.Errorf("dedupKey() not deterministic: %q vs %q", a, b)
	}
	c := dedupKey("telegram", "main", "auth_expired")
	if a == c {
		t.Error("dedupKey() with different target should differ")
	}
}

// ---------------------------------------------------------------------------
// sendReport (non-HTTP paths)
// ---------------------------------------------------------------------------

func TestSendReport_NoTargets(t *testing.T) {
	sender := &Sender{now: time.Now}
	cfg := &config.Config{Notifications: config.Notifications{}}
	run := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.Claimed}}}
	store := &state.Store{Notifications: map[string]time.Time{}}

	errs := sender.sendReport(context.Background(), cfg, run, store)
	if len(errs) != 0 {
		t.Errorf("sendReport() errors = %v; want none", errs)
	}
}

func TestSendReport_DisabledTarget(t *testing.T) {
	sender := &Sender{now: time.Now}
	cfg := &config.Config{Notifications: config.Notifications{
		Targets: []config.NotificationTarget{
			{Name: "test", Type: "discord", Enabled: false, Events: []string{"claimed"}},
		},
	}}
	run := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.Claimed}}}
	store := &state.Store{Notifications: map[string]time.Time{}}

	errs := sender.sendReport(context.Background(), cfg, run, store)
	if len(errs) != 0 {
		t.Errorf("sendReport() errors = %v; want none (disabled)", errs)
	}
}

func TestSendReport_NoMatchingEvents(t *testing.T) {
	sender := &Sender{now: time.Now}
	cfg := &config.Config{Notifications: config.Notifications{
		Targets: []config.NotificationTarget{
			{Name: "test", Type: "discord", Enabled: true, Events: []string{"error"}, Webhook: config.NewSecret("https://discord.com/api/webhooks/123/abc")},
		},
	}}
	run := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.Claimed}}}
	store := &state.Store{Notifications: map[string]time.Time{}}

	errs := sender.sendReport(context.Background(), cfg, run, store)
	if len(errs) != 0 {
		t.Errorf("sendReport() errors = %v; want none (event mismatch)", errs)
	}
}