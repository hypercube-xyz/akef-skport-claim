package notify

import (
	"testing"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func TestNotificationDetailFallbacks(t *testing.T) {
	if got := formatNotification(result.Run{}); got != "No account results" {
		t.Fatalf("empty notification=%q", got)
	}
	count := uint64(2)
	tests := []struct {
		account result.Account
		want    string
	}{
		{result.Account{Outcome: result.Claimed, Rewards: []result.Reward{{Name: "Item", Count: &count}}}, "Item x2"},
		{result.Account{Outcome: result.Claimed}, "claimed"},
		{result.Account{Outcome: result.AlreadyClaimed}, "already claimed"},
		{result.Account{Outcome: result.Unavailable}, "not available"},
		{result.Account{Outcome: result.AuthExpired}, "login required"},
		{result.Account{Outcome: result.TransientError}, "temporary failure"},
		{result.Account{Outcome: result.ClaimError}, "claim failed"},
		{result.Account{Outcome: result.AmbiguousClaim}, "claim status unknown"},
		{result.Account{Outcome: result.InternalError}, "internal failure"},
		{result.Account{Outcome: result.Skipped}, "available"},
		{result.Account{Outcome: result.Outcome("custom")}, "custom"},
		{result.Account{Outcome: result.Claimed, Summary: "provided"}, "provided"},
	}
	for _, test := range tests {
		if got := notificationAccountDetail(test.account); got != test.want {
			t.Fatalf("detail(%s)=%q want %q", test.account.Outcome, got, test.want)
		}
	}
}
