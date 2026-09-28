package nativeapi

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage/storagetest"
	"github.com/navidrome/navidrome/tests"
	"github.com/navidrome/navidrome/utils/cache"
)

// Regression tests for 评审 §4.14 P2-1: cloud/remote audiobook covers are served through
// a local disk cache, so repeat loads don't touch the gateway (or the internet) again.

func waitCoverCacheReady(t *testing.T, cc cache.FileCache) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cc.Available(context.Background()) || cc.Disabled(context.Background()) {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("cover cache did not become ready in time")
}

// countingCache wraps a FileCache and counts how often the backing fetcher runs.
func newCountingCoverCache(t *testing.T, counter *atomic.Int64, fetch cache.ReadFunc) cache.FileCache {
	t.Helper()
	cc := cache.NewFileCache("TestAudiobookCover", "10MB", "test-audiobook-covers", 0,
		func(ctx context.Context, item cache.Item) (io.Reader, error) {
			counter.Add(1)
			return fetch(ctx, item)
		})
	waitCoverCacheReady(t, cc)
	if cc.Disabled(context.Background()) {
		t.Fatal("cover cache unexpectedly disabled")
	}
	return cc
}

func TestCoverCacheKeyChangesWithSourceFingerprint(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	a := coverCacheKey("openlist://h/p", "三体/cover.jpg", ts, 100)
	if a != coverCacheKey("openlist://h/p", "三体/cover.jpg", ts, 100) {
		t.Fatal("same source must produce a stable cache key")
	}
	if a == coverCacheKey("openlist://h/p", "三体/cover.jpg", ts.Add(time.Second), 100) {
		t.Fatal("changed mtime must produce a new cache key")
	}
	if a == coverCacheKey("openlist://h/p", "三体/cover.jpg", ts, 101) {
		t.Fatal("changed size must produce a new cache key")
	}
	if a == coverCacheKey("openlist://h/other", "三体/cover.jpg", ts, 100) {
		t.Fatal("different library must produce a different cache key")
	}
	if !strings.HasPrefix(a, "abcover-") {
		t.Fatalf("cache key must be namespaced, got %q", a)
	}
}

func TestCoverURLCacheKeyIsStableWithinTheTestRun(t *testing.T) {
	k := coverURLCacheKey("https://example.com/a.jpg")
	if k != coverURLCacheKey("https://example.com/a.jpg") {
		t.Fatal("same URL must produce a stable key within a day bucket")
	}
	if k == coverURLCacheKey("https://example.com/b.jpg") {
		t.Fatal("different URL must produce a different key")
	}
}

func TestReadCoverForCacheFromLibrary(t *testing.T) {
	tests.Init(t, false)
	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(fstest.MapFS{
		"三体/cover.jpg": {Data: []byte("jpeg-bytes"), ModTime: time.Now()},
	})
	storagetest.Register("covercache", ffs)

	item := &coverCacheItem{
		libPath:  "covercache:///fnos/有声书",
		relCover: "三体/cover.jpg",
	}
	r, err := readCoverForCache(context.Background(), item)
	if err != nil {
		t.Fatalf("readCoverForCache: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "jpeg-bytes" {
		t.Fatalf("unexpected cover bytes %q", data)
	}
}

func TestReadCoverForCacheRejectsOversized(t *testing.T) {
	tests.Init(t, false)
	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(fstest.MapFS{
		"big/cover.jpg": {Data: []byte(strings.Repeat("x", maxCachedCoverBytes+1)), ModTime: time.Now()},
	})
	storagetest.Register("covercachefat", ffs)

	item := &coverCacheItem{libPath: "covercachefat:///x", relCover: "big/cover.jpg"}
	if _, err := readCoverForCache(context.Background(), item); err == nil {
		t.Fatal("oversized cover must be rejected")
	}
}

// uniqueCoverTestKey returns a per-run cache key. The cover cache lives on disk and
// survives across test runs, so a hardcoded key would be a HIT on the second run and the
// "fetched once" assertion would see zero fetches.
func uniqueCoverTestKey(prefix string) string {
	return prefix + time.Now().Format("150405.000000000")
}

func TestServeCachedCoverFetchesOnce(t *testing.T) {
	tests.Init(t, false)
	conf.Server.ImageCacheSize = "10MB"

	var fetches atomic.Int64
	cc := newCountingCoverCache(t, &fetches, func(_ context.Context, _ cache.Item) (io.Reader, error) {
		return strings.NewReader("cover-bytes"), nil
	})

	item := &coverCacheItem{keyStr: uniqueCoverTestKey("abcover-test-once-"), coverURL: "https://example.com/a.jpg"}
	serve := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/audiobook/b/cover", nil)
		w := httptest.NewRecorder()
		serveCachedCover(w, req, cc, item, "cover.jpg", time.Time{})
		return w
	}

	w := serve()
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cover-bytes") {
		t.Fatalf("first serve failed: code=%d body=%q", w.Code, w.Body.String())
	}

	// The cache fills asynchronously; wait until the second serve is a hit.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		serve()
		if fetches.Load() == 1 {
			break
		}
		runtime.Gosched()
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("cover must be fetched exactly once (cache hit afterwards), got %d fetches", got)
	}
}

func TestServeCachedCoverNotFound(t *testing.T) {
	tests.Init(t, false)
	conf.Server.ImageCacheSize = "10MB"

	var fetches atomic.Int64
	cc := newCountingCoverCache(t, &fetches, func(_ context.Context, _ cache.Item) (io.Reader, error) {
		return nil, os.ErrNotExist
	})

	req := httptest.NewRequest("GET", "/audiobook/b/cover", nil)
	w := httptest.NewRecorder()
	serveCachedCover(w, req, cc, &coverCacheItem{keyStr: uniqueCoverTestKey("abcover-test-missing-")}, "cover.jpg", time.Time{})
	if w.Code != 404 {
		t.Fatalf("failed fetch must 404, got %d", w.Code)
	}
}

func TestCoverUsesCacheForCloudLibrary(t *testing.T) {
	tests.Init(t, false)
	conf.Server.ImageCacheSize = "10MB"

	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(fstest.MapFS{
		"三体/cover.jpg": {Data: []byte("cloud-cover"), ModTime: time.Now()},
	})
	storagetest.Register("covercachee2e", ffs)

	h, _, _ := setupCoverFixture(t, "covercachee2e:///fnos/有声书")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "book-2")
	req := httptest.NewRequest("GET", "/audiobook/book-2/cover", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.cover(w, req)

	if w.Code != 200 {
		t.Fatalf("cloud cover must be served, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cloud-cover") {
		t.Fatalf("unexpected cover body %q", w.Body.String())
	}
}
