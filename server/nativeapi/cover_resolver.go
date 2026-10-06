package nativeapi

import (
	"context"
	"sync"
	"time"
)

// Scheduling for audiobook cover resolution.
//
// Resolving a cover is the one place an audiobook request reaches out to the storage gateway, and on
// a cloud source that is slow by construction: the openlist client paces its API calls at roughly one
// per two seconds (anti-bot jitter), so even a well-optimised resolve is a couple of seconds and an
// unlucky one runs for tens. Doing that inside the request is what wrecked the admin UI — a shelf
// fires one cover request per book, a plain-HTTP origin gets about six browser connections, so slow
// covers starved every JSON endpoint queued behind them. Clicking into a book then sat on
// "Loading..." for ten seconds while its own query took 30ms.
//
// So covers are walked by background jobs and the request path only ever answers from the cache:
//
//   - cache hit            -> served straight away. This is the steady state: walks also pre-fill the
//     cache in the background, so a shelf that has been seen once is pure local disk reads.
//   - walk in progress     -> "no cover" immediately; the client retries and picks the cover up.
//   - key parked           -> "no cover" immediately, without starting another walk. A book the
//     gateway cannot answer must not cost a walk per request, forever.
//   - cold                 -> start exactly one walk and give it coverServeBudget to land in the
//     cache. That budget only has to cover a local cache read, so restarting the server does not
//     cost a round of broken images.
//
// Concurrent requests for the same book share a single walk, and at most coverWalkConcurrency walks
// run at once so a shelf of forty covers cannot stampede the gateway (which would serialise them
// anyway and only slow everything else down).

const (
	// coverServeBudget is how long a request waits for a walk that may still be answerable from the
	// disk cache. It deliberately does NOT cover a real resolve: a request must never hold a browser
	// connection for the seconds a gateway walk needs.
	coverServeBudget = 250 * time.Millisecond

	// coverRetryDelay parks a book after its walk failed or timed out. Without it, a book the gateway
	// cannot answer costs a full walk on every single request — forever.
	coverRetryDelay = 2 * time.Minute

	// coverNoCoverTTL is how long a confidently "this book has no cover" answer may be cached.
	coverNoCoverTTL = 10 * time.Minute

	// coverWalkConcurrency bounds simultaneous walks. More buys nothing: the gateway client paces
	// its own API calls, extra walkers only queue behind each other.
	coverWalkConcurrency = 3

	// coverPendingRetry is what a client is told while a walk is still running.
	coverPendingRetry = 3 * time.Second
)

// coverWalk is one shared background resolution of a single book cover.
type coverWalk struct {
	done chan struct{}
	data []byte
	none bool
	err  error
}

// coverResolver runs cover walks: one per book at a time, bounded in number, and with a memory of
// the books that recently failed so a broken source degrades to instant 404s instead of a storm of
// slow ones.
type coverResolver struct {
	mu      sync.Mutex
	walking map[string]*coverWalk
	parked  map[string]time.Time
	sem     chan struct{}
}

// coverResolverInst is process-wide: one gateway, one budget, one book walked once.
var coverResolverInst = newCoverResolver(coverWalkConcurrency)

func newCoverResolver(concurrency int) *coverResolver {
	return &coverResolver{
		walking: map[string]*coverWalk{},
		parked:  map[string]time.Time{},
		sem:     make(chan struct{}, concurrency),
	}
}

// coverResult is the answer the request path writes out. When ready is false the walk is still going
// (or the key is parked) and the caller answers "no cover", caching it for retryIn.
type coverResult struct {
	data    []byte
	none    bool
	err     error
	ready   bool
	retryIn time.Duration
}

// resolve returns the cover for key, starting a shared background walk when the cache cannot answer
// it. It never blocks longer than budget.
func (r *coverResolver) resolve(ctx context.Context, key string, budget time.Duration, walk coverWalkFunc) coverResult {
	if res, parked := r.checkParked(key); parked {
		return res
	}
	w := r.start(key, walk)

	if budget <= 0 {
		return coverResult{retryIn: coverPendingRetry}
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-w.done:
		return coverResult{
			data: w.data, none: w.none, err: w.err, ready: true,
			retryIn: retryAfter(w),
		}
	case <-ctx.Done():
		return coverResult{retryIn: coverPendingRetry}
	case <-timer.C:
		// Still walking. Answer now and let it finish for the next request.
		return coverResult{retryIn: coverPendingRetry}
	}
}

// walkAndWait runs the walk for key and waits for it. Used by the background pre-warm, where
// blocking is fine and sharing the walk with live requests is the whole point.
func (r *coverResolver) walkAndWait(key string, walk coverWalkFunc) {
	<-r.start(key, walk).done
}

type coverWalkFunc func() (data []byte, none bool, err error)

// checkParked reports a recent failure so the caller can answer immediately instead of queueing
// another walk. Expired entries are dropped here so the map cannot grow with every book the gateway
// ever failed on.
func (r *coverResolver) checkParked(key string) (coverResult, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for k, until := range r.parked {
		if !now.Before(until) {
			delete(r.parked, k)
		}
	}
	until, hit := r.parked[key]
	if !hit {
		return coverResult{}, false
	}
	return coverResult{none: true, retryIn: time.Until(until)}, true
}

// start joins the walk already running for key, or starts one.
func (r *coverResolver) start(key string, walk coverWalkFunc) *coverWalk {
	r.mu.Lock()
	w, running := r.walking[key]
	if !running {
		w = &coverWalk{done: make(chan struct{})}
		r.walking[key] = w
		go r.run(key, w, walk)
	}
	r.mu.Unlock()
	return w
}

// run performs one walk, remembers failures so they are not retried immediately, and always releases
// its slot before waking the waiters.
func (r *coverResolver) run(key string, w *coverWalk, walk coverWalkFunc) {
	defer close(w.done)
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	w.data, w.none, w.err = walk()

	r.mu.Lock()
	delete(r.walking, key)
	if w.err != nil {
		r.parked[key] = time.Now().Add(coverRetryDelay)
	}
	r.mu.Unlock()
}

// retryAfter is how long a client may cache a "no cover" answer for this walk outcome.
func retryAfter(w *coverWalk) time.Duration {
	switch {
	case w.err != nil:
		return coverRetryDelay
	case w.none:
		return coverNoCoverTTL
	default:
		return coverPendingRetry
	}
}
