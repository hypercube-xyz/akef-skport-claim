package notify

import "github.com/hypercube-xyz/akef-skport-claim/internal/result"

type ntfyPresentation struct {
	Title    string
	Priority string
	Body     string
}

func newNtfyPresentation(runReport result.Run) ntfyPresentation {
	priority := "default"
	for _, account := range runReport.Accounts {
		switch account.Outcome {
		case result.AuthExpired, result.TransientError, result.ClaimError, result.AmbiguousClaim, result.InternalError:
			priority = "high"
		}
	}
	return ntfyPresentation{Title: "Arknights: Endfield SKPORT", Priority: priority, Body: truncateUTF8(formatNotification(runReport), 4000)}
}
