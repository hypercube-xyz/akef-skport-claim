package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/model"
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

func ExitCode(report model.RunReport) int {
	code := ExitOK
	for _, result := range report.Results {
		candidate := OutcomeExitCode(result.Outcome)
		if severity(candidate) > severity(code) {
			code = candidate
		}
	}
	return code
}

func OutcomeExitCode(outcome model.Outcome) int {
	switch outcome {
	case model.AuthExpired:
		return ExitAuth
	case model.TransientError:
		return ExitTransient
	case model.ClaimError:
		return ExitClaim
	case model.AmbiguousClaim:
		return ExitAmbiguous
	case model.InternalError:
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

func Format(report model.RunReport) string {
	hasErrors := false
	for _, result := range report.Results {
		if OutcomeExitCode(result.Outcome) != 0 {
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
	for _, result := range report.Results {
		summary := result.Summary
		if summary == "" {
			summary = rewardSummary(result.Rewards)
		}
		fmt.Fprintf(&builder, "%-12s %-18s %s\n", result.Account, result.Outcome, summary)
	}
	fmt.Fprintf(&builder, "\nDuration: %s", formatDuration(report.Duration))
	return builder.String()
}

func rewardSummary(rewards []model.Reward) string {
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
