package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
)

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

type cliRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn cliRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type failAfterWriter struct {
	writes int
	failAt int
}

func (writer *failAfterWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.writes == writer.failAt {
		return 0, errors.New("output unavailable")
	}
	return len(data), nil
}

func writeCLIConfig(t *testing.T, targets string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	text := `version = 1
[run]
random_delay = "0s"
account_delay = "0s"
[[accounts]]
name = "first"
enabled = true
cred = "credential-example-first"
game_role = "role-example-first"
[[accounts]]
name = "second"
enabled = true
cred = "credential-example-second"
game_role = "role-example-second"
` + targets
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func executeRoot(t *testing.T, options *rootOptions, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	root := newRoot(options)
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return output.String(), err
}

func isolateCLIUserDirs(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("HOME", base)
}

func TestConfigCommandsFailureAndSuccess(t *testing.T) {
	isolateCLIUserDirs(t)
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	options := &rootOptions{}
	output, err := executeRoot(t, options, "--config", path, "config", "path")
	if err != nil || strings.TrimSpace(output) != path {
		t.Fatalf("config path output=%q err=%v", output, err)
	}
	output, err = executeRoot(t, &rootOptions{}, "--config", path, "config", "init")
	if err != nil || strings.TrimSpace(output) != path {
		t.Fatalf("config init output=%q err=%v", output, err)
	}
	if _, err = executeRoot(t, &rootOptions{}, "--config", path, "config", "init"); errorCode(err) != report.ExitConfig {
		t.Fatalf("second config init error=%v code=%d", err, errorCode(err))
	}
	if _, err = executeRoot(t, &rootOptions{}, "--config", path, "config", "init", "--force"); err != nil {
		t.Fatalf("forced config init: %v", err)
	}

	valid := writeCLIConfig(t, "")
	output, err = executeRoot(t, &rootOptions{}, "--config", valid, "config", "validate")
	if err != nil || !strings.Contains(output, "2 enabled accounts") {
		t.Fatalf("config validate output=%q err=%v", output, err)
	}
	if _, err = executeRoot(t, &rootOptions{}, "--config", filepath.Join(t.TempDir(), "missing"), "config", "validate"); errorCode(err) != report.ExitConfig {
		t.Fatalf("missing config validation error=%v", err)
	}
}

func TestDoctorReportsConfigFailureAndLocalSuccess(t *testing.T) {
	isolateCLIUserDirs(t)
	missing := filepath.Join(t.TempDir(), "missing.toml")
	output, err := executeRoot(t, &rootOptions{}, "--config", missing, "doctor")
	if errorCode(err) != report.ExitConfig || !strings.Contains(output, "FAIL config") {
		t.Fatalf("doctor missing output=%q err=%v", output, err)
	}

	path := writeCLIConfig(t, "")
	output, err = executeRoot(t, &rootOptions{}, "--config", path, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PASS config path", "PASS config: 2 enabled accounts", "PASS cache directory", "PASS process lock", "network activity skipped"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, output)
		}
	}
}

func TestNotifyCommandPrioritizesConfigurationAndTransportFailures(t *testing.T) {
	disabledTarget := `
[[notifications.targets]]
name = "phone"
type = "ntfy"
enabled = false
server = "https://ntfy.sh"
topic = "alerts"
events = ["error"]
`
	path := writeCLIConfig(t, disabledTarget)
	if _, err := executeRoot(t, &rootOptions{}, "--config", path, "notify", "test"); errorCode(err) != report.ExitConfig || !strings.Contains(err.Error(), "no enabled") {
		t.Fatalf("disabled notification error=%v", err)
	}
	if _, err := executeRoot(t, &rootOptions{}, "--config", path, "notify", "test", "missing"); errorCode(err) != report.ExitConfig || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing notification error=%v", err)
	}
	placeholderTarget := `
[[notifications.targets]]
name = "template"
type = "ntfy"
enabled = false
server = "https://ntfy.sh"
topic = "replace-me"
events = ["error"]
`
	path = writeCLIConfig(t, placeholderTarget)
	if _, err := executeRoot(t, &rootOptions{}, "--config", path, "notify", "test", "template"); errorCode(err) != report.ExitConfig || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("enabled template target error=%v", err)
	}

	unreachableTarget := `
[[notifications.targets]]
name = "phone"
type = "ntfy"
enabled = true
server = "https://127.0.0.1:1"
topic = "alerts"
events = ["error"]
`
	path = writeCLIConfig(t, unreachableTarget)
	if _, err := executeRoot(t, &rootOptions{}, "--config", path, "notify", "test"); errorCode(err) != report.ExitTransient {
		t.Fatalf("transport failure error=%v code=%d", err, errorCode(err))
	}
}

func TestRootFailuresVersionAndHelpers(t *testing.T) {
	output, err := executeRoot(t, &rootOptions{}, "version")
	if err != nil || !strings.Contains(output, "commit") {
		t.Fatalf("version output=%q err=%v", output, err)
	}
	if _, err := executeRoot(t, &rootOptions{}, "--silent", "version"); errorCode(err) != report.ExitConfig {
		t.Fatalf("silent version error=%v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing.toml")
	for _, command := range []string{"run", "status"} {
		if _, err := executeRoot(t, &rootOptions{}, "--config", missing, command); errorCode(err) != report.ExitConfig {
			t.Fatalf("%s missing config error=%v", command, err)
		}
	}
	if _, err := executeRoot(t, &rootOptions{}, "notify", "test", "one", "two"); errorCode(err) != report.ExitConfig {
		t.Fatalf("maximum args error=%v", err)
	}

	if err := writeOutput(errorWriter{}, "value"); errorCode(err) != report.ExitInternal {
		t.Fatalf("write failure=%v", err)
	}
	if got := (&exitError{code: 7}).Error(); !strings.Contains(got, "7") {
		t.Fatalf("nil wrapped error text=%q", got)
	}
	if err := withExitCode(report.ExitConfig, nil); err != nil {
		t.Fatalf("nil error was wrapped: %v", err)
	}
	typed := withExitCode(report.ExitConfig, errors.New("bad"))
	if got := withExitCode(report.ExitInternal, typed); errorCode(got) != report.ExitConfig {
		t.Fatal("typed error was wrapped twice")
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := errorCode(err); got != report.ExitTransient {
			t.Fatalf("errorCode(%v)=%d", err, got)
		}
	}
	if isUsageError(nil) || isUsageError(errors.New("ordinary")) {
		t.Fatal("ordinary errors classified as usage")
	}
	for _, message := range []string{"unknown flag: --bad", "flag needs an argument: --config", "required flag x", "invalid argument x"} {
		if !isUsageError(errors.New(message)) {
			t.Fatalf("usage marker missed: %s", message)
		}
	}
	cfg := &config.Config{Accounts: []config.Account{{Enabled: true}, {}, {Enabled: true}}}
	if got := countEnabled(cfg); got != 2 {
		t.Fatalf("countEnabled()=%d", got)
	}
}

func TestExecuteReturnsUsageAndSuccessCodes(t *testing.T) {
	if code := Execute(context.Background(), []string{"version"}); code != report.ExitOK {
		t.Fatalf("version code=%d", code)
	}
	if code := Execute(context.Background(), []string{"unknown"}); code != report.ExitConfig {
		t.Fatalf("unknown command code=%d", code)
	}
	if code := Execute(context.Background(), []string{"--unknown-flag"}); code != report.ExitConfig {
		t.Fatalf("unknown flag code=%d", code)
	}
}

func TestCommandSuccessPathsUseApplicationResults(t *testing.T) {
	isolateCLIUserDirs(t)
	path := writeCLIConfig(t, `
[[notifications.targets]]
name = "phone"
type = "ntfy"
enabled = true
server = "https://notify.example"
topic = "alerts"
events = ["already_claimed"]
`)
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = cliRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, `{"code":0,"data":{"available":false,"done":true}}`
		switch {
		case strings.Contains(request.URL.Path, "/auth/refresh"):
			body = `{"code":0,"data":{"token":"token"}}`
		case request.URL.Host == "notify.example":
			status, body = http.StatusNoContent, ""
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})

	for _, args := range [][]string{
		{"--config", path, "run", "--account", "first"},
		{"--config", path, "status", "--account", "first"},
		{"--config", path, "doctor", "--network"},
		{"--config", path, "notify", "test", "phone"},
	} {
		options := &rootOptions{}
		output, err := executeRoot(t, options, args...)
		if err != nil {
			t.Fatalf("%v failed: %v output=%s", args, err, output)
		}
	}
	writer := &failAfterWriter{failAt: 1}
	root := newRoot(&rootOptions{})
	root.SetOut(writer)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--config", path, "notify", "test", "phone"})
	if err := root.ExecuteContext(context.Background()); errorCode(err) != report.ExitInternal {
		t.Fatalf("notification output failure=%v", err)
	}
}

func TestDoctorPropagatesOutputAndCacheFailures(t *testing.T) {
	path := writeCLIConfig(t, "")
	for failAt := 1; failAt <= 5; failAt++ {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			isolateCLIUserDirs(t)
			writer := &failAfterWriter{failAt: failAt}
			root := newRoot(&rootOptions{})
			root.SetOut(writer)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"--config", path, "doctor"})
			if err := root.ExecuteContext(context.Background()); errorCode(err) != report.ExitInternal {
				t.Fatalf("write %d error=%v", failAt, err)
			}
		})
	}

	blocker := filepath.Join(t.TempDir(), "cache-blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", blocker)
	t.Setenv("LOCALAPPDATA", blocker)
	if _, err := executeRoot(t, &rootOptions{}, "--config", path, "doctor"); errorCode(err) != report.ExitInternal {
		t.Fatalf("cache failure=%v", err)
	}
}

func TestSilentErrorLoggingFailuresRemainContained(t *testing.T) {
	base := t.TempDir()
	logs := filepath.Join(base, config.AppDir, "logs")
	if err := os.MkdirAll(filepath.Dir(logs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logs, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)
	writeSilentError(errors.New("expected failure"))
}

func TestCommandOutputFailuresPreserveExitCodes(t *testing.T) {
	valid := writeCLIConfig(t, "")
	tests := []struct {
		name   string
		args   []string
		failAt int
		code   int
	}{
		{"config path", []string{"--config", valid, "config", "path"}, 1, report.ExitInternal},
		{"config validate", []string{"--config", valid, "config", "validate"}, 1, report.ExitInternal},
		{"doctor config failure", []string{"--config", filepath.Join(t.TempDir(), "missing"), "doctor"}, 2, report.ExitInternal},
		{"notify config failure", []string{"--config", filepath.Join(t.TempDir(), "missing"), "notify", "test"}, 1, report.ExitConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &failAfterWriter{failAt: test.failAt}
			root := newRoot(&rootOptions{})
			root.SetOut(writer)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)
			if err := root.ExecuteContext(context.Background()); errorCode(err) != test.code {
				t.Fatalf("error=%v code=%d", err, errorCode(err))
			}
		})
	}
}

func TestConfigPathReportsDefaultResolutionFailure(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "")
	if _, err := executeRoot(t, &rootOptions{}, "config", "path"); errorCode(err) != report.ExitConfig {
		t.Fatalf("default path resolution error=%v", err)
	}
}
