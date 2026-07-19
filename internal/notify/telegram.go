package notify

import "github.com/hypercube-xyz/akef-skport-claim/internal/result"

type telegramMessagePayload struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

func newTelegramPayload(chatID string, runReport result.Run) telegramMessagePayload {
	return telegramMessagePayload{ChatID: chatID, Text: truncateUTF8(formatNotification(runReport), 4096)}
}
