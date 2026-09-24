package openlist

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RangeFetcher reads a byte range from a file's RawURL via HTTP Range requests.
// It is used by the lazy tag/cover readers of the "cloud media source" feature:
// instead of downloading a whole remote file, only the [offset, offset+length)
// bytes needed by the tag parser are fetched.
//
// Like Client, every request is throttled (default ≤1 req/2s with ±20% jitter)
// because these raw URLs still hit the drive's edge nodes (风控).
type RangeFetcher struct {
	httpClient     *http.Client
	lim            *limiter
	maxRetries     int
	retryBaseDelay time.Duration
	jitterFrac     float64

	// Test hooks, same rationale as Client's.
	sleep     func(ctx context.Context, d time.Duration) error
	randFloat func() float64
}

// RangeFetcherOptions configures a RangeFetcher; the zero value gets defaults.
type RangeFetcherOptions struct {
	HTTPClient     *http.Client  // optional, defaults to http.DefaultClient
	MinInterval    time.Duration // spacing between range reads (default 2s)
	JitterFraction float64       // ± fraction (default 0.2)
	MaxRetries     int           // retries on 429/5xx (default 3, hard cap 3)
	RetryBaseDelay time.Duration // exponential backoff base (default 500ms)
}

// NewRangeFetcher builds a RangeFetcher from opts.
func NewRangeFetcher(opts RangeFetcherOptions) *RangeFetcher {
	if opts.MinInterval == 0 {
		opts.MinInterval = defaultListInterval
	}
	if opts.JitterFraction == 0 {
		opts.JitterFraction = defaultJitterFraction
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.MaxRetries > defaultMaxRetries {
		opts.MaxRetries = defaultMaxRetries
	}
	if opts.RetryBaseDelay == 0 {
		opts.RetryBaseDelay = defaultRetryBaseDelay
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &RangeFetcher{
		httpClient:     hc,
		lim:            &limiter{interval: opts.MinInterval, jitterFrac: opts.JitterFraction, now: time.Now, randFloat: defaultRandFloat},
		maxRetries:     opts.MaxRetries,
		retryBaseDelay: opts.RetryBaseDelay,
		jitterFrac:     opts.JitterFraction,
		sleep:          defaultSleep,
		randFloat:      defaultRandFloat,
	}
}

// FetchRange downloads exactly the [offset, offset+length) bytes of the file at
// rawURL (a FileInfo.RawURL). length must be > 0.
//
// Servers honoring RFC 7233 answer 206 with just the requested slice. If a
// server answers 200 (Range ignored) the slice is cut locally, so correctness
// never depends on Range support.
func (f *RangeFetcher) FetchRange(ctx context.Context, rawURL string, offset, length int64) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("openlist: range fetch: length must be > 0, got %d", length)
	}
	if offset < 0 {
		return nil, fmt.Errorf("openlist: range fetch: offset must be >= 0, got %d", offset)
	}

	retries := 0
	for {
		if err := f.sleep(ctx, f.lim.reserve()); err != nil {
			return nil, fmt.Errorf("openlist: range fetch canceled while throttling: %w", err)
		}
		buf, retryable, err := f.fetchOnce(ctx, rawURL, offset, length)
		if err == nil {
			return buf, nil
		}
		if !retryable || retries >= f.maxRetries {
			return nil, err
		}
		retries++
		backoff := jittered(f.retryBaseDelay<<uint(retries-1), f.jitterFrac, f.randFloat)
		if serr := f.sleep(ctx, backoff); serr != nil {
			return nil, fmt.Errorf("openlist: range fetch canceled while backing off: %w", serr)
		}
	}
}

func (f *RangeFetcher) fetchOnce(ctx context.Context, rawURL string, offset, length int64) (buf []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("openlist: range fetch: create request: %w", err)
	}
	end := offset + length - 1
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("openlist: range fetch %s failed: %w", rawURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent: // 206: exactly the slice we asked for
		buf, err = io.ReadAll(io.LimitReader(resp.Body, length))
		if err != nil {
			return nil, true, fmt.Errorf("openlist: range fetch: read body: %w", err)
		}
		if int64(len(buf)) != length {
			return nil, true, fmt.Errorf("openlist: range fetch: short read: want %d bytes, got %d", length, len(buf))
		}
		return buf, false, nil
	case http.StatusOK: // 200: server ignored Range, cut the slice locally
		all, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, true, fmt.Errorf("openlist: range fetch: read body: %w", err)
		}
		if end >= int64(len(all)) {
			return nil, false, fmt.Errorf("openlist: range fetch: range [%d,%d) out of bounds (body %d bytes)", offset, offset+length, len(all))
		}
		return all[offset : offset+length], false, nil
	case http.StatusTooManyRequests, http.StatusRequestTimeout:
		return nil, true, fmt.Errorf("openlist: range fetch %s: server returned status %d", rawURL, resp.StatusCode)
	default:
		if resp.StatusCode >= 500 {
			return nil, true, fmt.Errorf("openlist: range fetch %s: server returned status %d", rawURL, resp.StatusCode)
		}
		return nil, false, fmt.Errorf("openlist: range fetch %s: unexpected status %d", rawURL, resp.StatusCode)
	}
}

// String makes log lines about a RangeFetcher readable.
func (f *RangeFetcher) String() string {
	return fmt.Sprintf("RangeFetcher{interval=%s}", f.lim.interval)
}
