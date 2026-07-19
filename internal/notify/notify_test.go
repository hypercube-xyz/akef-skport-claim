package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
	"github.com/hypercube-xyz/akef-skport-claim/internal/state"
)

func TestDiscordPayloadAndRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var payload discordWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if payload.Username != "Arknights: Endfield Daily Sign-in" ||
			payload.Content != "[test]: synthetic notification test" ||
			payload.AllowedMentions.Parse == nil {
			t.Errorf("bad payload: %s", body)
		}
		if calls.Add(1) == 1 {
			writer.WriteHeader(500)
			return
		}
		writer.WriteHeader(204)
	}))
	defer server.Close()
	sender := New(Options{HTTPClient: server.Client(), Sleep: func(context.Context, time.Duration) error { return nil }})
	target := config.NotificationTarget{Name: "discord", Type: "discord", Webhook: config.NewSecret(server.URL)}
	if err := sender.SendTest(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("wanted one retry, got %d calls", calls.Load())
	}
}

func TestFailedTargetDoesNotStopLaterTargetAndDeduplicatesErrors(t *testing.T) {
	var success atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/fail" {
			writer.WriteHeader(400)
			return
		}
		success.Add(1)
		writer.WriteHeader(204)
	}))
	defer server.Close()
	now := time.Unix(1000, 0)
	sender := New(Options{HTTPClient: server.Client(), Now: func() time.Time { return now }, Sleep: func(context.Context, time.Duration) error { return nil }})
	cfg := &config.Config{Run: config.RunConfig{NotificationErrorCooldown: config.Duration{Duration: time.Hour}}, Notifications: config.Notifications{Targets: []config.NotificationTarget{
		{Name: "bad", Type: "discord", Enabled: true, Webhook: config.NewSecret(server.URL + "/fail"), Events: []string{"error"}},
		{Name: "good", Type: "discord", Enabled: true, Webhook: config.NewSecret(server.URL + "/good"), Events: []string{"error"}},
	}}}
	runReport := result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.TransientError}}}
	stateFile := &state.Store{Notifications: map[string]time.Time{}}
	errs := sender.SendAll(context.Background(), cfg, runReport, stateFile)
	if len(errs) != 1 || success.Load() != 1 {
		t.Fatalf("errs=%v success=%d", errs, success.Load())
	}
	sender.SendAll(context.Background(), cfg, runReport, stateFile)
	if success.Load() != 1 {
		t.Fatal("successful target was not deduplicated")
	}
}

func TestTelegramAndNtfyPayloads(t *testing.T) {
	var telegram, ntfy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		switch {
		case strings.Contains(request.URL.Path, "/bottest-token/sendMessage"):
			var payload telegramMessagePayload
			_ = json.Unmarshal(body, &payload)
			telegram.Store(payload.ChatID == "chat-1" && payload.Text == "[test]: synthetic notification test")
		case request.URL.Path == "/topic-safe":
			ntfy.Store(request.Header.Get("Authorization") == "Bearer ntfy-token" &&
				request.Header.Get("Title") == "AKEF" &&
				request.Header.Get("Tags") == "" &&
				request.Header.Get("Priority") == "default" &&
				string(body) == "[test]: synthetic notification test")
		}
		writer.WriteHeader(200)
	}))
	defer server.Close()
	sender := New(Options{HTTPClient: server.Client(), TelegramBaseURL: server.URL, Sleep: func(context.Context, time.Duration) error { return nil }})
	if err := sender.SendTest(context.Background(), config.NotificationTarget{Type: "telegram", BotToken: config.NewSecret("test-token"), ChatID: config.NewSecret("chat-1")}); err != nil {
		t.Fatal(err)
	}
	if err := sender.SendTest(context.Background(), config.NotificationTarget{Type: "ntfy", Server: server.URL, Topic: "topic-safe", Token: config.NewSecret("ntfy-token")}); err != nil {
		t.Fatal(err)
	}
	if !telegram.Load() || !ntfy.Load() {
		t.Fatalf("telegram=%v ntfy=%v", telegram.Load(), ntfy.Load())
	}
}

func TestTelegramPayloadStaysPlain(t *testing.T) {
	payload := newTelegramPayload("chat", result.Run{Accounts: []result.Account{{
		Name:    "<admin>@everyone",
		Outcome: result.Claimed,
		Summary: "reward <script>@user",
	}}})
	if payload.Text != "[<admin>@everyone]: reward <script>@user" {
		t.Fatalf("unexpected Telegram payload: %#v", payload)
	}
}

func TestNtfyAttentionPresentationUsesHighPriority(t *testing.T) {
	presentation := newNtfyPresentation(result.Run{Accounts: []result.Account{{Name: "main", Outcome: result.AuthExpired, Summary: "login required"}}})
	if presentation.Priority != "high" || presentation.Title != "AKEF" || presentation.Body != "[main]: Error login required" {
		t.Fatalf("unexpected ntfy presentation: %#v", presentation)
	}
}

func TestRecentErrorDoesNotSuppressClaimedEvent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	now := time.Unix(1000, 0)
	sender := New(Options{HTTPClient: server.Client(), Now: func() time.Time { return now }})
	cfg := &config.Config{Run: config.RunConfig{NotificationErrorCooldown: config.Duration{Duration: time.Hour}}, Notifications: config.Notifications{Targets: []config.NotificationTarget{{
		Name: "mixed", Type: "discord", Enabled: true, Webhook: config.NewSecret(server.URL), Events: []string{"claimed", "error"},
	}}}}
	stateFile := &state.Store{Notifications: map[string]time.Time{dedupKey("mixed", "main", "error"): now.Add(-time.Minute)}}
	runReport := result.Run{Accounts: []result.Account{
		{Name: "main", Outcome: result.TransientError},
		{Name: "secondary", Outcome: result.Claimed},
	}}
	if errs := sender.SendAll(context.Background(), cfg, runReport, stateFile); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if calls.Load() != 1 {
		t.Fatalf("claimed notification was incorrectly deduplicated: %d calls", calls.Load())
	}
}

func TestErrorOutcomesShareDeduplicationCategory(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	now := time.Unix(1000, 0)
	sender := New(Options{HTTPClient: server.Client(), Now: func() time.Time { return now }})
	cfg := &config.Config{Run: config.RunConfig{NotificationErrorCooldown: config.Duration{Duration: time.Hour}}, Notifications: config.Notifications{Targets: []config.NotificationTarget{{
		Name: "errors", Type: "discord", Enabled: true, Webhook: config.NewSecret(server.URL), Events: []string{"error"},
	}}}}
	stateFile := &state.Store{Notifications: map[string]time.Time{}}
	for _, outcome := range []result.Outcome{result.TransientError, result.InternalError} {
		runReport := result.Run{Accounts: []result.Account{{Name: "main", Outcome: outcome}}}
		if errs := sender.SendAll(context.Background(), cfg, runReport, stateFile); len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("error category was not deduplicated: %d calls", calls.Load())
	}
}

func TestTruncateUTF8RespectsByteLimit(t *testing.T) {
	for _, test := range []struct {
		value string
		limit int
	}{
		{strings.Repeat("a", 2100), 2000},
		{strings.Repeat("ก", 1000), 2000},
		{"ก", 2},
	} {
		got := truncateUTF8(test.value, test.limit)
		if len(got) > test.limit || !utf8.ValidString(got) {
			t.Fatalf("invalid truncation: bytes=%d limit=%d valid=%v", len(got), test.limit, utf8.ValidString(got))
		}
	}
}

func TestNotificationCancellationStopsRetryAndKeepsEndpointSecret(t *testing.T) {
	transport := &blockingTransport{}
	sender := New(Options{HTTPClient: &http.Client{Transport: transport}})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	secretPath := "/webhook-secret-value"
	err := sender.SendTest(ctx, config.NotificationTarget{Type: "discord", Webhook: config.NewSecret("https://example.invalid" + secretPath)})
	if err == nil || strings.Contains(err.Error(), secretPath) {
		t.Fatalf("unsafe cancellation error: %v", err)
	}
	if transport.calls.Load() != 1 {
		t.Fatalf("canceled notification was retried: %d calls", transport.calls.Load())
	}
}

type blockingTransport struct{ calls atomic.Int32 }

func (t *blockingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	<-request.Context().Done()
	return nil, request.Context().Err()
}
