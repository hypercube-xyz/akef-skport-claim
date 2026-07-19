package app

import (
	"context"
	"errors"
	"strings"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/skport"
)

func executeAccount(ctx context.Context, client SKPortClient, account config.Account, statusOnly bool) result.Account {
	accountResult := result.Account{Name: account.Name}
	token, err := client.Refresh(ctx)
	if err != nil {
		return resultFromError(accountResult, err)
	}
	status, err := client.Status(ctx, token)
	if err != nil {
		return resultFromError(accountResult, err)
	}
	if skport.IsAuthCode(status.Code) {
		accountResult.Outcome = result.AuthExpired
		accountResult.Summary = "status indicates login is required"
		return accountResult
	}
	if status.Code != 0 {
		accountResult.Outcome = result.TransientError
		accountResult.Summary = "status API returned an error"
		return accountResult
	}

	attendance := status.State()
	if !attendance.SessionValid {
		accountResult.Outcome = result.AuthExpired
		accountResult.Summary = "status indicates login is required"
		return accountResult
	}
	if !attendance.AvailableKnown && !attendance.DoneKnown {
		accountResult.Outcome = result.InternalError
		accountResult.Summary = "status response did not contain a recognizable attendance state"
		return accountResult
	}
	if statusOnly {
		switch {
		case attendance.Available:
			accountResult.Outcome, accountResult.Summary = result.Skipped, "attendance is available"
		case attendance.Done:
			accountResult.Outcome, accountResult.Summary = result.AlreadyClaimed, doneSummary(attendance)
		default:
			accountResult.Outcome, accountResult.Summary = result.Unavailable, "attendance unavailable"
		}
		return accountResult
	}
	if !attendance.Available {
		if attendance.Done {
			accountResult.Outcome, accountResult.Summary = result.AlreadyClaimed, doneSummary(attendance)
		} else {
			accountResult.Outcome, accountResult.Summary = result.Unavailable, "no action required"
		}
		return accountResult
	}

	expectedRewards := status.AvailableRewards()
	claim, err := client.ClaimOnce(ctx, token)
	if err != nil {
		return resultFromError(accountResult, err)
	}
	switch claim.Classify() {
	case skport.ClaimSuccess:
		accountResult.Outcome = result.Claimed
		accountResult.Rewards = claim.Rewards()
		if len(accountResult.Rewards) == 0 {
			accountResult.Rewards = expectedRewards
		}
		accountResult.Summary = rewardsSummary(accountResult.Rewards)
	case skport.ClaimAlreadyDone:
		accountResult.Outcome, accountResult.Summary = result.AlreadyClaimed, "API reports already claimed"
	case skport.ClaimAuthError:
		accountResult.Outcome, accountResult.Summary = result.AuthExpired, "claim rejected because login is required"
	case skport.ClaimAPIError:
		accountResult.Outcome, accountResult.Summary = result.ClaimError, "claim API returned a definite error"
	}
	return accountResult
}

func resultFromError(accountResult result.Account, err error) result.Account {
	var requestError *skport.Error
	if !errors.As(err, &requestError) {
		accountResult.Outcome, accountResult.Summary = result.InternalError, "unexpected internal error"
		return accountResult
	}

	switch requestError.Kind {
	case skport.ErrorAuth:
		accountResult.Outcome, accountResult.Summary = result.AuthExpired, "login is required"
	case skport.ErrorTransient:
		accountResult.Outcome, accountResult.Summary = result.TransientError, "temporary network or server failure"
	case skport.ErrorClaim:
		accountResult.Outcome, accountResult.Summary = result.ClaimError, "claim API returned a definite error"
	case skport.ErrorAmbiguous:
		accountResult.Outcome, accountResult.Summary = result.AmbiguousClaim, "claim result is unknown; it will not be retried automatically"
	default:
		accountResult.Outcome, accountResult.Summary = result.InternalError, "unexpected internal error"
	}
	return accountResult
}

func doneSummary(attendance skport.AttendanceState) string {
	if attendance.Conflict {
		return "conflicting attendance flags; treated as already claimed"
	}
	return "already claimed"
}

func rewardsSummary(rewards []result.Reward) string {
	if len(rewards) == 0 {
		return "rewards unknown"
	}
	var summary strings.Builder
	for i, reward := range rewards {
		if i > 0 {
			summary.WriteString(", ")
		}
		summary.WriteString(reward.Summary())
	}
	return summary.String()
}
