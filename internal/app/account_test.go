package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
)

func TestAccountRequestFailuresAreClassified(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want result.Outcome
	}{
		{"ordinary", errors.New("unexpected"), result.InternalError},
		{"auth", &skport.Error{Kind: skport.ErrorAuth, Err: errors.New("auth")}, result.AuthExpired},
		{"transient", &skport.Error{Kind: skport.ErrorTransient, Err: errors.New("network")}, result.TransientError},
		{"claim", &skport.Error{Kind: skport.ErrorClaim, Err: errors.New("claim")}, result.ClaimError},
		{"ambiguous", &skport.Error{Kind: skport.ErrorAmbiguous, Err: errors.New("timeout")}, result.AmbiguousClaim},
		{"unknown kind", &skport.Error{Kind: skport.ErrorKind("unknown"), Err: errors.New("unknown")}, result.InternalError},
	}
	for _, test := range tests {
		t.Run("refresh "+test.name, func(t *testing.T) {
			if got := executeAccount(context.Background(), &fakeClient{refreshErr: test.err}, config.Account{Name: "main"}, false); got.Outcome != test.want {
				t.Fatalf("outcome=%s want %s", got.Outcome, test.want)
			}
		})
		t.Run("status "+test.name, func(t *testing.T) {
			if got := executeAccount(context.Background(), &fakeClient{statusErr: test.err}, config.Account{Name: "main"}, false); got.Outcome != test.want {
				t.Fatalf("outcome=%s want %s", got.Outcome, test.want)
			}
		})
	}
}

func TestAccountStatusAndClaimResponses(t *testing.T) {
	tests := []struct {
		name       string
		status     skport.AttendanceResponse
		claim      skport.ClaimResponse
		statusOnly bool
		want       result.Outcome
	}{
		{"status auth code", skport.AttendanceResponse{Code: 401}, claim("{}"), false, result.AuthExpired},
		{"status api error", skport.AttendanceResponse{Code: 500}, claim("{}"), false, result.TransientError},
		{"invalid session", attendance(`{"hasToday":false}`), claim("{}"), false, result.AuthExpired},
		{"status available", attendance(`{"available":true}`), claim("{}"), true, result.Skipped},
		{"status unavailable", attendance(`{"hasToday":true,"available":false,"done":false}`), claim("{}"), true, result.Unavailable},
		{"run unavailable", attendance(`{"hasToday":true,"available":false,"done":false}`), claim("{}"), false, result.Unavailable},
		{"claim already done", attendance(`{"available":true}`), skport.ClaimResponse{Code: 10001}, false, result.AlreadyClaimed},
		{"claim auth", attendance(`{"available":true}`), skport.ClaimResponse{Code: 401}, false, result.AuthExpired},
		{"claim api error", attendance(`{"available":true}`), skport.ClaimResponse{Code: 999}, false, result.ClaimError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{status: test.status, claim: test.claim}
			if got := executeAccount(context.Background(), client, config.Account{Name: "main"}, test.statusOnly); got.Outcome != test.want {
				t.Fatalf("result=%#v", got)
			}
		})
	}
}

func TestClaimFailureAndRewardFallbacks(t *testing.T) {
	client := &fakeClient{status: attendance(`{"available":true}`), claimErr: &skport.Error{Kind: skport.ErrorClaim, Err: errors.New("rejected")}}
	if got := executeAccount(context.Background(), client, config.Account{Name: "main"}, false); got.Outcome != result.ClaimError {
		t.Fatalf("claim failure=%#v", got)
	}
	withRewards := &fakeClient{status: attendance(`{"available":true}`), claim: claim(`{"awardIds":["a","b"],"resourceInfoMap":{"a":{"name":"A","count":1},"b":{"name":"B","count":2}}}`)}
	got := executeAccount(context.Background(), withRewards, config.Account{Name: "main"}, false)
	if got.Outcome != result.Claimed || len(got.Rewards) != 2 || !strings.Contains(got.Summary, "A") || !strings.Contains(got.Summary, ", ") {
		t.Fatalf("claim rewards=%#v", got)
	}
	if got := rewardsSummary(nil); got != "rewards unknown" {
		t.Fatalf("empty reward summary=%q", got)
	}
}
