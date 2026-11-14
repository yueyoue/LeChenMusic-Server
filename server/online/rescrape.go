package online

import (
	"context"
	"fmt"
	"os"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// RescrapeRequest lets an admin correct metadata while re-scraping.
// Empty fields fall back to the task's original values (or current file values).
type RescrapeRequest struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverURL string `json:"coverUrl"` // optional override for the cover source
}

// RescrapeResult reports what the re-scrape produced, so the admin UI can
// verify the outcome without re-reading the file.
type RescrapeResult struct {
	Info          EmbeddedInfo `json:"info"`
	CoverFetched  bool         `json:"coverFetched"`
	LyricsFetched bool         `json:"lyricsFetched"`
}

// Rescrape re-runs the metadata scrape step for an already-downloaded song:
// re-fetch cover + lyrics and rewrite the embedded metadata in place.
// The audio stream is untouched and the file keeps its path (no re-download).
//
// Field precedence: request overrides > current embedded values > task values.
func (ing *Ingestor) Rescrape(ctx context.Context, task *model.DownloadTask, req RescrapeRequest) (*RescrapeResult, error) {
	if task.FilePath == "" {
		return nil, fmt.Errorf("rescrape: task has no file path")
	}
	if _, err := os.Stat(task.FilePath); err != nil {
		return nil, fmt.Errorf("rescrape: file not available: %w", err)
	}

	current, err := ReadEmbeddedMetadata(task.FilePath)
	if err != nil {
		// Keep going with task values if the file's tags are unreadable.
		log.Warn(ctx, "Rescrape: could not read current embedded metadata", "task", task.ID, err)
		current = &EmbeddedInfo{}
	}

	meta := EmbeddedMeta{
		Title:       firstNonEmpty(req.Title, current.Title, task.SongName),
		Artist:      firstNonEmpty(req.Artist, current.Artist, task.Artist),
		Album:       firstNonEmpty(req.Album, current.Album, task.AlbumName),
		Year:        current.Year,
		TrackNumber: current.TrackNumber,
	}

	result := &RescrapeResult{}

	// Cover: fresh fetch preferred; keep the existing one if fetching fails.
	coverURL := firstNonEmpty(req.CoverURL, task.CoverURL)
	if coverURL != "" {
		cover, mime, err := ing.fetchCover(ctx, coverURL)
		if err != nil {
			log.Warn(ctx, "Rescrape: could not fetch cover, keeping existing", "url", coverURL, err)
		} else {
			meta.Cover, meta.CoverMIME = cover, mime
			result.CoverFetched = true
		}
	}
	if len(meta.Cover) == 0 {
		meta.Cover, meta.CoverMIME = current.Cover, current.CoverMIME
	}

	// Lyrics: fresh fetch preferred; keep existing on failure.
	if ing.lyrics != nil {
		if lyrics, err := ing.lyrics.GetLyrics(ctx, taskRef(task)); err == nil && lyrics != "" {
			meta.Lyrics = lyrics
			result.LyricsFetched = true
		} else if err != nil {
			log.Warn(ctx, "Rescrape: could not fetch lyrics, keeping existing", "task", task.ID, err)
		}
	}
	if meta.Lyrics == "" {
		meta.Lyrics = current.Lyrics
	}

	if err := WriteEmbeddedMetadata(task.FilePath, meta); err != nil {
		return nil, fmt.Errorf("rescrape: write tags: %w", err)
	}

	info, err := ReadEmbeddedMetadata(task.FilePath)
	if err != nil {
		// Write succeeded but read-back failed: report what we wrote.
		info = &EmbeddedInfo{
			Title:       meta.Title,
			Artist:      meta.Artist,
			Album:       meta.Album,
			Year:        meta.Year,
			TrackNumber: meta.TrackNumber,
			HasCover:    len(meta.Cover) > 0,
			CoverMIME:   meta.CoverMIME,
			Lyrics:      meta.Lyrics,
		}
	}
	result.Info = *info
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
