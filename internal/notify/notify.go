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
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/model"
	"github.com/hypercube-xyz/akef-skport-claim/internal/report"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

type Options struct {
	HTTPClient      *http.Client
	TelegramBaseURL string
	Now             func() time.Time
	Sleep           func(context.Context, time.Duration) error
}

type Sender struct {
	httpClient      *http.Client
	telegramBaseURL string
	now             func() time.Time
	sleep           func(context.Context, time.Duration) error
}

func New(options Options) *Sender {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		clone := *client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &clone
	}
	base := options.TelegramBaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	sleep := options.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	return &Sender{httpClient: client, telegramBaseURL: strings.TrimRight(base, "/"), now: now, sleep: sleep}
}

func (s *Sender) SendAll(ctx context.Context, cfg *config.Config, runReport model.RunReport, stateFile *state.File) []error {
	if stateFile == nil {
		stateFile = &state.File{Notifications: map[string]time.Time{}}
	}
	if !cfg.Notifications.Aggregate && len(runReport.Results) > 1 {
		var errs []error
		for _, result := range runReport.Results {
			single := runReport
			single.Results = []model.AccountResult{result}
			errs = append(errs, s.sendReport(ctx, cfg, single, stateFile)...)
		}
		return errs
	}
	return s.sendReport(ctx, cfg, runReport, stateFile)
}

func (s *Sender) sendReport(ctx context.Context, cfg *config.Config, runReport model.RunReport, stateFile *state.File) []error {
	text := report.Format(runReport)
	var errs []error
	for _, target := range cfg.Notifications.Targets {
		if !target.Enabled {
			continue
		}
		matched, hasNonDeduplicatedEvent, keys := selectEvents(target.Name, target.Events, runReport.Results)
		if !matched {
			continue
		}
		allRecent := !hasNonDeduplicatedEvent && len(keys) > 0
		now := s.now()
		for _, key := range keys {
			if !stateFile.Recent(key, now, cfg.Run.NotificationErrorCooldown.Duration) {
				allRecent = false
				break
			}
		}
		if allRecent {
			continue
		}
		if err := s.sendTarget(ctx, target, text); err != nil {
			errs = append(errs, fmt.Errorf("notification target %q failed: %w", target.Name, err))
			continue
		}
		for _, key := range keys {
			stateFile.Record(key, now)
		}
	}
	return errs
}

func (s *Sender) SendTest(ctx context.Context, target config.NotificationTarget) error {
	reportValue := model.RunReport{Duration: 10 * time.Millisecond, Results: []model.AccountResult{{Account: "test", Outcome: model.Claimed, Summary: "synthetic notification test"}}}
	return s.sendTarget(ctx, target, report.Format(reportValue))
}

func (s *Sender) sendTarget(ctx context.Context, target config.NotificationTarget, text string) error {
	var endpoint string
	var body []byte
	var encodeErr error
	headers := http.Header{}
	switch target.Type {
	case "discord":
		endpoint = target.Webhook.Expose()
		content := truncateUTF8(text, 2000)
		body, encodeErr = json.Marshal(map[string]string{"username": "Arknights: Endfield Daily Sign-in", "content": content})
		headers.Set("Content-Type", "application/json")
	case "telegram":
		endpoint = s.telegramBaseURL + "/bot" + url.PathEscape(target.BotToken.Expose()) + "/sendMessage"
		body, encodeErr = json.Marshal(map[string]string{"chat_id": target.ChatID.Expose(), "text": truncateUTF8(text, 4096)})
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
	for attempt := 0; attempt < 2; attempt++ {
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
				if value >= int64(30*time.Second/time.Second) {
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

func eventFor(outcome model.Outcome) string {
	if outcome == model.TransientError || outcome == model.ClaimError || outcome == model.AmbiguousClaim || outcome == model.InternalError {
		return "error"
	}
	return string(outcome)
}

func selectEvents(target string, events []string, results []model.AccountResult) (matched, hasNonDeduplicatedEvent bool, keys []string) {
	for _, result := range results {
		event := eventFor(result.Outcome)
		if !slices.Contains(events, event) {
			continue
		}
		matched = true
		if event == "auth_expired" || event == "error" {
			keys = append(keys, dedupKey(target, result.Account, event))
			continue
		}
		hasNonDeduplicatedEvent = true
	}
	return matched, hasNonDeduplicatedEvent, keys
}

func dedupKey(target, account, event string) string {
	return strconv.Itoa(len(target)) + ":" + target + strconv.Itoa(len(account)) + ":" + account + event
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
