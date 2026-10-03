package artwork

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif" // register decoder for image.Decode (png/jpeg/webp come from artwork.go)
	"image/jpeg"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// maxThumbnailBytes is the payload size above which an image is re-encoded even when its
// pixel dimensions already fit: a 640px cover that is still 500KB is exactly the problem
// this file exists to solve.
const maxThumbnailBytes = 256 << 10

// ThumbnailImage downscales an image so its longest side is at most maxDim pixels and
// re-encodes it as JPEG.
//
// It backs the "load a lot of covers at once" surfaces — audiobook lists, narrator and
// artist avatars. Serving the originals there (typically 100–500KB, up to 1.5MB for
// MusicTag hi-res / embedded art) fills the browser's ~6 connection slots, and then every
// JSON API call queues behind the images: whole pages sit on "加载中" and the admin UI
// feels frozen. A thumbnail is ~25KB, so the same list loads an order of magnitude faster.
//
// A payload that is already small enough — in pixels and in bytes — is returned untouched,
// so normal covers never take a needless quality-losing re-encode. Anything undecodable is
// returned as-is too: a broken cover must not turn into a 404.
//
// The second return value is the MIME type of the returned bytes.
func ThumbnailImage(data []byte, name string, maxDim int) ([]byte, string) {
	if maxDim <= 0 || len(data) == 0 {
		return data, ImageMime(data, name)
	}
	original, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, ImageMime(data, name)
	}
	bounds := original.Bounds()
	if max(bounds.Dx(), bounds.Dy()) <= maxDim && len(data) <= maxThumbnailBytes {
		return data, ImageMime(data, name)
	}

	// Aspect-fit, never upscale.
	scale := float64(maxDim) / float64(max(bounds.Dx(), bounds.Dy()))
	dstW := max(int(float64(bounds.Dx())*scale+0.5), 1)
	dstH := max(int(float64(bounds.Dy())*scale+0.5), 1)

	// Flatten onto white first: covers are photos, and JPEG has no alpha, so a
	// transparent PNG would otherwise come back with a black background.
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	original = toFastScaleType(original)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), original, bounds, draw.Over, nil)

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	err = jpeg.Encode(buf, dst, &jpeg.Options{Quality: 78})
	if err != nil {
		bufPool.Put(buf)
		return data, ImageMime(data, name)
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	bufPool.Put(buf)
	return out, "image/jpeg"
}

// ImageMime gives the best-effort MIME type for image bytes, falling back to the file name
// extension and finally to JPEG. Thumbnails are always JPEG, so this is only needed for
// the pass-through cases above.
func ImageMime(data []byte, name string) string {
	if ct := http.DetectContentType(data); strings.HasPrefix(ct, "image/") {
		return ct
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); strings.HasPrefix(ct, "image/") {
		return ct
	}
	return "image/jpeg"
}
