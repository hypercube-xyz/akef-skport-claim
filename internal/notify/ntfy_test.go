package notify

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestNewNtfyPresentation(t *testing.T) {
	run := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"},
	}}
	p := newNtfyPresentation(run)
	if p.Priority != "default" {
		t.Errorf("Priority = %q; want 'default'", p.Priority)
	}
	if p.Title == "" {
		t.Error("Title should not be empty")
	}

	// Error run → high priority.
	run2 := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.AuthExpired},
	}}
	p2 := newNtfyPresentation(run2)
	if p2.Priority != "high" {
		t.Errorf("Priority = %q; want 'high'", p2.Priority)
	}
}