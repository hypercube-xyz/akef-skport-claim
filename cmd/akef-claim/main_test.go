package main

import (
	"os"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
)

func TestRunReturnsCLIExitCodes(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"akef-claim", "version"}
	if code := run(); code != report.ExitOK {
		t.Fatalf("version exit code=%d", code)
	}
	os.Args = []string{"akef-claim", "unknown"}
	if code := run(); code != report.ExitConfig {
		t.Fatalf("usage exit code=%d", code)
	}
}
