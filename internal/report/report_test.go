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
	if !strings.Contains(text, "Arknights: Endfield SKPORT daily claim completed") || !strings.Contains(text, "main") || !strings.Contains(text, "4.8s") {
		t.Fatalf("unexpected report: %s", text)
	}
}

func TestOutcomeCodesAndSeverity(t *testing.T) {
	tests := []struct {
		outcome result.Outcome
		code    int
	}{
		{result.AuthExpired, ExitAuth}, {result.TransientError, ExitTransient},
		{result.ClaimError, ExitClaim}, {result.AmbiguousClaim, ExitAmbiguous},
		{result.InternalError, ExitInternal}, {result.Claimed, ExitOK},
	}
	for _, test := range tests {
		if got := OutcomeExitCode(test.outcome); got != test.code {
			t.Fatalf("OutcomeExitCode(%q)=%d want %d", test.outcome, got, test.code)
		}
	}
	for code, want := range map[int]int{ExitInternal: 6, ExitAmbiguous: 5, ExitClaim: 4, ExitTransient: 3, ExitAuth: 2, ExitConfig: 1, 999: 0} {
		if got := severity(code); got != want {
			t.Fatalf("severity(%d)=%d want %d", code, got, want)
		}
	}
}

func TestFormatFailuresRewardsAndDurationBoundaries(t *testing.T) {
	count := uint64(80)
	run := result.Run{Duration: 999500 * time.Microsecond, Accounts: []result.Account{
		{Name: "failed", Outcome: result.InternalError},
		{Name: "reward", Outcome: result.Claimed, Rewards: []result.Reward{{Name: "Oroberyl", Count: &count}}},
	}}
	text := Format(run)
	for _, want := range []string{"completed with errors", "no reward details", "Oroberyl x80", "1s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
	if got := formatDuration(1250 * time.Millisecond); got != "1.3s" {
		t.Fatalf("formatDuration()=%q", got)
	}
}
