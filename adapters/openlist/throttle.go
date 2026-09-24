package openlist

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// limiter spaces out requests to the same class of endpoints. It is a
// "min-interval with jitter" limiter rather than a classic token bucket: the
// effect is identical for our 1-request-per-interval budget (capacity 1 bucket
// refilling at 1/interval), but the implementation stays trivial and the jitter
// is trivially testable.
//
// The ±jitter on the interval is a hard requirement (风控规避): a perfectly
// periodic request pattern is trivially detectable by drive-side anti-bot rules.
type limiter struct {
	mu         sync.Mutex
	interval   time.Duration // minimum spacing between two requests
	jitterFrac float64       // e.g. 0.2 => spacing ∈ [0.8*interval, 1.2*interval]
	last       time.Time     // reserved time of the previous request (zero = none)
	now        func() time.Time
	randFloat  func() float64 // returns a value in [0,1); injectable for tests
}

// reserve books the next request slot and returns how long the caller must wait
// before firing the request. Waiting is delegated to the caller (via Client.sleep)
// so tests can fake the clock.
func (l *limiter) reserve() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	var wait time.Duration
	if !l.last.IsZero() && l.interval > 0 {
		jitter := 1 + l.jitterFrac*(2*l.randFloat()-1) // 1 ± jitterFrac
		next := l.last.Add(time.Duration(float64(l.interval) * jitter))
		if next.After(now) {
			wait = next.Sub(now)
		}
	}
	l.last = now.Add(wait)
	return wait
}

// defaultSleep waits for d (respecting ctx cancellation). It is a field on
// Client/RangeFetcher so tests can substitute a virtual clock.
func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// jittered applies the same ±frac jitter to a backoff delay.
func jittered(d time.Duration, frac float64, randFloat func() float64) time.Duration {
	if d <= 0 {
		return d
	}
	jitter := 1 + frac*(2*randFloat()-1)
	return time.Duration(float64(d) * jitter)
}

func defaultRandFloat() float64 {
	// rand.Float64 is safe for concurrent use.
	return rand.Float64()
}
