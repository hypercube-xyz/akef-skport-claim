package notify

import "github.com/hypercube-xyz/akef-skport-claim/internal/result"

type discordWebhookPayload struct {
	Username        string                 `json:"username"`
	Content         string                 `json:"content"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
}

type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}

func newDiscordPayload(runReport result.Run) discordWebhookPayload {
	return discordWebhookPayload{
		Username:        "Arknights: Endfield Daily Sign-in",
		Content:         truncateUTF8(formatNotification(runReport), 2000),
		AllowedMentions: discordAllowedMentions{Parse: []string{}},
	}
}
