package skport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func testAccount() config.Account {
	return config.Account{
		Name:       "test",
		Credential: config.NewSecret("test-cred"),
		GameRole:   config.NewSecret("test-role"),
		Language:   "en",
	}
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func noopSleep(_ context.Context, _ time.Duration) error { return nil }

// stubServer is a clocked httptest.Server with a helper to build an API
// that targets it and a known now.
type stubServer struct {
	*httptest.Server
	now time.Time
}

func newStub(t *testing.T) *stubServer {
	t.Helper()
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	s := &stubServer{now: now}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default: success with empty body.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"token": "session-token"}})
	}))
	return s
}

func (s *stubServer) API(options Options) *API {
	options.BaseURL = s.URL
	if options.Now == nil {
		options.Now = fixedNow(s.now)
	}
	if options.Sleep == nil {
		options.Sleep = noopSleep
	}
	return New(testAccount(), 10*time.Second, options)
}

func jsonResponse(code int64, data any) []byte {
	body := map[string]any{"code": code}
	if data != nil {
		body["data"] = data
	}
	b, _ := json.Marshal(body)
	return b
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	api := New(testAccount(), 5*time.Second, Options{})
	if api.baseURL != BaseURL {
		t.Errorf("baseURL = %q; want %q", api.baseURL, BaseURL)
	}
	if api.http.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v; want 5s", api.http.Timeout)
	}
	if api.userAgent != "akef-claim/dev" {
		t.Errorf("userAgent = %q; want 'akef-claim/dev'", api.userAgent)
	}
}

func TestNew_CustomOptions(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	api := New(testAccount(), 3*time.Second, Options{
		BaseURL:   "https://custom.example.com",
		Now:       fixedNow(now),
		Sleep:     noopSleep,
		UserAgent: "custom/1.0",
	})
	if api.baseURL != "https://custom.example.com" {
		t.Errorf("baseURL = %q", api.baseURL)
	}
	if api.now() != now {
		t.Error("now() does not match fixed time")
	}
	if api.userAgent != "custom/1.0" {
		t.Errorf("userAgent = %q", api.userAgent)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	api := New(testAccount(), time.Second, Options{BaseURL: "https://example.com/"})
	if api.baseURL != "https://example.com" {
		t.Errorf("baseURL = %q; want 'https://example.com'", api.baseURL)
	}
}

func TestNew_CustomClientPreservesTimeout(t *testing.T) {
	custom := &http.Client{Timeout: 99 * time.Second}
	api := New(testAccount(), 5*time.Second, Options{Client: custom})
	// New clones the client and overrides Timeout.
	if api.http.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v; want 5s (overridden)", api.http.Timeout)
	}
	// Original unchanged.
	if custom.Timeout != 99*time.Second {
		t.Errorf("original Timeout mutated = %v; want 99s", custom.Timeout)
	}
}

// ---------------------------------------------------------------------------
// Refresh
// ---------------------------------------------------------------------------

func TestRefresh_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s; want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, map[string]string{"token": "abc123"}))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	token, err := api.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if token != "abc123" {
		t.Errorf("token = %q; want 'abc123'", token)
	}
}

func TestRefresh_AuthCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(401, nil))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	_, err := api.Refresh(context.Background())
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAuth {
		t.Errorf("error = %v; want auth error", err)
	}
}

func TestRefresh_MissingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, map[string]string{}))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	_, err := api.Refresh(context.Background())
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAuth {
		t.Errorf("error = %v; want auth error", err)
	}
}

func TestRefresh_TransientRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, map[string]string{"token": "tok"}))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	token, err := api.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if token != "tok" {
		t.Errorf("token = %q; want 'tok'", token)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d; want 3", attempts)
	}
}

func TestRefresh_ExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	_, err := api.Refresh(context.Background())
	var skErr *Error
	if !errors.As(err, &skErr) {
		t.Fatalf("error = %v; want *Error", err)
	}
	// maxAttempts=3 → 3 attempts, all fail → last error returned.
	if skErr.Kind != ErrorTransient {
		t.Errorf("Kind = %q; want transient", skErr.Kind)
	}
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

func TestStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s; want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, map[string]bool{"available": true}))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	resp, err := api.Status(context.Background(), "token")
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("Code = %d; want 0", resp.Code)
	}
}

// ---------------------------------------------------------------------------
// ClaimOnce
// ---------------------------------------------------------------------------

func TestClaimOnce_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, map[string]any{"awardIds": []string{"r1"}}))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	resp, err := api.ClaimOnce(context.Background(), "token")
	if err != nil {
		t.Fatalf("ClaimOnce() error: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("Code = %d; want 0", resp.Code)
	}
}

func TestClaimOnce_DoubleCallPrevented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, nil))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	_, err := api.ClaimOnce(context.Background(), "token")
	if err != nil {
		t.Fatalf("first ClaimOnce() error: %v", err)
	}
	_, err = api.ClaimOnce(context.Background(), "token")
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorInternal {
		t.Errorf("second ClaimOnce() error = %v; want internal error", err)
	}
}

func TestClaimOnce_ConcurrentDoubleCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse(0, nil))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	for i := range 2 {
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = api.ClaimOnce(context.Background(), "token")
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d; want exactly 1", successes)
	}
}

// ---------------------------------------------------------------------------
// do (request budget, HTTP errors, body limits)
// ---------------------------------------------------------------------------

func TestDo_RequestBudgetExhausted(t *testing.T) {
	srv := newStub(t)
	t.Cleanup(srv.Close)

	api := srv.API(Options{})
	// 5 requests allowed.
	for range 5 {
		api.used++
	}
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorInternal {
		t.Errorf("error = %v; want internal (budget exhausted)", err)
	}
}

func TestDo_HTTPAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAuth {
		t.Errorf("error = %v; want auth error", err)
	}
}

func TestDo_HTTPTransientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep, Now: fixedNow(time.Now())})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorTransient {
		t.Errorf("error = %v; want transient", err)
	}
	if skErr.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v; want 30s", skErr.RetryAfter)
	}
}

func TestDo_ClaimTransientBecomesAmbiguous(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodPost, "/attendance", http.Header{}, "{}", true, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAmbiguous {
		t.Errorf("error = %v; want ambiguous (claim + 5xx)", err)
	}
	if skErr.Retryable {
		t.Error("Retryable = true; want false for claim")
	}
}

func TestDo_ClaimHTTPErrorBecomesClaimError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodPost, "/attendance", http.Header{}, "{}", true, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorClaim {
		t.Errorf("error = %v; want claim error", err)
	}
}

func TestDo_Claim3xxBecomesAmbiguous(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodPost, "/attendance", http.Header{}, "{}", true, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAmbiguous {
		t.Errorf("error = %v; want ambiguous (claim 3xx)", err)
	}
}

func TestDo_BodyTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return more than 1 MiB.
		large := make([]byte, maxBodyBytes+1)
		for i := range large {
			large[i] = 'x'
		}
		w.Write(large)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorTransient {
		t.Errorf("error = %v; want transient (body too large)", err)
	}
}

func TestDo_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorTransient {
		t.Errorf("error = %v; want transient (empty)", err)
	}
}

func TestDo_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorTransient {
		t.Errorf("error = %v; want transient (malformed JSON)", err)
	}
}

func TestDo_NetworkError(t *testing.T) {
	api := New(testAccount(), 5*time.Second, Options{BaseURL: "http://127.0.0.1:1", Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodGet, "/test", http.Header{}, "", false, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorTransient {
		t.Errorf("error = %v; want transient", err)
	}
}

func TestDo_ClaimNetworkErrorAmbiguous(t *testing.T) {
	api := New(testAccount(), 5*time.Second, Options{BaseURL: "http://127.0.0.1:1", Sleep: noopSleep})
	var resp map[string]any
	err := api.do(context.Background(), http.MethodPost, "/test", http.Header{}, "{}", true, &resp)
	var skErr *Error
	if !errors.As(err, &skErr) || skErr.Kind != ErrorAmbiguous {
		t.Errorf("error = %v; want ambiguous", err)
	}
}

// ---------------------------------------------------------------------------
// retryJSON
// ---------------------------------------------------------------------------

func TestRetryJSON_NonRetryableError(t *testing.T) {
	// Auth error is not retryable.
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: noopSleep})
	_, err := retryJSON[RefreshResponse](api, context.Background(), http.MethodGet, "/", func() http.Header { return http.Header{} }, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d; want 1 (no retry on auth)", attempts)
	}
}

func TestRetryJSON_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	api := New(testAccount(), 5*time.Second, Options{BaseURL: srv.URL, Sleep: func(ctx context.Context, d time.Duration) error {
		return ctx.Err() // simulate sleep that respects context
	}})
	_, err := retryJSON[RefreshResponse](api, ctx, http.MethodGet, "/", func() http.Header { return http.Header{} }, "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// parseRetryAfter
// ---------------------------------------------------------------------------

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"seconds", "60", 60 * time.Second},
		{"zero", "0", 0},
		{"date", "Wed, 15 Jan 2025 12:02:00 GMT", 2 * time.Minute},
		{"invalid", "not-a-number", 0},
		{"negative", "-5", 0},
		{"past date", "Wed, 15 Jan 2025 11:00:00 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.value, now)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v; want %v", tt.value, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// classifyNetworkError
// ---------------------------------------------------------------------------

func TestClassifyNetworkError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"generic", errors.New("some error"), "network request failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyNetworkError(tt.err)
			if !strings.Contains(got.Error(), tt.want) {
				t.Errorf("classifyNetworkError() = %q; want contains %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// commonHeaders / signedHeaders
// ---------------------------------------------------------------------------

func TestCommonHeaders(t *testing.T) {
	headers := commonHeaders(testAccount())
	if headers.Get("Cred") != "test-cred" {
		t.Errorf("Cred = %q; want 'test-cred'", headers.Get("Cred"))
	}
	if headers.Get("Platform") != "3" {
		t.Errorf("Platform = %q; want '3'", headers.Get("Platform"))
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", headers.Get("Content-Type"))
	}
}

func TestSignedHeaders(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	headers := signedHeaders(testAccount(), "token", "/path", "{}", now)
	if headers.Get("sk-language") != "en" {
		t.Errorf("sk-language = %q; want 'en'", headers.Get("sk-language"))
	}
	if headers.Get("timestamp") != "1736942400" {
		t.Errorf("timestamp = %q; want '1736942400'", headers.Get("timestamp"))
	}
	sign := headers.Get("sign")
	if len(sign) != 32 {
		t.Errorf("sign length = %d; want 32 (MD5 hex)", len(sign))
	}
}

// ---------------------------------------------------------------------------
// Error type
// ---------------------------------------------------------------------------

func TestError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &Error{Kind: ErrorAuth, Op: "refresh", Err: inner}
	if !errors.Is(e, inner) {
		t.Error("Unwrap() does not return inner error")
	}
}

func TestError_ErrorString(t *testing.T) {
	e := &Error{Kind: ErrorAuth, Op: "refresh", Err: errors.New("bad cred")}
	if !strings.Contains(e.Error(), "auth error during refresh: bad cred") {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestError_NilErr(t *testing.T) {
	e := &Error{Kind: ErrorTransient, Op: "status"}
	if !strings.Contains(e.Error(), "transient error during status") {
		t.Errorf("Error() = %q", e.Error())
	}
}

// ---------------------------------------------------------------------------
// remainingRequests
// ---------------------------------------------------------------------------

func TestRemainingRequests(t *testing.T) {
	api := New(testAccount(), time.Second, Options{})
	if got := api.remainingRequests(); got != requestLimit {
		t.Errorf("remainingRequests() = %d; want %d", got, requestLimit)
	}
	api.used = 3
	if got := api.remainingRequests(); got != requestLimit-3 {
		t.Errorf("remainingRequests() = %d; want %d", got, requestLimit-3)
	}
}