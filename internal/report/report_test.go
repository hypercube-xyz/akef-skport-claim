package report

import (
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestExitPriority(t *testing.T) {
	reportValue := result.Run{Accounts: []result.Account{{Outcome: result.AuthExpired}, {Outcome: result.ClaimError}, {Outcome: result.AmbiguousClaim}}}
	if got := ExitCode(reportValue); got != ExitAmbiguous {
		t.Fatalf("got %d", got)
	}
}

func TestFormat(t *testing.T) {
	value := result.Run{Duration: 4800 * time.Millisecond, Accounts: []result.Account{{Name: "main", Outcome: result.Claimed, Summary: "Oroberyl x80"}}}
	text := Format(value)
	if !strings.Contains(text, "AKEF daily run completed") || !strings.Contains(text, "main") || !strings.Contains(text, "4.8s") {
		t.Fatalf("unexpected report: %s", text)
	}
}
