package report

import (
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestOutcomeExitCode(t *testing.T) {
	tests := []struct {
		outcome result.Outcome
		want    int
	}{
		{result.Claimed, ExitOK},
		{result.AlreadyClaimed, ExitOK},
		{result.Unavailable, ExitOK},
		{result.Skipped, ExitOK},
		{result.AuthExpired, ExitAuth},
		{result.TransientError, ExitTransient},
		{result.ClaimError, ExitClaim},
		{result.AmbiguousClaim, ExitAmbiguous},
		{result.InternalError, ExitInternal},
	}
	for _, tt := range tests {
		if got := OutcomeExitCode(tt.outcome); got != tt.want {
			t.Errorf("OutcomeExitCode(%q) = %d; want %d", tt.outcome, got, tt.want)
		}
	}
}

func TestExitCode(t *testing.T) {
	// Highest severity wins.
	run := result.Run{Accounts: []result.Account{
		{Outcome: result.Claimed},
		{Outcome: result.AuthExpired},
		{Outcome: result.TransientError},
	}}
	if got := ExitCode(run); got != ExitTransient {
		t.Errorf("ExitCode() = %d; want %d (TransientError > AuthExpired)", got, ExitTransient)
	}

	// Internal beats everything.
	run2 := result.Run{Accounts: []result.Account{
		{Outcome: result.Claimed},
		{Outcome: result.InternalError},
	}}
	if got := ExitCode(run2); got != ExitInternal {
		t.Errorf("ExitCode() = %d; want %d", got, ExitInternal)
	}

	// All good = OK.
	run3 := result.Run{Accounts: []result.Account{
		{Outcome: result.Claimed},
		{Outcome: result.AlreadyClaimed},
	}}
	if got := ExitCode(run3); got != ExitOK {
		t.Errorf("ExitCode() = %d; want %d", got, ExitOK)
	}
}

func TestSeverity(t *testing.T) {
	order := []int{ExitOK, ExitConfig, ExitAuth, ExitTransient, ExitClaim, ExitAmbiguous, ExitInternal}
	for i := 1; i < len(order); i++ {
		if severity(order[i]) <= severity(order[i-1]) {
			t.Errorf("severity(%d) <= severity(%d); want monotonic increase", order[i], order[i-1])
		}
	}
}

func TestFormat(t *testing.T) {
	run := result.Run{
		Duration: 5*time.Second + 200*time.Millisecond,
		Accounts: []result.Account{
			{Name: "main", Outcome: result.Claimed, Summary: "Orundum x200"},
			{Name: "alt", Outcome: result.AlreadyClaimed, Summary: "already claimed"},
		},
	}
	got := Format(run)
	if !strings.Contains(got, "main") || !strings.Contains(got, "claimed") || !strings.Contains(got, "Orundum x200") {
		t.Errorf("Format() missing expected content:\n%s", got)
	}
	if !strings.Contains(got, "Duration:") {
		t.Error("Format() missing Duration")
	}
	// No errors -> no "with errors" suffix.
	if strings.Contains(got, "with errors") {
		t.Error("Format() should not include 'with errors' for all-OK run")
	}
}

func TestFormat_WithErrors(t *testing.T) {
	run := result.Run{
		Duration: time.Second,
		Accounts: []result.Account{
			{Name: "main", Outcome: result.AuthExpired, Summary: "login required"},
		},
	}
	got := Format(run)
	if !strings.Contains(got, "with errors") {
		t.Error("Format() should include 'with errors' when any account has errors")
	}
}

func TestRewardSummary(t *testing.T) {
	if got := rewardSummary(nil); got != "no reward details" {
		t.Errorf("rewardSummary(nil) = %q; want 'no reward details'", got)
	}
	count := uint64(100)
	rewards := []result.Reward{{Name: "LMD", Count: &count}, {Name: "Gold"}}
	if got := rewardSummary(rewards); got != "LMD x100, Gold" {
		t.Errorf("rewardSummary() = %q; want 'LMD x100, Gold'", got)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(500 * time.Millisecond); got != "500ms" {
		t.Errorf("formatDuration(500ms) = %q; want '500ms'", got)
	}
	if got := formatDuration(5200 * time.Millisecond); got != "5.2s" {
		t.Errorf("formatDuration(5.2s) = %q; want '5.2s'", got)
	}
}