package openlist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Default tuning. All values are overridable via Config.
const (
	defaultListInterval     = 2 * time.Second // ≤1 list request per 2s
	defaultGetInterval      = 2 * time.Second // ≤1 get request per 2s
	defaultJitterFraction   = 0.2             // ±20% random jitter on every spacing/backoff
	defaultMaxRetries       = 3               // retries on 429/5xx (hard cap)
	defaultRetryBaseDelay   = 500 * time.Millisecond
	defaultFailureThreshold = 5 // consecutive failures → circuit opens
	defaultCircuitOpenFor   = 30 * time.Minute
)

// Config configures a Client. The zero value of every optional field gets a sane
// default (see the constants above).
type Config struct {
	BaseURL  string // e.g. "http://localhost:5244" (no trailing slash required)
	Username string
	Password string

	// Token, when set, is used directly as the Authorization header value and no
	// login call is made (unless a 401 forces a re-login and credentials exist).
	Token string

	ListInterval   time.Duration // spacing between /api/fs/list requests
	GetInterval    time.Duration // spacing between /api/fs/get requests
	JitterFraction float64       // ± fraction for intervals and backoff (0.2 = ±20%)

	MaxRetries     int           // retries on 429/5xx, capped at 3
	RetryBaseDelay time.Duration // exponential backoff base (n-th retry: base*2^(n-1))

	FailureThreshold int           // consecutive failures before the circuit opens
	CircuitOpenFor   time.Duration // circuit open (fast-fail) duration

	HTTPClient *http.Client // optional, defaults to http.DefaultClient
}

// Client is a throttled, retrying, circuit-broken OpenList API client.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client

	maxRetries     int
	retryBaseDelay time.Duration
	jitterFrac     float64

	listLim *limiter
	getLim  *limiter
	authLim *limiter
	breaker *circuitBreaker

	// Test hooks (nil = real implementation). Unexported on purpose: they exist to
	// make the throttle/backoff/breaker deterministic under test, not for callers.
	now       func() time.Time
	sleep     func(ctx context.Context, d time.Duration) error
	randFloat func() float64

	tokenMu sync.Mutex
	token   string

	loginMu sync.Mutex // serializes logins to avoid a token stampede
}

// NewClient builds a Client from cfg. It does not perform any network I/O; the
// first login happens lazily on the first request.
func NewClient(cfg Config) *Client {
	if cfg.ListInterval == 0 {
		cfg.ListInterval = defaultListInterval
	}
	if cfg.GetInterval == 0 {
		cfg.GetInterval = defaultGetInterval
	}
	if cfg.JitterFraction == 0 {
		cfg.JitterFraction = defaultJitterFraction
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.MaxRetries > defaultMaxRetries {
		cfg.MaxRetries = defaultMaxRetries // hard cap, per spec
	}
	if cfg.RetryBaseDelay == 0 {
		cfg.RetryBaseDelay = defaultRetryBaseDelay
	}
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = defaultFailureThreshold
	}
	if cfg.CircuitOpenFor == 0 {
		cfg.CircuitOpenFor = defaultCircuitOpenFor
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	c := &Client{
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		username:       cfg.Username,
		password:       cfg.Password,
		httpClient:     hc,
		maxRetries:     cfg.MaxRetries,
		retryBaseDelay: cfg.RetryBaseDelay,
		jitterFrac:     cfg.JitterFraction,
		token:          cfg.Token,
		now:            time.Now,
		sleep:          defaultSleep,
		randFloat:      defaultRandFloat,
	}
	c.listLim = &limiter{interval: cfg.ListInterval, jitterFrac: cfg.JitterFraction, now: c.now, randFloat: c.randFloat}
	c.getLim = &limiter{interval: cfg.GetInterval, jitterFrac: cfg.JitterFraction, now: c.now, randFloat: c.randFloat}
	c.authLim = &limiter{interval: cfg.ListInterval, jitterFrac: cfg.JitterFraction, now: c.now, randFloat: c.randFloat}
	c.breaker = &circuitBreaker{
		threshold: cfg.FailureThreshold,
		openFor:   cfg.CircuitOpenFor,
		now:       c.now,
	}
	return c
}

// List returns one page of entries of the directory `path` (1-based page).
// Pagination is caller-driven: keep bumping `page` until an empty/nil slice is
// returned (or use listResponse.Total). On error the returned slice is nil.
func (c *Client) List(ctx context.Context, path string, page int) ([]Entry, error) {
	if page < 1 {
		page = 1
	}
	var data listResponse
	err := c.do(ctx, endpointList, listRequest{Path: path, Page: page}, &data, c.listLim, true)
	if err != nil {
		return nil, fmt.Errorf("openlist: list %q (page %d): %w", path, page, err)
	}
	return data.Content, nil
}

// Get returns the detail (incl. RawURL) of the object at `path`.
func (c *Client) Get(ctx context.Context, path string) (*FileInfo, error) {
	var info FileInfo
	err := c.do(ctx, endpointGet, getRequest{Path: path}, &info, c.getLim, true)
	if err != nil {
		return nil, fmt.Errorf("openlist: get %q: %w", path, err)
	}
	return &info, nil
}

// do runs one logical API request: breaker check → token → throttled send →
// retry on 429/5xx with exponential backoff → one re-login on 401.
func (c *Client) do(ctx context.Context, endpoint string, reqBody, out any, lim *limiter, auth bool) error {
	reauthed := false
	retries := 0
	for {
		if err := c.breaker.allow(); err != nil {
			return err
		}
		if auth {
			if err := c.ensureToken(ctx); err != nil {
				// ensureToken → login → do() already accounted the failure on the breaker.
				return err
			}
		}
		// Throttle: reserve the next slot, then sleep out the wait.
		if err := c.sleep(ctx, lim.reserve()); err != nil {
			return fmt.Errorf("openlist: request canceled while throttling: %w", err)
		}

		retryable, err := c.execute(ctx, endpoint, reqBody, out, auth)
		if err == nil {
			c.breaker.onSuccess()
			return nil
		}
		if errors.Is(err, ErrUnauthorized) {
			if auth && !reauthed && c.canRelogin() {
				// Token expired server-side: re-login once and replay the request.
				// This does not consume the retry budget nor the breaker budget —
				// an expired token is not an upstream failure.
				reauthed = true
				c.invalidateToken()
				continue
			}
			c.breaker.onFailure()
			return err
		}
		if !retryable || retries >= c.maxRetries {
			c.breaker.onFailure()
			return err
		}
		retries++
		backoff := jittered(c.retryBaseDelay<<uint(retries-1), c.jitterFrac, c.randFloat)
		if serr := c.sleep(ctx, backoff); serr != nil {
			return fmt.Errorf("openlist: request canceled while backing off: %w", serr)
		}
	}
}

// execute performs a single HTTP round trip (no retries). It returns
// retryable=true for transient failures (transport errors, 429, 5xx).
// ErrUnauthorized is returned bare so do() can decide whether to re-login.
func (c *Client) execute(ctx context.Context, endpoint string, reqBody, out any, auth bool) (retryable bool, err error) {
	var buf bytes.Buffer
	if reqBody != nil {
		if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
			return false, fmt.Errorf("openlist: encode request body: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, &buf)
	if err != nil {
		return false, fmt.Errorf("openlist: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		// OpenList expects the raw JWT in Authorization (no "Bearer " prefix).
		req.Header.Set("Authorization", c.getToken())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return true, fmt.Errorf("openlist: %s request failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, fmt.Errorf("openlist: read %s response: %w", endpoint, err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return false, ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return true, fmt.Errorf("openlist: %s: server returned status %d", endpoint, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("openlist: %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return false, fmt.Errorf("openlist: decode %s response: %w", endpoint, err)
	}
	switch {
	case env.Code == codeUnauthorized:
		return false, ErrUnauthorized
	case env.Code != codeOK:
		return false, &APIError{Code: env.Code, Message: env.Message}
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return false, fmt.Errorf("openlist: decode %s data: %w", endpoint, err)
		}
	}
	return false, nil
}

// canRelogin reports whether we are able to obtain a fresh token on 401.
func (c *Client) canRelogin() bool {
	return c.username != ""
}

func (c *Client) getToken() string {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	return c.token
}

func (c *Client) setToken(t string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.token = t
}

func (c *Client) invalidateToken() {
	c.setToken("")
}

// ensureToken logs in once (token is cached for subsequent requests).
func (c *Client) ensureToken(ctx context.Context) error {
	if c.getToken() != "" {
		return nil
	}
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.getToken() != "" {
		return nil // another goroutine logged in while we waited
	}
	return c.login(ctx)
}

func (c *Client) login(ctx context.Context) error {
	if c.username == "" {
		return errNoCredentials
	}
	var resp loginResponse
	err := c.do(ctx, endpointLogin, loginRequest{Username: c.username, Password: c.password}, &resp, c.authLim, false)
	if err != nil {
		return fmt.Errorf("openlist: login: %w", err)
	}
	if resp.Token == "" {
		return fmt.Errorf("openlist: login: empty token in response")
	}
	c.setToken(resp.Token)
	return nil
}
