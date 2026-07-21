package notify

import (
	"strings"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestFormatNotification(t *testing.T) {
	run := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"},
		{Name: "alt", Outcome: result.AlreadyClaimed, Summary: "already claimed"},
	}}
	got := formatNotification(run)
	if !strings.Contains(got, "[main]") || !strings.Contains(got, "[alt]") {
		t.Errorf("formatNotification() = %q; want account names", got)
	}
}

func TestFormatNotification_Empty(t *testing.T) {
	got := formatNotification(result.Run{})
	if got != "No account results" {
		t.Errorf("formatNotification(empty) = %q; want 'No account results'", got)
	}
}

func TestNotificationAccountMessage_Error(t *testing.T) {
	account := result.Account{Name: "main", Outcome: result.AuthExpired, Summary: "login required"}
	got := notificationAccountMessage(account)
	if !strings.HasPrefix(got, "Error ") {
		t.Errorf("notificationAccountMessage(auth_expired) = %q; want 'Error ...'", got)
	}
}

func TestNotificationAccountMessage_Claimed(t *testing.T) {
	count := uint64(200)
	account := result.Account{Name: "main", Outcome: result.Claimed, Rewards: []result.Reward{{Name: "Orundum", Count: &count}}}
	got := notificationAccountMessage(account)
	if got != "Orundum x200" {
		t.Errorf("notificationAccountMessage(claimed) = %q; want 'Orundum x200'", got)
	}
}

func TestNotificationAccountDetail_Fallbacks(t *testing.T) {
	tests := []struct {
		outcome result.Outcome
		want    string
	}{
		{result.Claimed, "claimed"},
		{result.AlreadyClaimed, "already claimed"},
		{result.Unavailable, "not available"},
		{result.AuthExpired, "login required"},
		{result.TransientError, "temporary failure"},
		{result.ClaimError, "claim failed"},
		{result.AmbiguousClaim, "claim status unknown"},
		{result.InternalError, "internal failure"},
		{result.Skipped, "available"},
	}
	for _, tt := range tests {
		account := result.Account{Name: "test", Outcome: tt.outcome}
		got := notificationAccountDetail(account)
		if got != tt.want {
			t.Errorf("notificationAccountDetail(%q) = %q; want %q", tt.outcome, got, tt.want)
		}
	}
}