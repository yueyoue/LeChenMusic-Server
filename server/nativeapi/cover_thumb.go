package nativeapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/navidrome/navidrome/core/artwork"
)

// maxCoverDimension caps the longest side of every cover / avatar served by the native
// API. 640px is comfortably above any thumbnail the web UI or the app draws, while the
// payload drops from 100KB–1.5MB to roughly 25KB.
//
// Why this matters beyond bandwidth: the web UI loads a whole shelf of covers at once
// (an audiobook category page fires one request per book). Originals eat the browser's
// ~6 per-host connection slots, every JSON endpoint then queues behind the images, and
// the whole admin UI appears to hang on "加载中". See artwork.ThumbnailImage.
const maxCoverDimension = 640

// serveCoverBytes writes cover bytes to the response, downscaling oversized originals on
// the way out. Small images pass through untouched (no needless re-encode).
func serveCoverBytes(w http.ResponseWriter, r *http.Request, name string, modTime time.Time, data []byte) {
	out, contentType := artwork.ThumbnailImage(data, name, maxCoverDimension)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", contentType)
	etag := coverETag(out)
	w.Header().Set("ETag", etag)
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, name, modTime, bytes.NewReader(out))
}

// coverETag derives a strong validator from the exact bytes served. The APP / browser sends it
// back as If-None-Match and gets a cheap 304 instead of a re-download; http.ServeContent already
// layers If-Modified-Since / Range handling on top of this.
func coverETag(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// noCover writes a 404 the client is allowed to cache. Without Cache-Control the APP and browser
// re-request every missing cover on every draw — exactly what made cover loads crawl. A moderate
// TTL stops the hammering while still letting a cover added later show up soon after.
func noCover(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=600")
	http.Error(w, "No cover found", http.StatusNotFound)
}
