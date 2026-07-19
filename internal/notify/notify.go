package notify

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
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

func (s *Sender) SendAll(ctx context.Context, cfg *config.Config, runReport result.Run, store *state.Store) []error {
	if store == nil {
		store = &state.Store{Notifications: map[string]time.Time{}}
	}
	if !cfg.Notifications.Aggregate && len(runReport.Accounts) > 1 {
		var errs []error
		for _, account := range runReport.Accounts {
			single := runReport
			single.Accounts = []result.Account{account}
			errs = append(errs, s.sendReport(ctx, cfg, single, store)...)
		}
		return errs
	}
	return s.sendReport(ctx, cfg, runReport, store)
}

func (s *Sender) sendReport(ctx context.Context, cfg *config.Config, runReport result.Run, store *state.Store) []error {
	var errs []error
	for _, target := range cfg.Notifications.Targets {
		if !target.Enabled {
			continue
		}
		matched, hasNonDeduplicatedEvent, keys := selectEvents(target.Name, target.Events, runReport.Accounts)
		if !matched {
			continue
		}
		allRecent := !hasNonDeduplicatedEvent && len(keys) > 0
		now := s.now()
		for _, key := range keys {
			if !store.Recent(key, now, cfg.Run.NotificationErrorCooldown.Duration) {
				allRecent = false
				break
			}
		}
		if allRecent {
			continue
		}
		if err := s.sendTarget(ctx, target, runReport); err != nil {
			errs = append(errs, fmt.Errorf("notification target %q failed: %w", target.Name, err))
			continue
		}
		for _, key := range keys {
			store.Record(key, now)
		}
	}
	return errs
}

func (s *Sender) SendTest(ctx context.Context, target config.NotificationTarget) error {
	reportValue := result.Run{Duration: 10 * time.Millisecond, Accounts: []result.Account{{Name: "test", Outcome: result.Claimed, Summary: "synthetic notification test"}}}
	return s.sendTarget(ctx, target, reportValue)
}

func eventFor(outcome result.Outcome) string {
	if outcome == result.TransientError || outcome == result.ClaimError || outcome == result.AmbiguousClaim || outcome == result.InternalError {
		return "error"
	}
	return string(outcome)
}

func selectEvents(target string, events []string, results []result.Account) (matched, hasNonDeduplicatedEvent bool, keys []string) {
	for _, account := range results {
		event := eventFor(account.Outcome)
		if !slices.Contains(events, event) {
			continue
		}
		matched = true
		if event == "auth_expired" || event == "error" {
			keys = append(keys, dedupKey(target, account.Name, event))
			continue
		}
		hasNonDeduplicatedEvent = true
	}
	return matched, hasNonDeduplicatedEvent, keys
}

func dedupKey(target, account, event string) string {
	return strconv.Itoa(len(target)) + ":" + target + strconv.Itoa(len(account)) + ":" + account + event
}
