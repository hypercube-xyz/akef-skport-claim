package cli

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
)

func TestIsUsageError(t *testing.T) {
	tests := []struct {
		err  string
		want bool
	}{
		{"unknown command ", true},
		{"unknown flag: --bad", true},
		{"flag needs an argument: --config", true},
		{"required flag --config", true},
		{"invalid argument ", true},
		{"some random error", false},
	}
	for _, tt := range tests {
		if got := isUsageError(&exitError{code: 1, err: &testError{msg: tt.err}}); got != tt.want {
			t.Errorf("isUsageError(%q) = %v; want %v", tt.err, got, tt.want)
		}
	}
}

func TestHasSilent(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"--silent"}, true},
		{[]string{"--silent=true"}, true},
		{[]string{"--silent=false"}, false},
		{[]string{"run"}, false},
		{[]string{"run", "--silent"}, true},
	}
	for _, tt := range tests {
		if got := hasSilent(tt.args); got != tt.want {
			t.Errorf("hasSilent(%v) = %v; want %v", tt.args, got, tt.want)
		}
	}
}

func TestErrorCode(t *testing.T) {
	// exitError with code.
	if got := errorCode(&exitError{code: report.ExitAuth}); got != report.ExitAuth {
		t.Errorf("errorCode(exitError{Auth}) = %d; want %d", got, report.ExitAuth)
	}
	// usage error (non-exitError with usage marker).
	if got := errorCode(&testError{msg: "unknown flag: --bad"}); got != report.ExitConfig {
		t.Errorf("errorCode(usage) = %d; want %d", got, report.ExitConfig)
	}
	// unknown error → internal.
	if got := errorCode(&testError{msg: "random"}); got != report.ExitInternal {
		t.Errorf("errorCode(random) = %d; want %d", got, report.ExitInternal)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }