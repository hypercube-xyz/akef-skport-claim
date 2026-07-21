package cli

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
)

func TestExitError(t *testing.T) {
	e := &exitError{code: report.ExitAuth, err: &testError{msg: "auth failed"}}
	if e.Error() != "auth failed" {
		t.Errorf("Error() = %q; want 'auth failed'", e.Error())
	}
	if e.Unwrap() == nil {
		t.Error("Unwrap() should return wrapped error")
	}
}

func TestExitError_NilError(t *testing.T) {
	e := &exitError{code: report.ExitOK}
	got := e.Error()
	if got == "" {
		t.Error("Error() should not be empty for nil wrapped error")
	}
}

func TestWithExitCode(t *testing.T) {
	err := withExitCode(report.ExitConfig, &testError{msg: "bad"})
	if err == nil {
		t.Fatal("withExitCode() should return error")
	}
	if err.(*exitError).code != report.ExitConfig {
		t.Errorf("withExitCode() code = %d; want %d", err.(*exitError).code, report.ExitConfig)
	}

	// Already an exitError → pass through.
	existing := &exitError{code: report.ExitAuth, err: &testError{msg: "auth"}}
	got := withExitCode(report.ExitConfig, existing)
	if got != existing {
		t.Error("withExitCode() should pass through existing exitError")
	}

	// nil error → nil.
	if got := withExitCode(report.ExitConfig, nil); got != nil {
		t.Error("withExitCode(nil) should return nil")
	}
}