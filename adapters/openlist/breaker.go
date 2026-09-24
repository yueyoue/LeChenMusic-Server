package openlist

import (
	"sync"
	"time"
)

// circuitBreaker opens after `threshold` consecutive failed requests, causing
// fast-fail (ErrCircuitOpen) for `openFor` (default 30min). After the cool-down
// it goes half-open: the next request is a trial — success closes the breaker,
// failure re-opens it for another window.
//
// Why: a broken/misconfigured upstream should not be hammered for the whole
// duration of a scan; failing fast keeps the rest of Navidrome healthy and
// lowers the risk of drive-side bans.
type circuitBreaker struct {
	mu        sync.Mutex
	failures  int
	threshold int
	openFor   time.Duration
	openUntil time.Time // zero = closed
	now       func() time.Time
}

func (b *circuitBreaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return nil
	}
	now := b.now()
	if now.Before(b.openUntil) {
		return ErrCircuitOpen
	}
	// Half-open: let one request through as a trial. If it fails, onFailure()
	// re-opens the breaker for a fresh window.
	return nil
}

func (b *circuitBreaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openUntil = time.Time{}
}

func (b *circuitBreaker) onFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if !b.openUntil.IsZero() && !now.Before(b.openUntil) {
		// Half-open trial failed: re-open for a full window.
		b.openUntil = now.Add(b.openFor)
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = now.Add(b.openFor)
	}
}

// isOpen reports whether requests are currently fast-failing (for tests/observability).
func (b *circuitBreaker) isOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.openUntil.IsZero() && b.now().Before(b.openUntil)
}
