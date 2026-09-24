package openlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers: virtual clock + scripted OpenList mock
// ---------------------------------------------------------------------------

// virtualClock replaces time.Now/time.Sleep so throttle/backoff/breaker tests
// run instantly yet deterministically.
type virtualClock struct {
	mu     sync.Mutex
	t      time.Time
	sleeps []time.Duration
}

func newVirtualClock() *virtualClock {
	return &virtualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (v *virtualClock) Now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.t
}

func (v *virtualClock) Sleep(_ context.Context, d time.Duration) error {
	if d < 0 {
		d = 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sleeps = append(v.sleeps, d)
	v.t = v.t.Add(d)
	return nil
}

func (v *virtualClock) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.t = v.t.Add(d)
}

func (v *virtualClock) recorded() []time.Duration {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]time.Duration, len(v.sleeps))
	copy(out, v.sleeps)
	return out
}

// positive returns only the non-zero waits (backoff/throttle sleeps, not the
// zero-wait probes issued before every request).
func (v *virtualClock) positive() []time.Duration {
	var out []time.Duration
	for _, d := range v.recorded() {
		if d > 0 {
			out = append(out, d)
		}
	}
	return out
}

func (v *virtualClock) resetSleeps() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sleeps = nil
}

// apiMock is a scripted OpenList server. Requests are recorded so tests can
// assert on headers/bodies/pagination.
type apiMock struct {
	mu      sync.Mutex
	t       *testing.T
	logins  int
	reqs    []recordedReq
	listFn  func(w http.ResponseWriter, r *http.Request, page int)
	getFn   func(w http.ResponseWriter, r *http.Request, body map[string]any)
	loginFn func(w http.ResponseWriter, r *http.Request)
	hits    int // total HTTP hits (for breaker assertions)
}

type recordedReq struct {
	path   string
	auth   string
	body   map[string]any
	status int
}

func newAPIMock(t *testing.T) *apiMock {
	return &apiMock{t: t}
}

func (m *apiMock) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(endpointLogin, func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.hits++
		m.logins++
		m.mu.Unlock()
		if m.loginFn != nil {
			m.loginFn(w, r)
			return
		}
		writeData(w, map[string]any{"token": fmt.Sprintf("tok-%d", m.logins)})
	})
	mux.HandleFunc(endpointList, func(w http.ResponseWriter, r *http.Request) {
		body := readBody(r)
		m.mu.Lock()
		m.hits++
		m.reqs = append(m.reqs, recordedReq{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
		m.mu.Unlock()
		if m.listFn != nil {
			m.listFn(w, r, int(asFloat(body["page"])))
			return
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	})
	mux.HandleFunc(endpointGet, func(w http.ResponseWriter, r *http.Request) {
		body := readBody(r)
		m.mu.Lock()
		m.hits++
		m.reqs = append(m.reqs, recordedReq{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
		m.mu.Unlock()
		if m.getFn != nil {
			m.getFn(w, r, body)
			return
		}
		writeData(w, map[string]any{"name": "a.mp3", "size": 10, "raw_url": "http://x/a.mp3"})
	})
	return httptest.NewServer(mux)
}

func (m *apiMock) requestBodies() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, 0, len(m.reqs))
	for _, r := range m.reqs {
		out = append(out, r.body)
	}
	return out
}

func (m *apiMock) loginCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logins
}

func (m *apiMock) hitCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

func writeData(w http.ResponseWriter, data any) {
	writeEnvelope(w, 200, "success", data)
}

func writeEnvelope(w http.ResponseWriter, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message, "data": data})
}

func readBody(r *http.Request) map[string]any {
	raw, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

// newTestClient wires a Client to a mock URL with virtual time and no jitter
// (randFloat returns 0.5 ⇒ jitter factor exactly 1.0, making timings exact).
func newTestClient(t *testing.T, baseURL string, cfg Config) (*Client, *virtualClock) {
	t.Helper()
	cfg.BaseURL = baseURL
	if cfg.ListInterval == 0 {
		cfg.ListInterval = -1 // -1 disables list throttling (interval<=0 ⇒ no-op)
	}
	if cfg.GetInterval == 0 {
		cfg.GetInterval = -1
	}
	if cfg.JitterFraction == 0 {
		cfg.JitterFraction = 0.2
	}
	if cfg.RetryBaseDelay == 0 {
		cfg.RetryBaseDelay = 100 * time.Millisecond
	}
	c := NewClient(cfg)
	// NewClient maps 0 → default; restore the test's "disabled" semantics.
	if cfg.ListInterval < 0 {
		c.listLim.interval = 0
		c.authLim.interval = 0
	}
	if cfg.GetInterval < 0 {
		c.getLim.interval = 0
	}
	clk := newVirtualClock()
	c.now = clk.Now
	c.sleep = clk.Sleep
	c.randFloat = func() float64 { return 0.5 } // zero jitter ⇒ exact timings
	c.listLim.now = clk.Now
	c.listLim.randFloat = c.randFloat
	c.getLim.now = clk.Now
	c.getLim.randFloat = c.randFloat
	c.authLim.now = clk.Now
	c.authLim.randFloat = c.randFloat
	c.breaker.now = clk.Now
	return c, clk
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

func TestLoginCachesToken(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()

	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if r.Header.Get("Authorization") == "" {
			t.Error("list request missing Authorization header")
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	c, _ := newTestClient(t, srv.URL, Config{Username: "u", Password: "p"})
	for i := 0; i < 3; i++ {
		if _, err := c.List(context.Background(), "/", 1); err != nil {
			t.Fatalf("list %d: %v", i, err)
		}
	}
	if got := m.loginCount(); got != 1 {
		t.Fatalf("expected exactly 1 login (token cached), got %d", got)
	}
}

func TestLoginAcceptsBareStringToken(t *testing.T) {
	// Some AList v3 builds return `data` as the bare token string instead of
	// {"token": "..."}; both must work.
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.loginFn = func(w http.ResponseWriter, r *http.Request) {
		writeData(w, "raw-token-string")
	}
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if r.Header.Get("Authorization") != "raw-token-string" {
			t.Errorf("Authorization = %q, want raw-token-string", r.Header.Get("Authorization"))
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	c, _ := newTestClient(t, srv.URL, Config{Username: "u", Password: "p"})
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitTokenSkipsLogin(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if r.Header.Get("Authorization") != "my-token" {
			t.Errorf("Authorization = %q, want my-token", r.Header.Get("Authorization"))
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "my-token"})
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatal(err)
	}
	if got := m.loginCount(); got != 0 {
		t.Fatalf("explicit-token mode must not call login, got %d logins", got)
	}
}

func TestReloginOnceOn401(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if r.Header.Get("Authorization") == "tok-1" {
			// First token expires mid-session (HTTP-level 401).
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	c, _ := newTestClient(t, srv.URL, Config{Username: "u", Password: "p"})
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatalf("list should succeed after re-login: %v", err)
	}
	if got := m.loginCount(); got != 2 {
		t.Fatalf("expected 2 logins (initial + re-login on 401), got %d", got)
	}
}

func TestReloginOnEnvelope401(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if r.Header.Get("Authorization") == "tok-1" {
			writeEnvelope(w, 401, "token has expired", nil) // application-level 401
			return
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	c, _ := newTestClient(t, srv.URL, Config{Username: "u", Password: "p"})
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatalf("list should succeed after re-login: %v", err)
	}
	if got := m.loginCount(); got != 2 {
		t.Fatalf("expected 2 logins, got %d", got)
	}
}

func Test401WithoutCredentialsFailsFast(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "expired-token"})
	_, err := c.List(context.Background(), "/", 1)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if got := m.loginCount(); got != 0 {
		t.Fatalf("no credentials ⇒ no login attempts, got %d", got)
	}
}

func TestLoginErrorWhenNothingConfigured(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	c, _ := newTestClient(t, srv.URL, Config{})
	_, err := c.List(context.Background(), "/", 1)
	if !errors.Is(err, errNoCredentials) {
		t.Fatalf("want errNoCredentials, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List / Get
// ---------------------------------------------------------------------------

func TestListPagination(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		switch page {
		case 1:
			writeData(w, map[string]any{
				"content": []any{
					map[string]any{"name": "a.mp3", "size": 1, "is_dir": false, "modified": "2026-01-02T03:04:05Z", "thumb": "t1"},
					map[string]any{"name": "album", "size": 0, "is_dir": true, "modified": "2026-01-01T00:00:00Z"},
				},
				"total": 3,
			})
		case 2:
			writeData(w, map[string]any{"content": []any{map[string]any{"name": "b.mp3", "size": 2}}, "total": 3})
		default:
			writeData(w, map[string]any{"content": []any{}, "total": 3})
		}
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "t"})
	var all []Entry
	for page := 1; ; page++ {
		entries, err := c.List(context.Background(), "/music", page)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			break
		}
		all = append(all, entries...)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 entries across pages, got %d", len(all))
	}
	if all[0].Name != "a.mp3" || all[0].Size != 1 || all[0].IsDir || all[0].Thumb != "t1" {
		t.Errorf("entry[0] parsed wrong: %+v", all[0])
	}
	if !all[0].Modified.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("entry[0].Modified = %v", all[0].Modified)
	}
	if !all[1].IsDir {
		t.Errorf("entry[1] should be a dir: %+v", all[1])
	}

	bodies := m.requestBodies()
	if len(bodies) != 3 {
		t.Fatalf("want 3 list requests, got %d", len(bodies))
	}
	for i, b := range bodies {
		if b["path"] != "/music" {
			t.Errorf("request %d: path = %v", i, b["path"])
		}
		if b["refresh"] != false {
			t.Errorf("request %d: refresh should be false, got %v", i, b["refresh"])
		}
		if asFloat(b["page"]) != float64(i+1) {
			t.Errorf("request %d: page = %v, want %d", i, b["page"], i+1)
		}
	}
}

func TestGetFileInfo(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.getFn = func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		if body["path"] != "/music/a.mp3" {
			t.Errorf("get path = %v", body["path"])
		}
		writeData(w, map[string]any{
			"name":       "a.mp3",
			"size":       12345,
			"is_dir":     false,
			"modified":   "2026-02-03T04:05:06Z",
			"raw_url":    "https://drive.example/dl/a.mp3?sign=abc",
			"sign":       "abc",
			"thumb":      "https://drive.example/t/a.jpg",
			"expires_at": "2026-02-03T05:05:06Z",
		})
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "t"})
	fi, err := c.Get(context.Background(), "/music/a.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Name != "a.mp3" || fi.Size != 12345 || fi.IsDir {
		t.Errorf("FileInfo parsed wrong: %+v", fi)
	}
	if fi.RawURL != "https://drive.example/dl/a.mp3?sign=abc" {
		t.Errorf("RawURL = %q", fi.RawURL)
	}
	if fi.Sign != "abc" || fi.Thumb == "" {
		t.Errorf("sign/thumb not preserved: %+v", fi)
	}
	if !fi.ExpiresAt.Equal(time.Date(2026, 2, 3, 5, 5, 6, 0, time.UTC)) {
		t.Errorf("ExpiresAt = %v, want 2026-02-03T05:05:06Z", fi.ExpiresAt)
	}
}

func TestGetWithoutExpiryField(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.getFn = func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		writeData(w, map[string]any{"name": "a.mp3", "size": 1, "raw_url": "http://x/a"})
	}
	c, _ := newTestClient(t, srv.URL, Config{Token: "t"})
	fi, err := c.Get(context.Background(), "/a.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt should be zero when absent, got %v", fi.ExpiresAt)
	}
}

func TestGetExpiryUnixTimestamp(t *testing.T) {
	fi := &FileInfo{}
	if err := json.Unmarshal([]byte(`{"name":"a","raw_url":"u","expired":1767225600}`), fi); err != nil {
		t.Fatal(err)
	}
	if !fi.ExpiresAt.Equal(time.Unix(1767225600, 0)) {
		t.Errorf("ExpiresAt = %v", fi.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// Error codes / retry / breaker
// ---------------------------------------------------------------------------

func TestAPIErrorCode(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		writeEnvelope(w, 500, "object not found", nil)
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "t"})
	_, err := c.List(context.Background(), "/", 1)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Code != 500 || apiErr.Message != "object not found" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if got := m.hitCount(); got != 1 {
		t.Errorf("application-level errors must not be retried, got %d hits", got)
	}
}

func TestRetryOn429And5xxWithBackoff(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	statuses := []int{429, 500, 200}
	calls := 0
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		st := statuses[calls]
		calls++
		if st != 200 {
			w.WriteHeader(st)
			return
		}
		writeData(w, map[string]any{"content": []any{map[string]any{"name": "ok"}}, "total": 1})
	}

	c, clk := newTestClient(t, srv.URL, Config{Token: "t", RetryBaseDelay: 100 * time.Millisecond})
	entries, err := c.List(context.Background(), "/", 1)
	if err != nil {
		t.Fatalf("should succeed after retries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	sleeps := clk.positive()
	if len(sleeps) != 2 {
		t.Fatalf("want 2 backoff sleeps, got %v", sleeps)
	}
	if sleeps[0] != 100*time.Millisecond {
		t.Errorf("first backoff = %v, want 100ms", sleeps[0])
	}
	if sleeps[1] != 200*time.Millisecond {
		t.Errorf("second backoff = %v, want 200ms (exponential)", sleeps[1])
	}
}

func TestRetryGivesUpAfterMax(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	c, _ := newTestClient(t, srv.URL, Config{Token: "t", MaxRetries: 3})
	_, err := c.List(context.Background(), "/", 1)
	if err == nil {
		t.Fatal("want error after exhausted retries")
	}
	// 1 initial attempt + 3 retries = 4 hits; then the breaker counts 1 failure.
	if got := m.hitCount(); got != 4 {
		t.Errorf("want 4 hits (1+3 retries), got %d", got)
	}
}

func TestBreakerOpensAndRecovers(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	fail := true
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}

	cfg := Config{Token: "t", MaxRetries: -1, FailureThreshold: 5, CircuitOpenFor: 30 * time.Minute}
	c, clk := newTestClient(t, srv.URL, cfg)
	c.maxRetries = 0 // each request = exactly 1 failure (no retries), keeps hit math simple

	// 5 consecutive failures ⇒ breaker opens (default threshold).
	for i := 0; i < 5; i++ {
		if _, err := c.List(context.Background(), "/", 1); err == nil {
			t.Fatalf("request %d should fail", i)
		}
	}
	hitsBefore := m.hitCount()

	// While open: fast-fail without touching the network.
	_, err := c.List(context.Background(), "/", 1)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen, got %v", err)
	}
	if m.hitCount() != hitsBefore {
		t.Fatalf("open circuit must not hit the server")
	}

	// After the cool-down the breaker goes half-open; upstream recovered.
	fail = false
	clk.Advance(31 * time.Minute)
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatalf("half-open trial should succeed: %v", err)
	}
	// Fully closed again: the next request goes through normally.
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatalf("closed breaker should pass: %v", err)
	}
	if err := c.breaker.allow(); err != nil {
		t.Errorf("breaker should be closed after recovery: %v", err)
	}
}

func TestBreakerReopensOnFailedTrial(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		w.WriteHeader(http.StatusBadGateway)
	}
	cfg := Config{Token: "t", FailureThreshold: 5, CircuitOpenFor: 30 * time.Minute}
	c, clk := newTestClient(t, srv.URL, cfg)
	c.maxRetries = 0

	for i := 0; i < 5; i++ {
		_, _ = c.List(context.Background(), "/", 1)
	}
	clk.Advance(31 * time.Minute) // half-open now
	if _, err := c.List(context.Background(), "/", 1); err == nil {
		t.Fatal("trial request should fail")
	}
	// Failed trial re-opens the circuit immediately (before another 30min).
	_, err := c.List(context.Background(), "/", 1)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen after failed trial, got %v", err)
	}
}

func TestSuccessResetsFailureStreak(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()
	fail := true
	m.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}
	cfg := Config{Token: "t", FailureThreshold: 3}
	c, _ := newTestClient(t, srv.URL, cfg)
	c.maxRetries = 0

	_, _ = c.List(context.Background(), "/", 1)
	_, _ = c.List(context.Background(), "/", 1)
	fail = false
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatal(err)
	}
	// Streak reset: 2 more failures must NOT open the breaker.
	fail = true
	_, _ = c.List(context.Background(), "/", 1)
	_, _ = c.List(context.Background(), "/", 1)
	if c.breaker.isOpen() {
		t.Error("breaker must not open: failure streak was reset by a success")
	}
}

// ---------------------------------------------------------------------------
// Throttling
// ---------------------------------------------------------------------------

func TestListThrottlingWithJitter(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()

	for _, tc := range []struct {
		name      string
		rnd       float64
		wantSleep time.Duration
	}{
		{"lower bound (-20%)", 0.0, 1600 * time.Millisecond},
		{"no jitter", 0.5, 2000 * time.Millisecond},
		{"upper bound (+20%)", 1.0, 2400 * time.Millisecond},
	} {
		// Fresh client per case so the limiter slot state starts clean.
		c, clk := newTestClient(t, srv.URL, Config{Token: "t", ListInterval: 2 * time.Second})
		c.randFloat = func() float64 { return tc.rnd }
		c.listLim.randFloat = c.randFloat

		if _, err := c.List(context.Background(), "/", 1); err != nil { // first call: no wait
			t.Fatal(err)
		}
		if _, err := c.List(context.Background(), "/", 2); err != nil { // second: throttled
			t.Fatal(err)
		}
		sleeps := clk.recorded()
		if len(sleeps) != 2 || sleeps[0] != 0 {
			t.Errorf("%s: sleeps = %v, want [0, %v]", tc.name, sleeps, tc.wantSleep)
		}
		if sleeps[1] != tc.wantSleep {
			t.Errorf("%s: second request waited %v, want %v", tc.name, sleeps[1], tc.wantSleep)
		}
	}
}

func TestGetAndListLimitersAreIndependent(t *testing.T) {
	m := newAPIMock(t)
	srv := m.server()
	defer srv.Close()

	c, clk := newTestClient(t, srv.URL, Config{Token: "t", ListInterval: 2 * time.Second, GetInterval: 5 * time.Second})
	if _, err := c.List(context.Background(), "/", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.List(context.Background(), "/", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/a.mp3"); err != nil {
		t.Fatal(err)
	}
	sleeps := clk.recorded()
	if len(sleeps) != 3 {
		t.Fatalf("sleeps = %v", sleeps)
	}
	if sleeps[1] != 2*time.Second {
		t.Errorf("2nd list wait = %v, want 2s", sleeps[1])
	}
	if sleeps[2] != 0 {
		t.Errorf("get must not be throttled by the list limiter, waited %v", sleeps[2])
	}
	// A second get IS throttled by the get limiter (5s).
	if _, err := c.Get(context.Background(), "/a.mp3"); err != nil {
		t.Fatal(err)
	}
	if sleeps = clk.recorded(); sleeps[3] != 5*time.Second {
		t.Errorf("2nd get wait = %v, want 5s", sleeps[3])
	}
}

// ---------------------------------------------------------------------------
// RangeFetcher
// ---------------------------------------------------------------------------

func TestRangeFetcherReadsExactSlice(t *testing.T) {
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	var gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusPartialContent)
		// Naive Range implementation good enough for the test.
		var start, end int
		if _, err := fmt.Sscanf(gotRange, "bytes=%d-%d", &start, &end); err != nil {
			t.Errorf("bad Range header %q", gotRange)
			return
		}
		_, _ = w.Write(content[start : end+1])
	}))
	defer srv.Close()

	f := NewRangeFetcher(RangeFetcherOptions{MinInterval: -1})
	f.lim.interval = 0
	clk := newVirtualClock()
	f.sleep = clk.Sleep

	buf, err := f.FetchRange(context.Background(), srv.URL+"/f.bin", 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "56789abcde" {
		t.Errorf("got %q, want %q", buf, "56789abcde")
	}
	if gotRange != "bytes=5-14" {
		t.Errorf("Range header = %q, want bytes=5-14", gotRange)
	}
}

func TestRangeFetcherFallsBackToFullBody(t *testing.T) {
	content := []byte("hello world!!")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content) // 200, Range ignored
	}))
	defer srv.Close()

	f := NewRangeFetcher(RangeFetcherOptions{})
	f.lim.interval = 0
	clk := newVirtualClock()
	f.sleep = clk.Sleep

	buf, err := f.FetchRange(context.Background(), srv.URL, 6, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Errorf("got %q, want %q", buf, "world")
	}

	// Out-of-bounds request errors instead of returning garbage.
	if _, err := f.FetchRange(context.Background(), srv.URL, 10, 10); err == nil {
		t.Error("out-of-bounds range should error")
	}
}

func TestRangeFetcherRetriesOn429(t *testing.T) {
	content := []byte("abcdefghij")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(content[2:5])
	}))
	defer srv.Close()

	f := NewRangeFetcher(RangeFetcherOptions{RetryBaseDelay: 50 * time.Millisecond})
	f.lim.interval = 0
	clk := newVirtualClock()
	f.sleep = clk.Sleep
	f.randFloat = func() float64 { return 0.5 } // zero jitter ⇒ exact backoff
	f.lim.randFloat = f.randFloat

	buf, err := f.FetchRange(context.Background(), srv.URL, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "cde" {
		t.Errorf("got %q, want %q", buf, "cde")
	}
	sleeps := clk.positive()
	if len(sleeps) != 2 {
		t.Fatalf("want 2 backoff sleeps, got %v", sleeps)
	}
	if sleeps[0] != 50*time.Millisecond || sleeps[1] != 100*time.Millisecond {
		t.Errorf("backoff = %v, want [50ms 100ms]", sleeps)
	}
}

func TestRangeFetcherThrottles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	f := NewRangeFetcher(RangeFetcherOptions{MinInterval: 2 * time.Second})
	clk := newVirtualClock()
	f.sleep = clk.Sleep
	f.randFloat = func() float64 { return 0.5 } // zero jitter ⇒ exact spacing
	f.lim.now = clk.Now
	f.lim.randFloat = f.randFloat

	if _, err := f.FetchRange(context.Background(), srv.URL, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchRange(context.Background(), srv.URL, 0, 1); err != nil {
		t.Fatal(err)
	}
	sleeps := clk.recorded()
	if len(sleeps) != 2 || sleeps[0] != 0 || sleeps[1] != 2*time.Second {
		t.Errorf("sleeps = %v, want [0 2s]", sleeps)
	}
}

func TestRangeFetcherRejectsBadArgs(t *testing.T) {
	f := NewRangeFetcher(RangeFetcherOptions{})
	if _, err := f.FetchRange(context.Background(), "http://x", 0, 0); err == nil {
		t.Error("length=0 should error")
	}
	if _, err := f.FetchRange(context.Background(), "http://x", -1, 5); err == nil {
		t.Error("negative offset should error")
	}
}

func TestDefaultConfig(t *testing.T) {
	c := NewClient(Config{})
	if c.listLim.interval != 2*time.Second {
		t.Errorf("default list interval = %v, want 2s", c.listLim.interval)
	}
	if c.getLim.interval != 2*time.Second {
		t.Errorf("default get interval = %v, want 2s", c.getLim.interval)
	}
	if c.jitterFrac != 0.2 {
		t.Errorf("default jitter = %v, want 0.2", c.jitterFrac)
	}
	if c.maxRetries != 3 {
		t.Errorf("default max retries = %d, want 3", c.maxRetries)
	}
	if c.breaker.threshold != 5 {
		t.Errorf("default failure threshold = %d, want 5", c.breaker.threshold)
	}
	if c.breaker.openFor != 30*time.Minute {
		t.Errorf("default circuit open window = %v, want 30m", c.breaker.openFor)
	}
	// Hard cap on retries, per spec.
	c2 := NewClient(Config{MaxRetries: 10})
	if c2.maxRetries != 3 {
		t.Errorf("MaxRetries must be capped at 3, got %d", c2.maxRetries)
	}
}
