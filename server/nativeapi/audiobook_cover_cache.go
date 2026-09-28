package nativeapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/cache"
)

// Disk cache for cloud/proxied audiobook covers (评审 §4.14 P2-1, 设计文档 §10.3⑥).
//
// Without it every cover request hit the drive again: a cloud cover cost one /api/fs/get
// (signed URL) plus a CDN round trip, and a scraped CoverUrl was re-downloaded from the
// internet on every single request. Covers are small, so caching them on disk is a clear
// win: repeated list/detail loads become local reads and no longer depend on the gateway
// being reachable.
//
// Scope: cloud library covers + CoverUrl proxy images. Local library covers already live
// on local disk and keep their direct ServeFile path (no copy, unchanged behavior).
// Audio is intentionally NOT cached here — the 302-direct-link design (media bytes never
// pass the NAS) stays untouched.

// maxCachedCoverBytes caps a single cached cover (defensive: a cover is an image, never
// hundreds of MB).
const maxCachedCoverBytes = 10 << 20

// coverCacheFolder is the LRU cache subfolder under the global cache dir (data/cache).
const coverCacheFolder = "audiobook-covers"

// coverCacheItem implements cache.Item for a single audiobook cover image.
type coverCacheItem struct {
	keyStr string

	libPath  string // storage URI of the library (empty when fetching from coverURL)
	relCover string // library-relative cover path
	coverURL string // remote image URL (the scraped CoverUrl fallback)
}

func (i *coverCacheItem) Key() string { return i.keyStr }

// coverCacheKey builds the key for a library cover. The source fingerprint (mtime+size)
// is part of the key, so a replaced cover file gets a fresh cache entry instead of a
// stale hit; the old entry ages out via the LRU.
func coverCacheKey(libPath, relCover string, mtime time.Time, size int64) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "lib\x00%s\x00%s\x00%d\x00%d", libPath, relCover, mtime.UnixMilli(), size))
	return "abcover-" + hex.EncodeToString(sum[:])
}

// coverURLCacheKey builds the key for a proxied CoverUrl image. The day bucket gives
// remote images a ~24h TTL, so a changed remote image is picked up without any explicit
// invalidation (and 304s still work within the day).
func coverURLCacheKey(rawURL string) string {
	bucket := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	sum := sha256.Sum256(fmt.Appendf(nil, "url\x00%s\x00%d", rawURL, bucket))
	return "abcover-" + hex.EncodeToString(sum[:])
}

// readCoverForCache fetches cover bytes on a cache miss. Library covers are read through
// the storage abstraction (a lazy, bounded Range read on a cloud source); remote images
// go through the SSRF-guarded fetcher.
func readCoverForCache(ctx context.Context, item cache.Item) (io.Reader, error) {
	ci, ok := item.(*coverCacheItem)
	if !ok {
		return nil, fmt.Errorf("audiobook cover cache: unexpected item type %T", item)
	}
	if ci.coverURL != "" {
		data, _, err := fetchRemoteImage(ctx, ci.coverURL)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}

	fsys, err := storage.FSFor(ctx, ci.libPath)
	if err != nil {
		return nil, err
	}
	f, err := fsys.Open(ci.relCover)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxCachedCoverBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCachedCoverBytes {
		return nil, fmt.Errorf("audiobook cover %q exceeds %d bytes", ci.relCover, maxCachedCoverBytes)
	}
	return bytes.NewReader(data), nil
}

var (
	coverCacheOnce sync.Once
	coverCacheInst cache.FileCache
)

// getCoverCache returns the process-wide audiobook cover cache. The cache size reuses the
// image cache setting; the LRU budget is separate from the album-art cache (different
// folder), so tuning ImageCacheSize scales both predictably.
func getCoverCache() cache.FileCache {
	coverCacheOnce.Do(func() {
		coverCacheInst = cache.NewFileCache("AudiobookCover", conf.Server.ImageCacheSize, coverCacheFolder,
			0 /*unlimited items*/, readCoverForCache)
	})
	return coverCacheInst
}

// serveCachedCover streams a cover through the given disk cache. On a cache hit the
// stream is seekable, so http.ServeContent gives Range + If-Modified-Since/304 support;
// while the first fill is still streaming we fall back to a plain copy.
func serveCachedCover(w http.ResponseWriter, r *http.Request, cc cache.FileCache, item *coverCacheItem, name string, modTime time.Time) {
	stream, err := cc.Get(r.Context(), item)
	if err != nil {
		log.Debug(r.Context(), "[cloud] cover cache read failed", "key", item.Key(), err)
		http.Error(w, "No cover found", 404)
		return
	}
	defer func() { _ = stream.Close() }()

	w.Header().Set("Cache-Control", "public, max-age=3600")
	if rs, ok := stream.Reader.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, modTime, rs)
		return
	}
	if ext := filepath.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	_, _ = io.Copy(w, stream.Reader)
}
