package skport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
)

func TestSigningVector(t *testing.T) {
	if got := HeaderJSON("3", "1700000000", "1.0.0"); got != `{"platform":"3","timestamp":"1700000000","dId":"","vName":"1.0.0"}` {
		t.Fatalf("header JSON mismatch: %s", got)
	}
	got := GenerateSign(AttendancePath, "{}", "1700000000", "test-token", "3", "1.0.0")
	if got != "02870379a4fd448bccada11917fe9e1e" {
		t.Fatalf("signature mismatch: %s", got)
	}
}

func TestResponseFixtures(t *testing.T) {
	tests := []struct {
		name      string
		available bool
		done      bool
		valid     bool
		conflict  bool
	}{
		{"attendance_available.json", true, false, true, false},
		{"attendance_done.json", false, true, true, false},
		{"attendance_nested_available.json", true, false, true, false},
		{"attendance_claimed_has_today_false.json", false, true, true, true},
		{"attendance_invalid_session.json", false, false, false, false},
		{"attendance_mixed_available_today.json", true, false, true, false},
		{"attendance_after_claim_june_03.json", false, true, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response AttendanceResponse
			readFixture(t, test.name, &response)
			state := response.State()
			if state.Available != test.available || state.Done != test.done || state.SessionValid != test.valid || state.Conflict != test.conflict {
				t.Fatalf("unexpected parse: state=%+v", state)
			}
		})
	}
	var mixed AttendanceResponse
	readFixture(t, "attendance_mixed_available_today.json", &mixed)
	if got := mixed.AvailableRewards()[0].Summary(); got != "Talosian Credit Notes|T-Creds x2000" {
		t.Fatalf("reward mismatch: %s", got)
	}
	var conflicting AttendanceResponse
	readFixture(t, "attendance_claimed_has_today_false.json", &conflicting)
	if rewards := conflicting.AvailableRewards(); len(rewards) != 0 {
		t.Fatalf("conflicting attendance item exposed claimable rewards: %#v", rewards)
	}

	var mixedConflict AttendanceResponse
	mixedConflict.Code = 0
	mixedConflict.Data = json.RawMessage(`{"calendar":[{"available":true,"awardId":"safe-looking"},{"available":true,"done":true,"awardId":"conflict"}]}`)
	state := mixedConflict.State()
	if !state.Conflict || state.Available || !state.Done {
		t.Fatalf("mixed conflict must fail closed: state=%+v", state)
	}
	if rewards := mixedConflict.AvailableRewards(); len(rewards) != 0 {
		t.Fatalf("a conflicting response exposed claimable rewards: %#v", rewards)
	}
	var claim ClaimResponse
	readFixture(t, "claim_success_with_rewards.json", &claim)
	if claim.Classify() != ClaimSuccess || claim.Rewards()[0].Summary() != "Talosian Credit Notes|T-Creds x2000" {
		t.Fatal("claim fixture mismatch")
	}
	readFixture(t, "claim_already_done.json", &claim)
	if claim.Classify() != ClaimAlreadyDone {
		t.Fatal("already-done classification mismatch")
	}
}

func TestHTTPFlowHeadersAndClaimAtMostOnce(t *testing.T) {
	const timestamp = "1700000000"
	var refresh, status, claims atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "*/*" ||
			request.Header.Get("Cred") != "cred-secret" ||
			request.Header.Get("Content-Type") != "application/json" ||
			request.Header.Get("Origin") != "https://game.skport.com" ||
			request.Header.Get("Referer") != "https://game.skport.com/" ||
			request.Header.Get("Platform") != platform ||
			request.Header.Get("Vname") != clientVersion {
			t.Errorf("missing protocol headers: %v", request.Header)
		}
		for _, header := range []string{"Priority", "Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform", "Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site"} {
			if value := request.Header.Get(header); value != "" {
				t.Errorf("browser-only header %s must not be synthesized: %q", header, value)
			}
		}
		switch {
		case request.URL.Path == RefreshPath:
			refresh.Add(1)
			if request.Header.Get("Sign") != "" || request.Header.Get("Timestamp") != "" || request.Header.Get("Sk-Game-Role") != "" {
				t.Error("refresh request unexpectedly contained signed attendance headers")
			}
			_, _ = io.WriteString(writer, `{"code":0,"data":{"token":"token"}}`)
		case request.Method == http.MethodGet:
			status.Add(1)
			wantSign := GenerateSign(AttendancePath, "", timestamp, "token", platform, clientVersion)
			if request.Header.Get("Sign") != wantSign ||
				request.Header.Get("Timestamp") != timestamp ||
				request.Header.Get("Sk-Game-Role") != "role-secret" ||
				request.Header.Get("Sk-Language") != "en" {
				t.Errorf("invalid signed GET headers: %v", request.Header)
			}
			_, _ = io.WriteString(writer, `{"code":0,"data":{"available":true}}`)
		case request.Method == http.MethodPost:
			claims.Add(1)
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read claim body: %v", err)
			}
			wantSign := GenerateSign(AttendancePath, "{}", timestamp, "token", platform, clientVersion)
			if string(body) != "{}" ||
				request.Header.Get("Sign") != wantSign ||
				request.Header.Get("Timestamp") != timestamp ||
				request.Header.Get("Sk-Game-Role") != "role-secret" ||
				request.Header.Get("Sk-Language") != "en" {
				t.Errorf("invalid signed POST request: body=%q headers=%v", body, request.Header)
			}
			_, _ = io.WriteString(writer, `{"code":0,"data":{"awardIds":[]}}`)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	token, err := client.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Status(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ClaimOnce(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if refresh.Load() != 1 || status.Load() != 1 || claims.Load() != 1 {
		t.Fatalf("request counts: refresh=%d status=%d claim=%d", refresh.Load(), status.Load(), claims.Load())
	}
}

func TestClaimTransportAndMalformedResponsesAreAmbiguousAndNeverRetried(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"server", 500, "down"},
		{"redirect", 302, "redirect"},
		{"empty", 200, ""},
		{"malformed", 200, "{"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.WriteHeader(test.status)
				io.WriteString(writer, test.body)
			}))
			defer server.Close()
			_, err := testClient(server.URL, server.Client()).ClaimOnce(context.Background(), "token")
			var typed *Error
			if !errors.As(err, &typed) || typed.Kind != ErrorAmbiguous {
				t.Fatalf("expected ambiguous error, got %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("claim was retried %d times", calls.Load())
			}
		})
	}
}

func TestStatusRetriesTransientFailureAndRejectsLargeBodyAndRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/large":
			io.WriteString(writer, strings.Repeat("x", maxBodyBytes+1))
		case "/redirect":
			http.Redirect(writer, request, "/target", http.StatusFound)
		default:
			if calls.Add(1) < 3 {
				writer.WriteHeader(500)
				return
			}
			io.WriteString(writer, `{"code":0,"data":{"available":false}}`)
		}
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	if _, err := client.Status(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("wanted 3 attempts, got %d", calls.Load())
	}
	var target APIResponse
	if err := client.do(context.Background(), http.MethodGet, "/large", nil, "", false, &target); err == nil {
		t.Fatal("expected oversized response error")
	}
	if err := client.do(context.Background(), http.MethodGet, "/redirect", nil, "", false, &target); err == nil {
		t.Fatal("expected redirect error")
	}
}

func TestRefreshMissingTokenAndHTTPAuth(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing-token", 200, `{"code":0,"data":{"token":""}}`},
		{"unauthorized", 401, `{"code":0,"data":{"token":"ignored"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				io.WriteString(writer, test.body)
			}))
			defer server.Close()
			_, err := testClient(server.URL, server.Client()).Refresh(context.Background())
			var typed *Error
			if !errors.As(err, &typed) || typed.Kind != ErrorAuth {
				t.Fatalf("expected auth error, got %v", err)
			}
		})
	}
}

func TestRequestBudgetExhaustion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		io.WriteString(writer, `{"code":0,"data":{}}`)
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	for range requestLimit {
		var response APIResponse
		if err := client.do(context.Background(), http.MethodGet, AttendancePath, nil, "", false, &response); err != nil {
			t.Fatal(err)
		}
	}
	var response APIResponse
	err := client.do(context.Background(), http.MethodGet, AttendancePath, nil, "", false, &response)
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorInternal || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected request budget error, got %v", err)
	}
}

func TestClaimOnceRejectsSecondCallBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		io.WriteString(writer, `{"code":0,"data":{"awardIds":[]}}`)
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	if _, err := client.ClaimOnce(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	_, err := client.ClaimOnce(context.Background(), "token")
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorInternal || calls.Load() != 1 {
		t.Fatalf("second claim was not blocked: calls=%d err=%v", calls.Load(), err)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	if got := parseRetryAfter("60", now); got != time.Minute {
		t.Fatalf("seconds Retry-After: %s", got)
	}
	date := now.Add(45 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(date, now); got != 45*time.Second {
		t.Fatalf("date Retry-After: %s", got)
	}
}

func TestStatusRetriesReserveOneRequestForClaim(t *testing.T) {
	var refreshCalls, statusCalls, claimCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == RefreshPath:
			if refreshCalls.Add(1) < 3 {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			io.WriteString(writer, `{"code":0,"data":{"token":"token"}}`)
		case request.Method == http.MethodGet:
			statusCalls.Add(1)
			writer.WriteHeader(http.StatusInternalServerError)
		case request.Method == http.MethodPost:
			claimCalls.Add(1)
			io.WriteString(writer, `{"code":0,"data":{"awardIds":[]}}`)
		}
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	token, err := client.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Status(context.Background(), token); err == nil {
		t.Fatal("expected status failure")
	}
	if statusCalls.Load() != 1 {
		t.Fatalf("status consumed the claim reserve: %d calls", statusCalls.Load())
	}
	if _, err := client.ClaimOnce(context.Background(), token); err != nil {
		t.Fatalf("reserved claim request was unavailable: %v", err)
	}
	if refreshCalls.Load() != 3 || claimCalls.Load() != 1 {
		t.Fatalf("unexpected counts: refresh=%d claim=%d", refreshCalls.Load(), claimCalls.Load())
	}
}

func TestStatusRetriesRegenerateSignedHeaders(t *testing.T) {
	var calls atomic.Int32
	var timestamps []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		timestamps = append(timestamps, request.Header.Get("timestamp"))
		if calls.Add(1) < 3 {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(writer, `{"code":0,"data":{"available":false}}`)
	}))
	defer server.Close()
	now := time.Unix(1700000000, 0)
	client := New(
		config.Account{Credential: config.NewSecret("cred"), GameRole: config.NewSecret("role"), Language: "en"},
		time.Second,
		Options{
			BaseURL: server.URL,
			Client:  server.Client(),
			Now: func() time.Time {
				now = now.Add(time.Second)
				return now
			},
			Sleep: func(context.Context, time.Duration) error { return nil },
		},
	)
	if _, err := client.Status(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if len(timestamps) != 3 || timestamps[0] == timestamps[1] || timestamps[1] == timestamps[2] {
		t.Fatalf("signed headers were reused across retries: %v", timestamps)
	}
}

func TestOrdinaryHTTP4xxNeverLooksSuccessful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		if request.Method == http.MethodGet {
			io.WriteString(writer, `{"code":0,"data":{"token":"token"}}`)
			return
		}
		io.WriteString(writer, `{"code":0,"data":{"awardIds":[]}}`)
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	_, err := client.Refresh(context.Background())
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorInternal || typed.Retryable {
		t.Fatalf("ordinary GET 4xx classification: %v", err)
	}
	_, err = client.ClaimOnce(context.Background(), "token")
	if !errors.As(err, &typed) || typed.Kind != ErrorClaim {
		t.Fatalf("ordinary claim 4xx classification: %v", err)
	}
}

func TestRefreshAPIErrorDoesNotExposeServerMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.Header.Get("User-Agent"), "akef-claim/") {
			t.Errorf("missing versioned user agent: %q", request.Header.Get("User-Agent"))
		}
		io.WriteString(writer, `{"code":999,"message":"credential-secret","data":{}}`)
	}))
	defer server.Close()
	client := testClient(server.URL, server.Client())
	_, err := client.Refresh(context.Background())
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorTransient || strings.Contains(err.Error(), "credential-secret") {
		t.Fatalf("unsafe refresh error: %v", err)
	}
}

func TestInjectedHTTPClientStillUsesConfiguredTimeout(t *testing.T) {
	client := New(config.Account{}, 3*time.Second, Options{Client: &http.Client{Timeout: time.Minute}})
	if client.http.Timeout != 3*time.Second {
		t.Fatalf("timeout was not applied: %s", client.http.Timeout)
	}
}

func TestStatusHonorsRetryAfterFor429(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writer.Header().Set("Retry-After", "2")
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(writer, `{"code":0,"data":{"available":false}}`)
	}))
	defer server.Close()
	var slept time.Duration
	client := New(
		config.Account{Credential: config.NewSecret("cred"), GameRole: config.NewSecret("role"), Language: "en"},
		time.Second,
		Options{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return time.Unix(1700000000, 0) }, Sleep: func(_ context.Context, delay time.Duration) error {
			slept = delay
			return nil
		}},
	)
	if _, err := client.Status(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || slept != 2*time.Second {
		t.Fatalf("calls=%d slept=%s", calls.Load(), slept)
	}
}

func TestClaimTimeoutIsAmbiguousAndNotRetried(t *testing.T) {
	transport := &blockingTransport{}
	client := New(
		config.Account{Credential: config.NewSecret("cred"), GameRole: config.NewSecret("role"), Language: "en"},
		50*time.Millisecond,
		Options{Client: &http.Client{Transport: transport}, Now: func() time.Time { return time.Unix(1700000000, 0) }},
	)
	_, err := client.ClaimOnce(context.Background(), "token")
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorAmbiguous {
		t.Fatalf("expected ambiguous timeout, got %v", err)
	}
	if transport.calls.Load() != 1 {
		t.Fatalf("timed-out claim was retried: %d calls", transport.calls.Load())
	}
}

func TestClaimClassificationTable(t *testing.T) {
	for code, want := range map[int64]ClaimClass{0: ClaimSuccess, 10001: ClaimAlreadyDone, 401: ClaimAuthError, 10003: ClaimAuthError, 999: ClaimAPIError} {
		if got := (ClaimResponse{Code: code}).Classify(); got != want {
			t.Fatalf("code %d classified as %v, want %v", code, got, want)
		}
	}
}

type blockingTransport struct{ calls atomic.Int32 }

func (t *blockingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	<-request.Context().Done()
	return nil, request.Context().Err()
}

func testClient(baseURL string, httpClient *http.Client) *Client {
	account := config.Account{Name: "main", Enabled: true, Credential: config.NewSecret("cred-secret"), GameRole: config.NewSecret("role-secret"), Language: "en"}
	return New(account, time.Second, Options{BaseURL: baseURL, Client: httpClient, Now: func() time.Time { return time.Unix(1700000000, 0) }, Sleep: func(context.Context, time.Duration) error { return nil }})
}

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
