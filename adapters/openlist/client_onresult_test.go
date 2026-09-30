package openlist

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 评审 P2-10: every round trip must be reported to Config.OnResult exactly once.

type recordedResult struct {
	op      string
	status  int
	err     error
	elapsed time.Duration
}

type resultRecorder struct {
	mu      sync.Mutex
	results []recordedResult
}

func (r *resultRecorder) hook(op string, status int, err error, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, recordedResult{op: op, status: status, err: err, elapsed: elapsed})
}

func (r *resultRecorder) all() []recordedResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedResult(nil), r.results...)
}

func TestOnResultHookReportsSuccess(t *testing.T) {
	mock := newAPIMock(t)
	mock.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}
	srv := mock.server()
	defer srv.Close()

	rec := &resultRecorder{}
	c, _ := newTestClient(t, srv.URL, Config{Token: "tok", OnResult: rec.hook})

	_, err := c.List(context.Background(), "/music", 1)
	assert.NoError(t, err)

	got := rec.all()
	assert.Len(t, got, 1)
	assert.Equal(t, endpointList, got[0].op)
	assert.Equal(t, 200, got[0].status)
	assert.NoError(t, got[0].err)
	assert.GreaterOrEqual(t, got[0].elapsed, time.Duration(0))
}

func TestOnResultHookReportsHTTPError(t *testing.T) {
	mock := newAPIMock(t)
	mock.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable → single attempt
	}
	srv := mock.server()
	defer srv.Close()

	rec := &resultRecorder{}
	c, _ := newTestClient(t, srv.URL, Config{Token: "tok", OnResult: rec.hook})

	_, err := c.List(context.Background(), "/music", 1)
	assert.Error(t, err)

	got := rec.all()
	assert.Len(t, got, 1)
	assert.Equal(t, 400, got[0].status)
	assert.Error(t, got[0].err)
}

func TestOnResultHookReportsTransportError(t *testing.T) {
	mock := newAPIMock(t)
	srv := mock.server()
	url := srv.URL
	srv.Close() // connection refused → transport error (retryable: 1 + 1 retry)

	rec := &resultRecorder{}
	c, _ := newTestClient(t, url, Config{Token: "tok", MaxRetries: 1, OnResult: rec.hook})

	_, err := c.List(context.Background(), "/music", 1)
	assert.Error(t, err)

	got := rec.all()
	assert.Len(t, got, 2) // one report per attempt
	for _, r := range got {
		assert.Equal(t, 0, r.status) // 0 == transport error (no HTTP status)
		assert.Error(t, r.err)
	}
}

func TestOnResultHookCountsEveryRetryAttempt(t *testing.T) {
	mock := newAPIMock(t)
	mock.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		w.WriteHeader(http.StatusTooManyRequests) // retryable
	}
	srv := mock.server()
	defer srv.Close()

	rec := &resultRecorder{}
	c, _ := newTestClient(t, srv.URL, Config{Token: "tok", MaxRetries: 1, OnResult: rec.hook})

	_, err := c.List(context.Background(), "/music", 1)
	assert.Error(t, err)

	got := rec.all()
	assert.Len(t, got, 2) // 1 attempt + 1 retry, one report each
	for _, r := range got {
		assert.Equal(t, 429, r.status)
	}
}

func TestOnResultHookIsOptional(t *testing.T) {
	mock := newAPIMock(t)
	mock.listFn = func(w http.ResponseWriter, r *http.Request, page int) {
		writeData(w, map[string]any{"content": []any{}, "total": 0})
	}
	srv := mock.server()
	defer srv.Close()

	c, _ := newTestClient(t, srv.URL, Config{Token: "tok"}) // no hook
	_, err := c.List(context.Background(), "/music", 1)
	assert.NoError(t, err) // must not panic with a nil hook
}
