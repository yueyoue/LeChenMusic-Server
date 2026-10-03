package nativeapi

import (
	"bytes"
	"net/http"
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
	http.ServeContent(w, r, name, modTime, bytes.NewReader(out))
}
