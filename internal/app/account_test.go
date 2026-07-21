package app

import (
	"context"
	"errors"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
)

type stubClient struct {
	refreshErr     error
	refreshToken   string
	statusErr      error
	statusResponse skport.AttendanceResponse
	claimErr       error
	claimResponse  skport.ClaimResponse
	claimCalled    bool
}

func (c *stubClient) Refresh(_ context.Context) (string, error) {
	return c.refreshToken, c.refreshErr
}

func (c *stubClient) Status(_ context.Context, _ string) (skport.AttendanceResponse, error) {
	return c.statusResponse, c.statusErr
}

func (c *stubClient) ClaimOnce(_ context.Context, _ string) (skport.ClaimResponse, error) {
	c.claimCalled = true
	return c.claimResponse, c.claimErr
}

func TestExecuteAccount_RefreshError(t *testing.T) {
	client := &stubClient{refreshErr: &skport.Error{Kind: skport.ErrorAuth, Op: "refresh", Err: errors.New("bad")}}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AuthExpired {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AuthExpired)
	}
}

func TestExecuteAccount_StatusError(t *testing.T) {
	client := &stubClient{
		refreshToken: "token",
		statusErr:    &skport.Error{Kind: skport.ErrorTransient, Op: "status", Err: errors.New("timeout")},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.TransientError {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.TransientError)
	}
}

func TestExecuteAccount_AuthCode(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 401},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AuthExpired {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AuthExpired)
	}
}

func TestExecuteAccount_StatusOnly_Available(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, true)
	if got.Outcome != result.Skipped {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.Skipped)
	}
}

func TestExecuteAccount_StatusOnly_AlreadyDone(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"isDone":true}`)},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, true)
	if got.Outcome != result.AlreadyClaimed {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AlreadyClaimed)
	}
}

func TestExecuteAccount_ClaimSuccess(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimResponse:  skport.ClaimResponse{Code: 0, Data: []byte(`{"awardIds":[],"resourceInfoMap":{}}`)},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.Claimed {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.Claimed)
	}
	if !client.claimCalled {
		t.Error("ClaimOnce() was not called")
	}
}

func TestExecuteAccount_NotAvailable(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"isDone":true}`)},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AlreadyClaimed {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AlreadyClaimed)
	}
	if client.claimCalled {
		t.Error("ClaimOnce() should not be called when not available")
	}
}

func TestExecuteAccount_ClaimAlreadyDone(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimResponse:  skport.ClaimResponse{Code: 10001},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AlreadyClaimed {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AlreadyClaimed)
	}
}

func TestExecuteAccount_ClaimAuthError(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimResponse:  skport.ClaimResponse{Code: 401},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AuthExpired {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AuthExpired)
	}
}

func TestExecuteAccount_ClaimAPIError(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimResponse:  skport.ClaimResponse{Code: 99999},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.ClaimError {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.ClaimError)
	}
}

func TestExecuteAccount_ClaimError(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimErr:       &skport.Error{Kind: skport.ErrorClaim, Op: "claim", Err: errors.New("claim failed")},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.ClaimError {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.ClaimError)
	}
}

func TestExecuteAccount_ClaimAmbiguous(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":true}`)},
		claimErr:       &skport.Error{Kind: skport.ErrorAmbiguous, Op: "claim", Err: errors.New("unknown")},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.AmbiguousClaim {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.AmbiguousClaim)
	}
}

func TestExecuteAccount_InternalError(t *testing.T) {
	client := &stubClient{
		refreshToken: "token",
		statusErr:    errors.New("unexpected internal error"),
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, false)
	if got.Outcome != result.InternalError {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.InternalError)
	}
}

func TestExecuteAccount_StatusOnly_Unavailable(t *testing.T) {
	client := &stubClient{
		refreshToken:   "token",
		statusResponse: skport.AttendanceResponse{Code: 0, Data: []byte(`{"available":false}`)},
	}
	account := config.Account{Name: "test"}
	got := executeAccount(context.Background(), client, account, true)
	if got.Outcome != result.Unavailable {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.Unavailable)
	}
}

func TestResultFromError(t *testing.T) {
	tests := []struct {
		kind    skport.ErrorKind
		outcome result.Outcome
	}{
		{skport.ErrorAuth, result.AuthExpired},
		{skport.ErrorTransient, result.TransientError},
		{skport.ErrorClaim, result.ClaimError},
		{skport.ErrorAmbiguous, result.AmbiguousClaim},
		{skport.ErrorInternal, result.InternalError},
	}
	for _, tt := range tests {
		r := result.Account{Name: "test"}
		got := resultFromError(r, &skport.Error{Kind: tt.kind})
		if got.Outcome != tt.outcome {
			t.Errorf("resultFromError(%q) = %q; want %q", tt.kind, got.Outcome, tt.outcome)
		}
	}
}

func TestResultFromError_NonSKPortError(t *testing.T) {
	r := result.Account{Name: "test"}
	got := resultFromError(r, errors.New("random"))
	if got.Outcome != result.InternalError {
		t.Errorf("Outcome = %q; want %q", got.Outcome, result.InternalError)
	}
}

func TestDoneSummary(t *testing.T) {
	if got := doneSummary(skport.AttendanceState{Conflict: true}); got == "" {
		t.Error("doneSummary(conflict) should not be empty")
	}
	if got := doneSummary(skport.AttendanceState{}); got != "already claimed" {
		t.Errorf("doneSummary(normal) = %q; want 'already claimed'", got)
	}
}

func TestRewardsSummary(t *testing.T) {
	if got := rewardsSummary(nil); got != "rewards unknown" {
		t.Errorf("rewardsSummary(nil) = %q; want 'rewards unknown'", got)
	}
	count := uint64(500)
	rewards := []result.Reward{{Name: "LMD", Count: &count}}
	if got := rewardsSummary(rewards); got != "LMD x500" {
		t.Errorf("rewardsSummary() = %q; want 'LMD x500'", got)
	}
}