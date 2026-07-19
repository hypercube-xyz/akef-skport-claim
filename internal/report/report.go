package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

const (
	ExitOK        = 0
	ExitConfig    = 10
	ExitAuth      = 20
	ExitTransient = 30
	ExitClaim     = 40
	ExitAmbiguous = 41
	ExitInternal  = 50
)

func ExitCode(run result.Run) int {
	code := ExitOK
	for _, account := range run.Accounts {
		candidate := OutcomeExitCode(account.Outcome)
		if severity(candidate) > severity(code) {
			code = candidate
		}
	}
	return code
}

func OutcomeExitCode(outcome result.Outcome) int {
	switch outcome {
	case result.AuthExpired:
		return ExitAuth
	case result.TransientError:
		return ExitTransient
	case result.ClaimError:
		return ExitClaim
	case result.AmbiguousClaim:
		return ExitAmbiguous
	case result.InternalError:
		return ExitInternal
	default:
		return ExitOK
	}
}

func severity(code int) int {
	switch code {
	case ExitInternal:
		return 6
	case ExitAmbiguous:
		return 5
	case ExitClaim:
		return 4
	case ExitTransient:
		return 3
	case ExitAuth:
		return 2
	case ExitConfig:
		return 1
	default:
		return 0
	}
}

func Format(run result.Run) string {
	hasErrors := false
	for _, account := range run.Accounts {
		if OutcomeExitCode(account.Outcome) != 0 {
			hasErrors = true
			break
		}
	}
	var builder strings.Builder
	if hasErrors {
		builder.WriteString("AKEF daily run completed with errors\n\n")
	} else {
		builder.WriteString("AKEF daily run completed\n\n")
	}
	for _, account := range run.Accounts {
		summary := account.Summary
		if summary == "" {
			summary = rewardSummary(account.Rewards)
		}
		fmt.Fprintf(&builder, "%-12s %-18s %s\n", account.Name, account.Outcome, summary)
	}
	fmt.Fprintf(&builder, "\nDuration: %s", formatDuration(run.Duration))
	return builder.String()
}

func rewardSummary(rewards []result.Reward) string {
	if len(rewards) == 0 {
		return "no reward details"
	}
	parts := make([]string, len(rewards))
	for i, reward := range rewards {
		parts[i] = reward.Summary()
	}
	return strings.Join(parts, ", ")
}

func formatDuration(duration time.Duration) string {
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	return duration.Round(100 * time.Millisecond).String()
}
