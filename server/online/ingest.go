package online

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// LibraryMatcher reports whether the library already contains a song.
// Used for the dedup gate (exact match blocks download unless the user
// opted into fuzzy matching).
type LibraryMatcher interface {
	Exists(ctx context.Context, title, artist string) (bool, error)
}

// MatcherFunc adapts a plain function to LibraryMatcher.
type MatcherFunc func(ctx context.Context, title, artist string) (bool, error)

// Exists implements LibraryMatcher.
func (f MatcherFunc) Exists(ctx context.Context, title, artist string) (bool, error) {
	return f(ctx, title, artist)
}

var (
	bracketedSuffix = regexp.MustCompile(`[（(][^（()）]*[)）]$`)
	illegalFileChar = regexp.MustCompile(`[\\/:*?"<>|]`)
	multiSpace      = regexp.MustCompile(`\s+`)
)

// NormalizeSongKey produces a comparison key for dedup matching:
// lowercase, full-width → half-width, whitespace/punctuation collapsed,
// and a trailing bracketed note (e.g. "（Live）") stripped.
func NormalizeSongKey(title, artist string) string {
	normalize := func(s string) string {
		s = strings.TrimSpace(strings.ToLower(s))
		s = bracketedSuffix.ReplaceAllString(s, "")
		var b strings.Builder
		for _, r := range s {
			switch {
			case r > 0xFF00 && r < 0xFF5F: // full-width ASCII block
				b.WriteRune(r - 0xFEE0)
			case unicode.IsSpace(r):
				continue
			case unicode.IsPunct(r) || unicode.IsSymbol(r):
				continue
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return normalize(title) + "|" + normalize(artist)
}

// SanitizeFilename removes characters that are unsafe in file/dir names.
func SanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = illegalFileChar.ReplaceAllString(s, "_")
	s = multiSpace.ReplaceAllString(s, " ")
	s = strings.Trim(s, " .")
	return s
}

// Ingestor implements PostProcessor: it finalizes a downloaded song by
// writing embedded metadata (info/cover/lyrics) and moving the file into the
// media library layout: <libRoot>/<Artist>/<Album>/<Title>.<ext>
type Ingestor struct {
	libRoot     string
	lyrics      Source // optional lyrics provider (usually the download source)
	matcher     LibraryMatcher
	coverClient *http.Client
	onIngested  func(ctx context.Context, finalPath string) // optional, e.g. trigger library scan
}

// NewIngestor creates an ingest PostProcessor. lyricsSource and matcher may be nil.
func NewIngestor(libRoot string, lyricsSource Source, matcher LibraryMatcher) *Ingestor {
	return &Ingestor{
		libRoot:     libRoot,
		lyrics:      lyricsSource,
		matcher:     matcher,
		coverClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// OnIngested registers a callback invoked after a successful ingest
// (used to trigger the media library scan).
func (ing *Ingestor) OnIngested(fn func(ctx context.Context, finalPath string)) {
	ing.onIngested = fn
}

// Exists exposes the dedup check (exact normalized match against the library).
func (ing *Ingestor) Exists(ctx context.Context, title, artist string) (bool, error) {
	if ing.matcher == nil {
		return false, nil
	}
	return ing.matcher.Exists(ctx, title, artist)
}

// Process implements PostProcessor.
func (ing *Ingestor) Process(ctx context.Context, task *model.DownloadTask, tmpPath string) (string, error) {
	ext := formatExt(task.ActualFormat, tmpPath)

	// Give the temp file its real extension before tagging (tag writers dispatch on it).
	stagePath := strings.TrimSuffix(tmpPath, filepath.Ext(tmpPath)) + ext
	if stagePath != tmpPath {
		if err := os.Rename(tmpPath, stagePath); err != nil {
			return "", fmt.Errorf("ingest: stage rename: %w", err)
		}
	}
	cleanupStage := func() { _ = os.Remove(stagePath) }

	meta := EmbeddedMeta{
		Title:  CleanSongTitle(task.SongName),
		Artist: NormalizeArtist(task.Artist),
		Album:  task.AlbumName,
	}
	if meta.Album == "" {
		meta.Album = "Unknown Album"
	}
	if task.CoverURL != "" {
		cover, mime, err := ing.fetchCover(ctx, task.CoverURL)
		if err != nil {
			log.Warn(ctx, "Ingest: could not fetch cover, continuing without it", "url", task.CoverURL, err)
		} else {
			meta.Cover = cover
			meta.CoverMIME = mime
		}
	}
	if ing.lyrics != nil {
		if lyrics, err := ing.lyrics.GetLyrics(ctx, taskRef(task)); err == nil {
			meta.Lyrics = lyrics
		} else {
			log.Warn(ctx, "Ingest: could not fetch lyrics, continuing without them", "task", task.ID, err)
		}
	}

	if err := WriteEmbeddedMetadata(stagePath, meta); err != nil {
		cleanupStage()
		return "", fmt.Errorf("ingest: write tags: %w", err)
	}

	finalPath, err := ing.targetPath(task, ext)
	if err != nil {
		cleanupStage()
		return "", err
	}
	if err := moveFile(stagePath, finalPath); err != nil {
		cleanupStage()
		return "", fmt.Errorf("ingest: move into library: %w", err)
	}

	if ing.onIngested != nil {
		ing.onIngested(ctx, finalPath)
	}
	return finalPath, nil
}

// targetPath computes <libRoot>/<Artist>/<Album>/<Title>.<ext>, avoiding collisions.
func (ing *Ingestor) targetPath(task *model.DownloadTask, ext string) (string, error) {
	artist := SanitizeFilename(NormalizeArtist(task.Artist))
	if artist == "" {
		artist = "Unknown Artist"
	}
	album := SanitizeFilename(task.AlbumName)
	if album == "" {
		album = "Unknown Album"
	}
	title := SanitizeFilename(CleanSongTitle(task.SongName))
	if title == "" {
		title = task.ID
	}

	dir := filepath.Join(ing.libRoot, artist, album)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ingest: create dir: %w", err)
	}

	candidate := filepath.Join(dir, title+ext)
	for n := 2; ; n++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", title, n, ext))
	}
}

// formatExt maps the actual download format to a file extension.
func formatExt(format, fallbackPath string) string {
	switch strings.ToLower(format) {
	case "flac":
		return ".flac"
	case "mp3":
		return ".mp3"
	}
	if ext := strings.ToLower(filepath.Ext(fallbackPath)); ext != "" && ext != ".part" {
		return ext
	}
	return ".mp3"
}

// fetchCover downloads cover art and sniffs its MIME type.
func (ing *Ingestor) fetchCover(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := ing.coverClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover server returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" || mime == "application/octet-stream" {
		mime = http.DetectContentType(data)
	}
	return data, mime, nil
}

// moveFile moves src to dst, falling back to copy+remove across filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
