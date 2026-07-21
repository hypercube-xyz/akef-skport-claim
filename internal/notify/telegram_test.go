package notify

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestNewTelegramPayload(t *testing.T) {
	run := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"},
	}}
	payload := newTelegramPayload("12345", run)
	if payload.ChatID != "12345" {
		t.Errorf("ChatID = %q; want '12345'", payload.ChatID)
	}
	if payload.Text == "" {
		t.Error("Text should not be empty")
	}
}