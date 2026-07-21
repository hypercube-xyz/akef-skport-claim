package notify

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestNewDiscordPayload(t *testing.T) {
	run := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"},
	}}
	payload := newDiscordPayload(run)
	if payload.Username == "" {
		t.Error("Username should not be empty")
	}
	if payload.Content == "" {
		t.Error("Content should not be empty")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Error("AllowedMentions.Parse should be empty (no @mentions)")
	}
}