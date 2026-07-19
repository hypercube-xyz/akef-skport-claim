package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
)

func (s *Sender) sendTarget(ctx context.Context, target config.NotificationTarget, text string) error {
	var endpoint string
	var body []byte
	var encodeErr error
	headers := http.Header{}
	switch target.Type {
	case "discord":
		endpoint = target.Webhook.Expose()
		body, encodeErr = json.Marshal(map[string]string{
			"username": "Arknights: Endfield Daily Sign-in",
			"content":  truncateUTF8(text, 2000),
		})
		headers.Set("Content-Type", "application/json")
	case "telegram":
		endpoint = s.telegramBaseURL + "/bot" + url.PathEscape(target.BotToken.Expose()) + "/sendMessage"
		body, encodeErr = json.Marshal(map[string]string{
			"chat_id": target.ChatID.Expose(),
			"text":    truncateUTF8(text, 4096),
		})
		headers.Set("Content-Type", "application/json")
	case "ntfy":
		u, err := url.Parse(target.Server)
		if err != nil || u == nil {
			return errors.New("invalid ntfy server URL")
		}
		u.Path = path.Join(u.Path, target.Topic)
		endpoint = u.String()
		body = []byte(text)
		headers.Set("Content-Type", "text/plain; charset=utf-8")
		if !target.Token.Empty() {
			headers.Set("Authorization", "Bearer "+target.Token.Expose())
		}
	default:
		return errors.New("unsupported notification type")
	}
	if encodeErr != nil {
		return errors.New("failed to encode notification payload")
	}

	targetCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.postWithRetry(targetCtx, endpoint, headers, body)
}

func (s *Sender) postWithRetry(ctx context.Context, endpoint string, headers http.Header, body []byte) error {
	for attempt := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return errors.New("failed to create request")
		}
		req.Header = headers.Clone()
		response, err := s.httpClient.Do(req)
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
		}

		retryable := err != nil
		if err == nil && response != nil {
			retryable = response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		}
		if !retryable {
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return fmt.Errorf("HTTP %d", response.StatusCode)
			}
			return nil
		}
		if attempt == 1 {
			if err != nil {
				return errors.New("network request failed")
			}
			return fmt.Errorf("HTTP %d after retry", response.StatusCode)
		}

		delay := time.Second
		if err == nil {
			if value, parseErr := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); parseErr == nil && value >= 0 {
				if value >= 30 {
					delay = 30 * time.Second
				} else {
					delay = time.Duration(value) * time.Second
				}
			}
		}
		if err := s.sleep(ctx, delay); err != nil {
			return errors.New("retry interrupted")
		}
	}
	return errors.New("notification retry loop failed")
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	const suffix = "…"
	if limit < len(suffix) {
		cut := value[:limit]
		for !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
		return cut
	}
	cut := value[:limit-len(suffix)]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + suffix
}
