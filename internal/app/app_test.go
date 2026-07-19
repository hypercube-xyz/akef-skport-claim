package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	processlock "github.com/hypercube-xyz/akef-skport-claim/internal/lock"
	"github.com/hypercube-xyz/akef-skport-claim/internal/model"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

type fakeClient struct {
	status     skport.AttendanceResponse
	claim      skport.ClaimResponse
	refreshErr error
	statusErr  error
	claimErr   error
	claims     int
}

type fakeNotifier struct{ calls int }

type lockCheckingNotifier struct {
	t       *testing.T
	path    string
	checked bool
}

func (n *lockCheckingNotifier) SendAll(ctx context.Context, _ *config.Config, _ model.RunReport, _ *state.File) []error {
	n.t.Helper()
	acquired, ok, err := processlock.Try(ctx, n.path)
	if err != nil || !ok {
		n.t.Fatalf("claim lock was still held during notifications: acquired=%v err=%v", ok, err)
	}
	if err := acquired.Close(); err != nil {
		n.t.Fatalf("release notification test lock: %v", err)
	}
	n.checked = true
	return nil
}

func (f *fakeNotifier) SendAll(context.Context, *config.Config, model.RunReport, *state.File) []error {
	f.calls++
	return []error{errors.New("notification failed")}
}

func (f *fakeClient) Refresh(context.Context) (string, error) { return "token", f.refreshErr }
func (f *fakeClient) Status(context.Context, string) (skport.AttendanceResponse, error) {
	return f.status, f.statusErr
}
func (f *fakeClient) ClaimOnce(context.Context, string) (skport.ClaimResponse, error) {
	f.claims++
	return f.claim, f.claimErr
}

func TestGuardedFlowClaimsAtMostOnce(t *testing.T) {
	client := &fakeClient{status: attendance(`{"available":true}`), claim: claim(`{"awardIds":[]}`)}
	result := executeAccount(context.Background(), client, config.Account{Name: "main"}, false)
	if result.Outcome != model.Claimed || client.claims != 1 {
		t.Fatalf("unexpected result: %#v, claims=%d", result, client.claims)
	}
}

func TestStatusNeverClaimsAndDoneSkips(t *testing.T) {
	for _, statusOnly := range []bool{true, false} {
		client := &fakeClient{status: attendance(`{"available":false,"done":true}`)}
		result := executeAccount(context.Background(), client, config.Account{Name: "main"}, statusOnly)
		if result.Outcome != model.AlreadyClaimed || client.claims != 0 {
			t.Fatalf("unexpected result: %#v", result)
		}
	}
}

func TestConflictingAttendanceFlagsNeverClaim(t *testing.T) {
	client := &fakeClient{status: attendance(`{"calendar":[{"available":true,"done":true}],"hasToday":false}`)}
	result := executeAccount(context.Background(), client, config.Account{Name: "main"}, false)
	if result.Outcome != model.AlreadyClaimed || client.claims != 0 {
		t.Fatalf("conflicting flags must fail closed: result=%#v claims=%d", result, client.claims)
	}
	if result.Summary != "conflicting attendance flags; treated as already claimed" {
		t.Fatalf("unexpected conflict summary: %q", result.Summary)
	}
}

func TestAmbiguousClaimIsPreserved(t *testing.T) {
	client := &fakeClient{status: attendance(`{"available":true}`), claimErr: &skport.Error{Kind: skport.ErrorAmbiguous, Op: "claim", Err: errors.New("timeout")}}
	result := executeAccount(context.Background(), client, config.Account{Name: "main"}, false)
	if result.Outcome != model.AmbiguousClaim || client.claims != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func attendance(data string) skport.AttendanceResponse {
	return skport.AttendanceResponse{Code: 0, Data: []byte(data)}
}

func claim(data string) skport.ClaimResponse {
	return skport.ClaimResponse{Code: 0, Data: []byte(data)}
}

func TestExecuteContinuesAfterAccountFailure(t *testing.T) {
	path := writeAppConfig(t, "0s")
	isolateUserDirs(t)
	clients := map[string]*fakeClient{
		"first":  {refreshErr: &skport.Error{Kind: skport.ErrorTransient, Op: "refresh", Err: errors.New("down")}},
		"second": {status: attendance(`{"available":false,"done":true}`)},
	}
	runReport, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientFactory: func(account config.Account, _ time.Duration) SKPortClient {
			return clients[account.Name]
		},
	})
	if err != nil || code != 30 || len(runReport.Results) != 2 {
		t.Fatalf("execute result: code=%d err=%v report=%#v", code, err, runReport)
	}
	if runReport.Results[0].Outcome != model.TransientError || runReport.Results[1].Outcome != model.AlreadyClaimed {
		t.Fatalf("unexpected outcomes: %#v", runReport.Results)
	}
}

func TestExecuteReportsInterruptedAccountDelay(t *testing.T) {
	path := writeAppConfig(t, "1s")
	isolateUserDirs(t)
	client := &fakeClient{status: attendance(`{"available":false,"done":true}`)}
	runReport, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Sleep:      func(context.Context, time.Duration) error { return context.Canceled },
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			return client
		},
	})
	if !errors.Is(err, context.Canceled) || code != 30 || len(runReport.Results) != 1 {
		t.Fatalf("interrupted delay: code=%d err=%v report=%#v", code, err, runReport)
	}
}

func TestRandomDelayHandlesMaximumDuration(t *testing.T) {
	maximum := time.Duration(1<<63 - 1)
	if got := randomDelay(maximum); got < 0 || got >= maximum {
		t.Fatalf("random delay out of range: %s", got)
	}
}

func TestNotificationFailureNeverRepeatsClaim(t *testing.T) {
	path := writeAppConfig(t, "0s")
	isolateUserDirs(t)
	client := &fakeClient{status: attendance(`{"available":true}`), claim: claim(`{"awardIds":[]}`)}
	notifier := &fakeNotifier{}
	runReport, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		Account:    "first",
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Notifier:   notifier,
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			return client
		},
	})
	if err != nil || code != 0 || client.claims != 1 || notifier.calls != 1 || runReport.Results[0].Outcome != model.Claimed {
		t.Fatalf("code=%d err=%v claims=%d notifications=%d report=%#v", code, err, client.claims, notifier.calls, runReport)
	}
}

func TestClaimLockIsReleasedBeforeNotifications(t *testing.T) {
	isolateUserDirs(t)
	path := writeAppConfig(t, "0s")
	cacheDir, err := config.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	notifier := &lockCheckingNotifier{t: t, path: filepath.Join(cacheDir, "run.lock")}
	client := &fakeClient{status: attendance(`{"available":false,"done":true}`)}

	_, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		Account:    "first",
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Notifier:   notifier,
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			return client
		},
	})
	if err != nil || code != 0 || !notifier.checked {
		t.Fatalf("lock release check failed: code=%d err=%v checked=%v", code, err, notifier.checked)
	}
}

func TestUnknownAttendanceStateDoesNotLookSuccessful(t *testing.T) {
	client := &fakeClient{status: attendance(`{"unexpected":true}`)}
	result := executeAccount(context.Background(), client, config.Account{Name: "main"}, false)
	if result.Outcome != model.InternalError || client.claims != 0 {
		t.Fatalf("unexpected result: %#v, claims=%d", result, client.claims)
	}
}

func TestStatusDoesNotWaitForClaimLock(t *testing.T) {
	isolateUserDirs(t)
	path := writeAppConfig(t, "0s")
	held := acquireRunLock(t)
	defer held.Close()

	client := &fakeClient{status: attendance(`{"available":false,"done":true}`)}
	runReport, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		StatusOnly: true,
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			return client
		},
	})
	if err != nil || code != 0 || len(runReport.Results) != 2 {
		t.Fatalf("status was blocked by the claim lock: code=%d err=%v report=%#v", code, err, runReport)
	}
}

func TestRunWaitsForClaimLockThenRechecksStatus(t *testing.T) {
	isolateUserDirs(t)
	path := writeAppConfig(t, "0s")
	held := acquireRunLock(t)
	released := make(chan struct{})
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = held.Close()
		close(released)
	}()

	client := &fakeClient{status: attendance(`{"available":false,"done":true}`)}
	runReport, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		LockWait:   time.Second,
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			return client
		},
	})
	<-released
	if err != nil || code != 0 || len(runReport.Results) != 2 {
		t.Fatalf("run did not wait and recheck: code=%d err=%v report=%#v", code, err, runReport)
	}
}

func TestRunLockTimeoutIsTransientFailure(t *testing.T) {
	isolateUserDirs(t)
	path := writeAppConfig(t, "0s")
	held := acquireRunLock(t)
	defer held.Close()

	_, code, err := Execute(context.Background(), Options{
		ConfigPath: path,
		LockWait:   30 * time.Millisecond,
		Output:     io.Discard,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientFactory: func(config.Account, time.Duration) SKPortClient {
			t.Fatal("client must not be created before the run lock is acquired")
			return nil
		},
	})
	if code != 30 || !errors.Is(err, processlock.ErrWaitTimeout) {
		t.Fatalf("lock timeout must be retryable: code=%d err=%v", code, err)
	}
}

func acquireRunLock(t *testing.T) *processlock.Lock {
	t.Helper()
	cacheDir, err := config.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	held, acquired, err := processlock.Try(context.Background(), filepath.Join(cacheDir, "run.lock"))
	if err != nil || !acquired {
		t.Fatalf("acquire test lock: acquired=%v err=%v", acquired, err)
	}
	return held
}

func writeAppConfig(t *testing.T, accountDelay string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	text := `version = 1
[run]
random_delay = "0s"
account_delay = "` + accountDelay + `"
[[accounts]]
name = "first"
enabled = true
cred = "example-credential-first"
game_role = "example-role-first"
[[accounts]]
name = "second"
enabled = true
cred = "example-credential-second"
game_role = "example-role-second"
`
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func isolateUserDirs(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("HOME", base)
}
