package skport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type readErrorBody struct{}

func (readErrorBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (readErrorBody) Close() error             { return nil }

func TestErrorContractAndDefaultClientOptions(t *testing.T) {
	inner := errors.New("cause")
	withCause := &Error{Kind: ErrorTransient, Op: "status", Err: inner}
	if !errors.Is(withCause, inner) || !strings.Contains(withCause.Error(), "cause") {
		t.Fatalf("wrapped error=%v", withCause)
	}
	withoutCause := &Error{Kind: ErrorAuth, Op: "refresh"}
	if got := withoutCause.Error(); got != "auth error during refresh" {
		t.Fatalf("error text=%q", got)
	}

	client := New(config.Account{}, time.Second, Options{BaseURL: "https://example.test///", UserAgent: "custom-agent"})
	if client.baseURL != "https://example.test" || client.userAgent != "custom-agent" || client.http == nil || client.now == nil || client.sleep == nil {
		t.Fatalf("client defaults=%#v", client)
	}
	if err := client.sleep(context.Background(), time.Nanosecond); err != nil {
		t.Fatalf("default sleep completion=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("default sleep cancellation=%v", err)
	}
}

func TestRefreshAuthCodeAndInterruptedRetry(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"code":10003,"data":{}}`)
	}))
	defer authServer.Close()
	if _, err := testClient(authServer.URL, authServer.Client()).Refresh(context.Background()); err == nil {
		t.Fatal("auth API code should fail")
	} else {
		var typed *Error
		if !errors.As(err, &typed) || typed.Kind != ErrorAuth {
			t.Fatalf("auth API code error=%v", err)
		}
	}

	retryServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer retryServer.Close()
	client := New(config.Account{}, time.Second, Options{
		BaseURL: retryServer.URL,
		Client:  retryServer.Client(),
		Sleep:   func(context.Context, time.Duration) error { return context.Canceled },
	})
	if _, err := client.Status(context.Background(), "token"); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("interrupted retry error=%v", err)
	}
}

func TestRequestConstructionAndBodyReadFailures(t *testing.T) {
	client := New(config.Account{}, time.Second, Options{BaseURL: "://", Client: &http.Client{}})
	var target APIResponse
	if err := client.do(context.Background(), http.MethodGet, "/bad", nil, "", false, &target); err == nil {
		t.Fatal("invalid request URL should fail")
	}

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: readErrorBody{}, Header: make(http.Header)}, nil
	})
	for _, claim := range []bool{false, true} {
		client = New(config.Account{}, time.Second, Options{BaseURL: "https://example.test", Client: &http.Client{Transport: transport}})
		err := client.do(context.Background(), http.MethodGet, "/read", nil, "", claim, &target)
		var typed *Error
		if !errors.As(err, &typed) {
			t.Fatalf("read failure was not typed: %v", err)
		}
		want := ErrorTransient
		if claim {
			want = ErrorAmbiguous
		}
		if typed.Kind != want {
			t.Fatalf("claim=%v kind=%s want %s", claim, typed.Kind, want)
		}
	}
	if got := classifyNetworkError(context.DeadlineExceeded); got.Error() != "timeout" {
		t.Fatalf("deadline classification=%v", got)
	}
}
