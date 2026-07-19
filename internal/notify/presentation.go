package notify

import (
	"strings"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

func formatNotification(runReport result.Run) string {
	if len(runReport.Accounts) == 0 {
		return "No account results"
	}

	lines := make([]string, len(runReport.Accounts))
	for i, account := range runReport.Accounts {
		lines[i] = "[" + account.Name + "]: " + notificationAccountMessage(account)
	}
	return strings.Join(lines, "\n")
}

func notificationAccountMessage(account result.Account) string {
	detail := notificationAccountDetail(account)
	switch account.Outcome {
	case result.AuthExpired, result.TransientError, result.ClaimError, result.AmbiguousClaim, result.InternalError:
		return "Error " + detail
	default:
		return detail
	}
}

func notificationAccountDetail(account result.Account) string {
	if account.Outcome == result.Claimed && len(account.Rewards) > 0 {
		parts := make([]string, len(account.Rewards))
		for i, reward := range account.Rewards {
			parts[i] = reward.Summary()
		}
		return strings.Join(parts, ", ")
	}
	if account.Summary != "" {
		return account.Summary
	}
	switch account.Outcome {
	case result.Claimed:
		return "claimed"
	case result.AlreadyClaimed:
		return "already claimed"
	case result.Unavailable:
		return "not available"
	case result.AuthExpired:
		return "login required"
	case result.TransientError:
		return "temporary failure"
	case result.ClaimError:
		return "claim failed"
	case result.AmbiguousClaim:
		return "claim status unknown"
	case result.InternalError:
		return "internal failure"
	case result.Skipped:
		return "available"
	default:
		return string(account.Outcome)
	}
}
