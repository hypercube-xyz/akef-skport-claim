package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
)

type failingTransport struct{ calls atomic.Int32 }

func (transport *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("network unavailable")
}

func TestTransportFailureBranches(t *testing.T) {
	sender := New(Options{})
	if err := sender.SendTest(context.Background(), config.NotificationTarget{Type: "unknown"}); err == nil {
		t.Fatal("unsupported target should fail")
	}
	if err := sender.SendTest(context.Background(), config.NotificationTarget{Type: "ntfy", Server: "://", Topic: "x"}); err == nil {
		t.Fatal("invalid ntfy URL should fail")
	}
	if err := sender.postWithRetry(context.Background(), "://", nil, nil); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("invalid endpoint error=%v", err)
	}

	transport := &failingTransport{}
	sender = New(Options{HTTPClient: &http.Client{Transport: transport}, Sleep: func(context.Context, time.Duration) error { return nil }})
	if err := sender.postWithRetry(context.Background(), "https://example.invalid", nil, nil); err == nil || !strings.Contains(err.Error(), "network request failed") || transport.calls.Load() != 2 {
		t.Fatalf("network retry error=%v calls=%d", err, transport.calls.Load())
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "60")
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	var delay time.Duration
	sender = New(Options{HTTPClient: server.Client(), Sleep: func(_ context.Context, got time.Duration) error {
		delay = got
		return context.Canceled
	}})
	if err := sender.postWithRetry(context.Background(), server.URL, nil, nil); err == nil || !strings.Contains(err.Error(), "retry interrupted") || delay != 30*time.Second {
		t.Fatalf("interrupted retry error=%v delay=%s", err, delay)
	}
}

func TestTruncateBoundaries(t *testing.T) {
	if got := truncateUTF8("abc", 0); got != "" {
		t.Fatalf("zero limit=%q", got)
	}
	if got := truncateUTF8("abc", 3); got != "abc" {
		t.Fatalf("exact limit=%q", got)
	}
}
