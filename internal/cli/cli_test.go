package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/spf13/cobra"
)

func TestSilentConfigurationErrorIsLoggedAndReturnsConfigExitCode(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("HOME", base)
	missing := filepath.Join(t.TempDir(), "missing.toml")
	if code := Execute(context.Background(), []string{"--silent", "run", "--config", missing}); code != 10 {
		t.Fatalf("silent configuration error returned %d", code)
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(cacheDir, "logs", "scheduled-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one scheduled log, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "silent command failed") || strings.Contains(string(data), "credential-secret") {
		t.Fatalf("unexpected silent log: %s", data)
	}
}

func TestHasSilentRecognizesBooleanSpellings(t *testing.T) {
	for _, args := range [][]string{{"--silent"}, {"run", "--silent=true"}, {"--silent=TRUE", "run"}, {"--silent=1"}} {
		if !hasSilent(args) {
			t.Fatalf("silent flag was missed: %v", args)
		}
	}
	for _, args := range [][]string{{"--silent=false"}, {"--silent=0"}, {"run"}} {
		if hasSilent(args) {
			t.Fatalf("false silent flag was treated as enabled: %v", args)
		}
	}
}

func TestRunAndStatusRejectPositionalArgumentsBeforeExecution(t *testing.T) {
	options := &rootOptions{}
	for _, command := range []*cobra.Command{runCommand(options), statusCommand(options)} {
		if err := command.Args(command, []string{"unexpected"}); err == nil {
			t.Fatalf("%s accepted an ignored positional argument", command.Name())
		}
	}
}

func TestRootDoesNotExposeInternalCommands(t *testing.T) {
	root := newRoot(&rootOptions{})
	for _, command := range root.Commands() {
		switch command.Name() {
		case "completion", "schedule":
			t.Fatalf("unexpected command %q", command.Name())
		}
	}
}

func TestErrorCodeSeparatesUsageConfigurationAndInternalFailures(t *testing.T) {
	if got := errorCode(errors.New("unexpected filesystem failure")); got != report.ExitInternal {
		t.Fatalf("raw internal failure returned %d", got)
	}
	if got := errorCode(errors.New(`unknown command "wat" for "akef-claim"`)); got != report.ExitConfig {
		t.Fatalf("usage failure returned %d", got)
	}
	if got := errorCode(withExitCode(report.ExitConfig, errors.New("invalid config"))); got != report.ExitConfig {
		t.Fatalf("classified configuration failure returned %d", got)
	}
}
