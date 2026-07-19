package skport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hypercube-xyz/akef-skport-claim/internal/config"
	"github.com/hypercube-xyz/akef-skport-claim/internal/version"
)

const (
	BaseURL        = "https://zonai.skport.com"
	RefreshPath    = "/web/v1/auth/refresh"
	AttendancePath = "/web/v1/game/endfield/attendance"
	maxBodyBytes   = 1 << 20
	maxAttempts    = 3
	requestLimit   = 5
)

type ErrorKind string

const (
	ErrorAuth      ErrorKind = "auth"
	ErrorTransient ErrorKind = "transient"
	ErrorClaim     ErrorKind = "claim"
	ErrorAmbiguous ErrorKind = "ambiguous_claim"
	ErrorInternal  ErrorKind = "internal"
)

type Error struct {
	Kind       ErrorKind
	Op         string
	Err        error
	Retryable  bool
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Kind) + " error during " + e.Op
	}
	return string(e.Kind) + " error during " + e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

type Options struct {
	BaseURL   string
	Client    *http.Client
	Now       func() time.Time
	Sleep     func(context.Context, time.Duration) error
	UserAgent string
}

type Client struct {
	account        config.Account
	baseURL        string
	http           *http.Client
	now            func() time.Time
	sleep          func(context.Context, time.Duration) error
	userAgent      string
	mu             sync.Mutex
	used           int
	claimAttempted bool
}

func New(account config.Account, timeout time.Duration, options Options) *Client {
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = BaseURL
	}
	httpClient := options.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		clone := *httpClient
		clone.Timeout = timeout
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		httpClient = &clone
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
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = "akef-claim/" + version.Version
	}
	return &Client{account: account, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, now: now, sleep: sleep, userAgent: userAgent}
}

func (c *Client) Refresh(ctx context.Context) (string, error) {
	var response RefreshResponse
	if err := c.retry(ctx, http.MethodGet, RefreshPath, func() http.Header { return refreshHeaders(c.account) }, "", 2, &response); err != nil {
		return "", err
	}
	if response.Code != 0 {
		kind := ErrorTransient
		if IsAuthCode(response.Code) {
			kind = ErrorAuth
		}
		return "", &Error{Kind: kind, Op: "refresh", Err: fmt.Errorf("API code %d", response.Code)}
	}
	if response.Data.Token == "" {
		return "", &Error{Kind: ErrorAuth, Op: "refresh", Err: errors.New("response did not contain a session token")}
	}
	return response.Data.Token, nil
}

func (c *Client) Status(ctx context.Context, token string) (AttendanceResponse, error) {
	var response AttendanceResponse
	headers := func() http.Header { return signedHeaders(c.account, token, AttendancePath, "", c.now()) }
	if err := c.retry(ctx, http.MethodGet, AttendancePath, headers, "", 1, &response); err != nil {
		return response, err
	}
	return response, nil
}

func (c *Client) ClaimOnce(ctx context.Context, token string) (ClaimResponse, error) {
	var response ClaimResponse
	c.mu.Lock()
	if c.claimAttempted {
		c.mu.Unlock()
		return response, &Error{Kind: ErrorInternal, Op: "claim", Err: errors.New("claim was already attempted for this account")}
	}
	c.claimAttempted = true
	c.mu.Unlock()
	body := "{}"
	err := c.do(ctx, http.MethodPost, AttendancePath, signedHeaders(c.account, token, AttendancePath, body, c.now()), body, true, &response)
	return response, err
}

func (c *Client) retry(ctx context.Context, method, path string, headers func() http.Header, body string, reservedRequests int, target any) error {
	delay := time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := c.do(ctx, method, path, headers(), body, false, target)
		if err == nil {
			return nil
		}
		var typed *Error
		if !errors.As(err, &typed) || typed.Kind != ErrorTransient || !typed.Retryable || attempt == maxAttempts || c.remainingRequests() <= reservedRequests {
			return err
		}
		wait := delay
		if typed.RetryAfter > 0 {
			wait = min(typed.RetryAfter, 30*time.Second)
		}
		if err := c.sleep(ctx, wait); err != nil {
			return &Error{Kind: ErrorTransient, Op: path, Err: err}
		}
		delay *= 2
	}
	return &Error{Kind: ErrorInternal, Op: path, Err: errors.New("retry loop exited unexpectedly")}
}

func (c *Client) remainingRequests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return requestLimit - c.used
}

func (c *Client) do(ctx context.Context, method, path string, headers http.Header, body string, claim bool, target any) error {
	c.mu.Lock()
	if c.used >= requestLimit {
		c.mu.Unlock()
		return &Error{Kind: ErrorInternal, Op: path, Err: errors.New("SKPORT request budget exhausted")}
	}
	c.used++
	c.mu.Unlock()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return &Error{Kind: ErrorInternal, Op: path, Err: err}
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(req)
	if err != nil {
		kind := ErrorTransient
		if claim {
			kind = ErrorAmbiguous
		}
		return &Error{Kind: kind, Op: path, Err: classifyNetworkError(err), Retryable: !claim}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		switch {
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			return &Error{Kind: ErrorAuth, Op: path, Err: fmt.Errorf("HTTP %d", response.StatusCode)}
		case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
			kind := ErrorTransient
			if claim {
				kind = ErrorAmbiguous
			}
			retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), c.now())
			return &Error{Kind: kind, Op: path, Err: fmt.Errorf("HTTP %d", response.StatusCode), Retryable: !claim, RetryAfter: retryAfter}
		default:
			kind := ErrorInternal
			if claim {
				kind = ErrorClaim
				if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
					kind = ErrorAmbiguous
				}
			}
			return &Error{Kind: kind, Op: path, Err: fmt.Errorf("HTTP %d", response.StatusCode)}
		}
	}
	limited := io.LimitReader(response.Body, maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		kind := ErrorTransient
		if claim {
			kind = ErrorAmbiguous
		}
		return &Error{Kind: kind, Op: path, Err: errors.New("failed to read response"), Retryable: !claim}
	}
	if len(data) > maxBodyBytes {
		kind := ErrorTransient
		if claim {
			kind = ErrorAmbiguous
		}
		return &Error{Kind: kind, Op: path, Err: errors.New("response exceeded 1 MiB")}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		kind := ErrorTransient
		if claim {
			kind = ErrorAmbiguous
		}
		return &Error{Kind: kind, Op: path, Err: errors.New("empty response")}
	}
	if err := json.Unmarshal(data, target); err != nil {
		kind := ErrorTransient
		if claim {
			kind = ErrorAmbiguous
		}
		return &Error{Kind: kind, Op: path, Err: errors.New("malformed JSON response")}
	}
	return nil
}

func refreshHeaders(account config.Account) http.Header {
	headers := commonHeaders(account)
	return headers
}

func commonHeaders(account config.Account) http.Header {
	return http.Header{
		"Accept":       []string{"*/*"},
		"Content-Type": []string{"application/json"},
		"Cred":         []string{account.Cred.Expose()},
		"Platform":     []string{config.Platform},
		"Vname":        []string{config.VName},
		"Origin":       []string{"https://game.skport.com"},
		"Referer":      []string{"https://game.skport.com/"},
	}
}

func signedHeaders(account config.Account, token, path, body string, now time.Time) http.Header {
	headers := commonHeaders(account)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	headers.Set("sk-language", account.Language)
	headers.Set("sk-game-role", account.GameRole.Expose())
	headers.Set("timestamp", timestamp)
	headers.Set("sign", GenerateSign(path, body, timestamp, token, config.Platform, config.VName))
	return headers
}

func classifyNetworkError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout")
	}
	return errors.New("network request failed")
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 && seconds <= int64((1<<63-1)/int64(time.Second)) {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return 0
}
