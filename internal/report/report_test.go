package report

import (
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/model"
)

func TestExitPriority(t *testing.T) {
	reportValue := model.RunReport{Results: []model.AccountResult{{Outcome: model.AuthExpired}, {Outcome: model.ClaimError}, {Outcome: model.AmbiguousClaim}}}
	if got := ExitCode(reportValue); got != ExitAmbiguous {
		t.Fatalf("got %d", got)
	}
}

func TestFormat(t *testing.T) {
	value := model.RunReport{Duration: 4800 * time.Millisecond, Results: []model.AccountResult{{Account: "main", Outcome: model.Claimed, Summary: "Oroberyl x80"}}}
	text := Format(value)
	if !strings.Contains(text, "AKEF daily run completed") || !strings.Contains(text, "main") || !strings.Contains(text, "4.8s") {
		t.Fatalf("unexpected report: %s", text)
	}
}
