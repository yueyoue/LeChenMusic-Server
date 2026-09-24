package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/utils/cache"
	"github.com/navidrome/navidrome/utils/req"
)

type MediaStreamer interface {
	NewStream(ctx context.Context, mf *model.MediaFile, req Request) (*Stream, error)
}

type TranscodingCache cache.FileCache

func NewMediaStreamer(ds model.DataStore, t ffmpeg.FFmpeg, cache TranscodingCache) MediaStreamer {
	return &mediaStreamer{
		ds:         ds,
		transcoder: t,
		cache:      cache,
		limiter:    NewTranscodeLimiter(conf.Server.Transcoding.MaxConcurrent, conf.Server.Transcoding.MaxConcurrentPerUser),
	}
}

type mediaStreamer struct {
	ds         model.DataStore
	transcoder ffmpeg.FFmpeg
	cache      cache.FileCache
	limiter    TranscodeLimiter
}

type streamJob struct {
	ms         *mediaStreamer
	mf         *model.MediaFile
	filePath   string
	format     string
	bitRate    int
	sampleRate int
	bitDepth   int
	channels   int
	offset     int
}

func (j *streamJob) Key() string {
	return fmt.Sprintf("%s.%s.%d.%d.%d.%d.%s.%d", j.mf.ID, j.mf.UpdatedAt.Format(time.RFC3339Nano), j.bitRate, j.sampleRate, j.bitDepth, j.channels, j.format, j.offset)
}

// NewStream creates a Stream for the given MediaFile and Request. It handles both raw streaming (no transcoding)
// and transcoded streaming based on the requested format and bitrate. It also logs detailed information about
// the streaming request and whether the transcoding result was served from cache or not.
func (ms *mediaStreamer) NewStream(ctx context.Context, mf *model.MediaFile, req Request) (*Stream, error) {
	var format string
	var bitRate int
	var cached bool
	defer func() {
		log.Info(ctx, "Streaming file", "title", mf.Title, "artist", mf.Artist, "format", format, "cached", cached,
			"bitRate", bitRate, "sampleRate", req.SampleRate, "bitDepth", req.BitDepth, "channels", req.Channels,
			"user", userName(ctx), "transcoding", format != "raw",
			"originalFormat", mf.Suffix, "originalBitRate", mf.BitRate)
	}()

	format = req.Format
	bitRate = req.BitRate
	if format == "" || format == "raw" {
		format = "raw"
		bitRate = 0
	}
	filePath := mf.AbsolutePath()

	// A cloud library has no local path for ffmpeg to open: hand ffmpeg the file's direct
	// link instead, so it pulls straight from the drive's CDN (the media bytes still never
	// pass through this server). With no usable link, fall back to relaying the original
	// bytes rather than failing playback entirely.
	if format != "raw" {
		if _, remote := ms.remoteStorage(ctx, mf); remote {
			raw, ok := ms.directURL(ctx, mf)
			if !ok {
				log.Warn(ctx, "stream: cannot transcode cloud file without a direct link, serving original", "id", mf.ID)
				format, bitRate = "raw", 0
			} else {
				filePath = raw
			}
		}
	}

	s := &Stream{ctx: ctx, mf: mf, format: format, bitRate: bitRate}

	if format == "raw" {
		log.Debug(ctx, "Streaming RAW file", "id", mf.ID, "path", filePath,
			"requestBitrate", req.BitRate, "requestFormat", req.Format, "requestOffset", req.Offset,
			"originalBitrate", mf.BitRate, "originalFormat", mf.Suffix,
			"selectedBitrate", bitRate, "selectedFormat", format)

		// Cloud media sources can hand the client a direct link to the origin (302), so
		// the media bytes never pass through this server (方案 B: 索引 + 302 直链).
		if raw, ok := ms.directURL(ctx, mf); ok {
			s.redirectURL = raw
			s.format = mf.Suffix
			return s, nil
		}

		f, err := ms.openRaw(ctx, mf, filePath)
		if err != nil {
			return nil, err
		}
		s.ReadCloser = f
		s.Seeker = f
		s.format = mf.Suffix
		return s, nil
	}

	job := &streamJob{
		ms:         ms,
		mf:         mf,
		filePath:   filePath,
		format:     format,
		bitRate:    bitRate,
		sampleRate: req.SampleRate,
		bitDepth:   req.BitDepth,
		channels:   req.Channels,
		offset:     req.Offset,
	}
	r, err := ms.cache.Get(ctx, job)
	if err != nil {
		// Rate-limit rejections are already logged at warn level by the
		// producer; treating them as cache failures here would both
		// double-log and mask actual cache problems.
		if !errors.Is(err, ErrTooManyTranscodes) {
			log.Error(ctx, "Error accessing transcoding cache", "id", mf.ID, err)
		}
		return nil, err
	}
	cached = r.Cached

	s.ReadCloser = r
	s.Seeker = r.Seeker

	log.Debug(ctx, "Streaming TRANSCODED file", "id", mf.ID, "path", filePath,
		"requestBitrate", req.BitRate, "requestFormat", req.Format, "requestOffset", req.Offset,
		"originalBitrate", mf.BitRate, "originalFormat", mf.Suffix,
		"selectedBitrate", bitRate, "selectedFormat", format, "cached", cached, "seekable", s.Seekable())

	return s, nil
}

type Stream struct {
	ctx     context.Context
	mf      *model.MediaFile
	bitRate int
	format  string
	// redirectURL, when set, is a signed direct link to the file's origin (a cloud
	// drive CDN). The stream then answers HTTP 302 instead of relaying bytes, so media
	// data never passes through this server. It is a credential: never log it.
	redirectURL string
	io.ReadCloser
	io.Seeker
}

func (s *Stream) Seekable() bool      { return s.Seeker != nil }
func (s *Stream) Redirected() bool    { return s.redirectURL != "" }
func (s *Stream) Duration() float32   { return s.mf.Duration }
func (s *Stream) ContentType() string { return mime.TypeByExtension("." + s.format) }
func (s *Stream) Name() string        { return s.mf.Title + "." + s.format }
func (s *Stream) ModTime() time.Time  { return s.mf.UpdatedAt }
func (s *Stream) EstimatedContentLength() int {
	return int(s.mf.Duration * float32(s.bitRate) / 8 * 1024)
}

// Close releases the underlying reader. A redirected stream has no reader at all, so
// the embedded io.ReadCloser is nil there and must not be dereferenced.
func (s *Stream) Close() error {
	if s.ReadCloser == nil {
		return nil
	}
	return s.ReadCloser.Close()
}

// Serve writes the stream to the HTTP response. For seekable streams it uses http.ServeContent
// (supporting range requests). For non-seekable streams it writes directly and logs any errors.
// Returns the number of bytes written and an error only when io.Copy fails with 0 bytes written
// (meaning the HTTP 200 status has not been flushed yet and the caller can still send an error response).
// Empty output (0 bytes, no error) is logged but not treated as an error.
func (s *Stream) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request) (int64, error) {
	if s.Redirected() {
		// 302 straight to the drive's CDN. We deliberately forward none of our own
		// headers (especially not any auth material) to the CDN, and the signed URL is
		// never logged. Clients follow the redirect with their normal HTTP stack.
		log.Debug(ctx, "Streaming via 302 direct link", "id", s.mf.ID, "title", s.mf.Title)
		http.Redirect(w, r, s.redirectURL, http.StatusFound)
		return 0, nil
	}

	if s.Seekable() {
		http.ServeContent(w, r, s.Name(), s.ModTime(), s)
		return -1, nil
	}

	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Content-Type", s.ContentType())

	if req.Params(r).BoolOr("estimateContentLength", false) {
		length := strconv.Itoa(s.EstimatedContentLength())
		log.Trace(ctx, "Estimated content-length", "contentLength", length)
		w.Header().Set("Content-Length", length)
	}

	if r.Method == http.MethodHead {
		go func() { _, _ = io.Copy(io.Discard, s) }()
		return 0, nil
	}

	id := s.mf.ID
	c, err := io.Copy(w, s)
	if err != nil {
		log.Error(ctx, "Error sending transcoded file", "id", id, err)
		if c == 0 {
			w.Header().Del("Content-Length")
			return 0, fmt.Errorf("sending transcoded file: %w", err)
		}
		return c, nil
	}
	if c == 0 {
		log.Error(ctx, "Transcoding returned empty output, ffmpeg may have failed. "+
			"Check that ffmpeg supports the requested codec. Enable Trace logging for ffmpeg stderr details",
			"id", id, "format", s.ContentType())
	} else {
		log.Trace(ctx, "Success sending transcoded file", "id", id, "size", c)
	}
	return c, nil
}

// NewStream creates a non-seekable Stream from the given components.
func NewStream(mf *model.MediaFile, format string, bitRate int, r io.ReadCloser) *Stream {
	return &Stream{
		ctx:        context.Background(),
		mf:         mf,
		format:     format,
		bitRate:    bitRate,
		ReadCloser: r,
	}
}

// readSeekCloser is what raw streaming needs from a file handle: it is handed to
// http.ServeContent, which seeks for range requests.
type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// isRemotePath reports whether a library path points at a remote (cloud) source rather
// than at the local filesystem.
func isRemotePath(libraryPath string) bool {
	return strings.Contains(libraryPath, "://") && !strings.HasPrefix(libraryPath, storage.LocalSchemaID+"://")
}

// remoteStorage resolves the storage backend of mf's library when it is a cloud source.
func (ms *mediaStreamer) remoteStorage(ctx context.Context, mf *model.MediaFile) (storage.Storage, bool) {
	libPath := mf.LibraryPath
	if libPath == "" {
		lib, err := ms.ds.Library(ctx).Get(mf.LibraryID)
		if err != nil {
			log.Warn(ctx, "stream: cannot resolve library for media file", "id", mf.ID, "library", mf.LibraryID, err)
			return nil, false
		}
		libPath = lib.Path
	}
	if !isRemotePath(libPath) {
		return nil, false
	}
	st, err := storage.For(libPath)
	if err != nil {
		log.Warn(ctx, "stream: cannot resolve cloud storage for library", "library", mf.LibraryID, "path", libPath, err)
		return nil, false
	}
	return st, true
}

// directURL resolves a fresh, signed direct link for cloud sources (the 302 playback
// mode). Links expire in hours at most, so one is resolved per playback and never cached.
// The second return value is false whenever the 302 mode cannot be used and the caller
// must fall back to relaying the bytes through this server.
func (ms *mediaStreamer) directURL(ctx context.Context, mf *model.MediaFile) (string, bool) {
	st, ok := ms.remoteStorage(ctx, mf)
	if !ok {
		return "", false
	}
	dlp, ok := st.(storage.DirectLinkProvider)
	if !ok {
		return "", false
	}
	raw, _, err := dlp.DirectURL(ctx, mf.Path)
	if err != nil {
		// Missing/expired direct link, IP-bound link the client could not use, or the
		// gateway refused: fall back to the server-side relay (中转流), exactly as
		// required by the design doc §2.2. The URL itself must never be logged.
		log.Debug(ctx, "stream: no direct link available, relaying through server", "id", mf.ID, err)
		return "", false
	}
	return raw, true
}

// openRaw opens mf for raw streaming. Local libraries keep hitting the OS filesystem
// directly (unchanged behaviour); cloud libraries go through their storage backend,
// which yields a lazy, seekable reader — the 中转流 fallback of the design doc.
func (ms *mediaStreamer) openRaw(ctx context.Context, mf *model.MediaFile, filePath string) (readSeekCloser, error) {
	st, ok := ms.remoteStorage(ctx, mf)
	if !ok {
		return os.Open(filePath) //nolint:gosec
	}

	var fsys storage.MusicFS
	var err error
	if cs, ok := st.(storage.ContextualStorage); ok {
		fsys, err = cs.FSWithContext(ctx)
	} else {
		fsys, err = st.FS()
	}
	if err != nil {
		return nil, fmt.Errorf("opening cloud library: %w", err)
	}

	f, err := fsys.Open(mf.Path)
	if err != nil {
		return nil, err
	}
	rsc, ok := f.(readSeekCloser)
	if !ok {
		_ = f.Close()
		return nil, fmt.Errorf("stream: %s is not seekable", mf.Path)
	}
	return rsc, nil
}

var (
	onceTranscodingCache     sync.Once
	instanceTranscodingCache TranscodingCache
)

func GetTranscodingCache() TranscodingCache {
	onceTranscodingCache.Do(func() {
		instanceTranscodingCache = NewTranscodingCache()
	})
	return instanceTranscodingCache
}

func NewTranscodingCache() TranscodingCache {
	return cache.NewFileCache("Transcoding", conf.Server.TranscodingCacheSize,
		consts.TranscodingCacheDir, consts.DefaultTranscodingCacheMaxItems,
		func(ctx context.Context, arg cache.Item) (io.Reader, error) {
			job := arg.(*streamJob)
			command := LookupTranscodeCommand(ctx, job.ms.ds, job.format)
			if command == "" {
				log.Error(ctx, "No transcoding command available", "format", job.format)
				return nil, os.ErrInvalid
			}

			release, err := job.ms.limiter.Acquire(ctx, limiterKey(ctx))
			if err != nil {
				log.Warn(ctx, "Refusing transcode: concurrent transcode limit reached",
					"id", job.mf.ID, "user", userName(ctx),
					"maxConcurrent", conf.Server.Transcoding.MaxConcurrent,
					"maxPerUser", conf.Server.Transcoding.MaxConcurrentPerUser)
				return nil, err
			}

			// Choose the context that drives the ffmpeg process.
			//
			// When the limiter is enabled, force the request context so a
			// client disconnect cancels ffmpeg and frees the slot promptly.
			// Otherwise a client could open many transcodes, disconnect
			// immediately, and still leave the configured cap's worth of
			// ffmpeg processes draining in the background — which is exactly
			// the DoS the limiter is meant to prevent.
			//
			// When the limiter is disabled, preserve the legacy behavior
			// governed by Transcoding.EnableCancellation so unchanged configs
			// keep their previous observable behavior.
			var transcodingCtx context.Context
			if job.ms.limiter.Enabled() || conf.Server.Transcoding.EnableCancellation {
				transcodingCtx = ctx
			} else {
				transcodingCtx = request.AddValues(context.Background(), ctx)
			}

			out, err := job.ms.transcoder.Transcode(transcodingCtx, ffmpeg.TranscodeOptions{
				Command:    command,
				Format:     job.format,
				FilePath:   job.filePath,
				BitRate:    job.bitRate,
				SampleRate: job.sampleRate,
				BitDepth:   job.bitDepth,
				Channels:   job.channels,
				Offset:     job.offset,
			})
			if err != nil {
				release()
				log.Error(ctx, "Error starting transcoder", "id", job.mf.ID, err)
				return nil, os.ErrInvalid
			}
			// Tie the slot to the ffmpeg process: copyAndClose calls Close
			// on this reader after io.Copy returns, which is exactly when
			// ffmpeg has exited (either EOF or context cancellation).
			return &releasingReadCloser{ReadCloser: out, release: release}, nil
		})
}

// userName extracts the username from the context for logging purposes.
func userName(ctx context.Context) string {
	if user, ok := request.UserFrom(ctx); !ok {
		return "UNKNOWN"
	} else {
		return user.UserName
	}
}

// limiterKey returns the per-user bucket key used by the transcode limiter.
// For anonymous requests (e.g. public shares) it returns the empty string,
// which signals the limiter to skip the per-user cap entirely — otherwise
// every anonymous viewer of a public share would collide on the same key
// and starve each other within MaxConcurrentPerUser slots. The global cap
// still applies and remains the protection against runaway anonymous load.
func limiterKey(ctx context.Context) string {
	if user, ok := request.UserFrom(ctx); ok {
		return user.UserName
	}
	return ""
}
