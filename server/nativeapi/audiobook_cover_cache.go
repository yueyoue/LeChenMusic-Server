package nativeapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/audiobookcover"
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

// coverCachePrefix versions the cache keys. It changed when the cache started storing
// thumbnails instead of the originals: bumping it makes the pre-thumbnail entries
// unreachable, so they age out of the LRU instead of being served at full size forever.
const coverCachePrefix = "abcover-thumb-"

// embeddedProbeTimeout caps how long a single embedded-cover probe may spend reading the book's
// audio tags from the storage backend. On a slow cloud gateway an unbounded probe was one of the
// things that made cover loads crawl; a hard ceiling keeps requests moving.
const embeddedProbeTimeout = 10 * time.Second

// noCoverSentinel is stored in the cache to mean "this book definitively has no embedded cover".
// It is not a valid image, so it can never collide with real cover bytes.
var noCoverSentinel = []byte("__LECHEN_NOCOVER__")

func isNoCoverSentinel(b []byte) bool { return bytes.Equal(b, noCoverSentinel) }

// coverCacheItem implements cache.Item for a single audiobook cover image.
type coverCacheItem struct {
	keyStr string

	libPath    string // storage URI of the library (empty when fetching from coverURL)
	relCover   string // library-relative cover path
	coverURL   string // remote image URL (the scraped CoverUrl fallback)
	embeddedDir string // library-relative book folder to probe for embedded art (fallback)
}

func (i *coverCacheItem) Key() string { return i.keyStr }

// coverCacheKey builds the key for a library cover. The source fingerprint (mtime+size)
// is part of the key, so a replaced cover file gets a fresh cache entry instead of a
// stale hit; the old entry ages out via the LRU.
func coverCacheKey(libPath, relCover string, mtime time.Time, size int64) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "lib\x00%s\x00%s\x00%d\x00%d", libPath, relCover, mtime.UnixMilli(), size))
	return coverCachePrefix + hex.EncodeToString(sum[:])
}

// coverURLCacheKey builds the key for a proxied CoverUrl image. The day bucket gives
// remote images a ~24h TTL, so a changed remote image is picked up without any explicit
// invalidation (and 304s still work within the day).
func coverURLCacheKey(rawURL string) string {
	bucket := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	sum := sha256.Sum256(fmt.Appendf(nil, "url\x00%s\x00%d", rawURL, bucket))
	return coverCachePrefix + hex.EncodeToString(sum[:])
}

// embeddedCoverCacheKey builds the key for a book's embedded-cover resolution. It is stable per
// book (no timestamp), so the probe result — including a confident "no cover" — is reused across
// requests and the audio is never re-probed on every draw. A manual cover upload takes the local
// override path (read fresh) and a replaced library cover file has its own fingerprint-keyed
// entry, so both still refresh automatically; only an in-place embedded-art swap waits for the LRU
// to age the entry out.
func embeddedCoverCacheKey(libPath, bookPath string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "emb\x00%s\x00%s", libPath, bookPath))
	return coverCachePrefix + hex.EncodeToString(sum[:])
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
		// Store the thumbnail, not the original: the cache is what ends up on the wire.
		out, _ := artwork.ThumbnailImage(data, "cover.jpg", maxCoverDimension)
		return bytes.NewReader(out), nil
	}

	// Embedded-art fallback: probe the book's audio for an embedded cover. Bounded by a timeout so
	// a slow cloud gateway can't hang the request; the result (including a confident "no embedded
	// art") is cached so repeat loads never re-probe the audio files again.
	if ci.embeddedDir != "" {
		tctx, cancel := context.WithTimeout(ctx, embeddedProbeTimeout)
		defer cancel()
		fsys, ferr := storage.FSFor(tctx, ci.libPath)
		if ferr != nil {
			return nil, ferr
		}
		data, ext, err := audiobookcover.EmbeddedCover(fsys, ci.embeddedDir)
		if err != nil {
			if errors.Is(err, audiobookcover.ErrNoEmbeddedCover) {
				return bytes.NewReader(noCoverSentinel), nil // confident negative -> cache it
			}
			return nil, err // transient read/timeout: not cached, retried next time
		}
		out, _ := artwork.ThumbnailImage(data, "cover"+ext, maxCoverDimension)
		return bytes.NewReader(out), nil
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
	// Store the thumbnail, not the original: the cache is what ends up on the wire.
	out, _ := artwork.ThumbnailImage(data, ci.relCover, maxCoverDimension)
	return bytes.NewReader(out), nil
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

// serveCachedCover streams a cover through the given disk cache. The cache holds
// thumbnails (readCoverForCache downscales on fill), and serveCoverBytes re-checks on the
// way out so entries written before that still get shrunk instead of going out at full
// size. Range / If-Modified-Since / 304 all keep working via http.ServeContent.
func serveCachedCover(w http.ResponseWriter, r *http.Request, cc cache.FileCache, item *coverCacheItem, name string, modTime time.Time) {
	stream, err := cc.Get(r.Context(), item)
	if err != nil {
		log.Debug(r.Context(), "[cloud] cover cache read failed", "key", item.Key(), err)
		http.Error(w, "No cover found", 404)
		return
	}
	defer func() { _ = stream.Close() }()

	data, err := io.ReadAll(io.LimitReader(stream.Reader, maxCachedCoverBytes+1))
	if err != nil || len(data) > maxCachedCoverBytes {
		log.Debug(r.Context(), "[cloud] cover cache read failed", "key", item.Key(), err)
		http.Error(w, "No cover found", 404)
		return
	}
	serveCoverBytes(w, r, name, modTime, data)
}

// resolveCachedCover fetches a cover through the given disk cache and reports whether it resolved
// to "none" (a cached negative). A positive result returns the image bytes; a cache/IO error is
// returned as err so the caller can decide whether to fall through to another cover source.
func resolveCachedCover(ctx context.Context, cc cache.FileCache, item cache.Item) (data []byte, none bool, err error) {
	stream, err := cc.Get(ctx, item)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = stream.Close() }()
	data, err = io.ReadAll(io.LimitReader(stream.Reader, maxCachedCoverBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxCachedCoverBytes {
		return nil, false, fmt.Errorf("audiobook cover cache: entry for %s too large", item.Key())
	}
	if isNoCoverSentinel(data) {
		return nil, true, nil
	}
	return data, false, nil
}
