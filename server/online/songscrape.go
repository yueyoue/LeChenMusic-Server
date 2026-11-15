package online

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/navidrome/navidrome/log"
)

// SongScrapeRequest lets the caller correct keywords while scraping a library
// song. Empty fields fall back to the file's current embedded metadata.
type SongScrapeRequest struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

// SongScrapeResult reports the online match and what was written into the file.
type SongScrapeResult struct {
	Matched       bool         `json:"matched"`
	Match         *Song        `json:"match,omitempty"`
	Info          EmbeddedInfo `json:"info"`
	CoverFetched  bool         `json:"coverFetched"`
	LyricsFetched bool         `json:"lyricsFetched"`
}

// ScrapeSong scrapes metadata for an arbitrary library audio file:
//  1. search the online source for the song (best-match scoring),
//  2. fetch cover + lyrics from the match,
//  3. merge with the file's current embedded metadata (never wipe existing
//     values when the scrape has nothing better),
//  4. rewrite embedded tags in place. The audio stream is untouched.
func (ing *Ingestor) ScrapeSong(ctx context.Context, filePath string, req SongScrapeRequest) (*SongScrapeResult, error) {
	if filePath == "" {
		return nil, fmt.Errorf("scrape: empty file path")
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("scrape: file not available: %w", err)
	}

	current, err := ReadEmbeddedMetadata(filePath)
	if err != nil {
		log.Warn(ctx, "ScrapeSong: could not read current embedded metadata", "file", filePath, err)
		current = &EmbeddedInfo{}
	}

	title := CleanSongTitle(firstNonEmpty(req.Title, current.Title))
	artist := NormalizeArtist(firstNonEmpty(req.Artist, current.Artist))
	album := firstNonEmpty(req.Album, current.Album)

	result := &SongScrapeResult{}
	meta := EmbeddedMeta{
		Title:       title,
		Artist:      artist,
		AlbumArtist: current.AlbumArtist,
		Album:       album,
		Year:        current.Year,
		TrackNumber: current.TrackNumber,
	}

	albumID, songID := "", ""
	if ing.lyrics != nil {
		if match := ing.bestMatch(ctx, title, artist); match != nil {
			result.Matched = true
			result.Match = match
			albumID, songID = match.AlbumID, match.ID
			if meta.Album == "" && match.AlbumName != "" {
				meta.Album = match.AlbumName
			}
			if match.CoverURL != "" {
				if cover, mime, err := ing.fetchCover(ctx, match.CoverURL); err == nil {
					meta.Cover, meta.CoverMIME = cover, mime
					result.CoverFetched = true
				} else {
					log.Warn(ctx, "ScrapeSong: could not fetch cover, keeping existing", "url", match.CoverURL, err)
				}
			}
			if lyrics, err := ing.lyrics.GetLyrics(ctx, match.Ref()); err == nil && lyrics != "" {
				meta.Lyrics = lyrics
				result.LyricsFetched = true
			}
		}
	}

	// Album artist + release year only exist at album level; fill them from the
	// upstream album page when the file (or the song-level match) lacks them.
	applyAlbumInfo(&meta, ing.albumInfoFor(ctx, albumID, songID, title, artist))

	if len(meta.Cover) == 0 {
		meta.Cover, meta.CoverMIME = current.Cover, current.CoverMIME
	}
	if meta.Lyrics == "" {
		meta.Lyrics = current.Lyrics
	}

	if err := WriteEmbeddedMetadata(filePath, meta); err != nil {
		return nil, fmt.Errorf("scrape: write tags: %w", err)
	}

	info, err := ReadEmbeddedMetadata(filePath)
	if err != nil {
		info = &EmbeddedInfo{
			Title:       meta.Title,
			Artist:      meta.Artist,
			AlbumArtist: meta.AlbumArtist,
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

// bestMatch searches the online source and returns the best-scoring song,
// requiring at least a title match. Returns nil when nothing matches.
func (ing *Ingestor) bestMatch(ctx context.Context, title, artist string) *Song {
	keyword := strings.TrimSpace(title + " " + strings.ReplaceAll(artist, " / ", " "))
	if keyword == "" {
		return nil
	}
	res, err := ing.lyrics.SearchSong(ctx, SearchRequest{Keyword: keyword, Page: 1, PageSize: 10})
	if err != nil || res == nil {
		if err != nil {
			log.Warn(ctx, "ScrapeSong: online search failed", "keyword", keyword, err)
		}
		return nil
	}

	wantTitle := NormalizeSongKey(CleanSongTitle(title), "")
	wantKey := NormalizeSongKey(CleanSongTitle(title), NormalizeArtist(artist))
	artistTokens := artistTokens(artist)

	best, bestScore := (*Song)(nil), 0
	for i := range res.Songs {
		s := &res.Songs[i]
		sTitle := NormalizeSongKey(CleanSongTitle(s.Name), "")
		sKey := NormalizeSongKey(CleanSongTitle(s.Name), NormalizeArtist(s.Artist))
		score := 0
		if sKey == wantKey {
			score = 100
		} else if sTitle == wantTitle {
			score = 60
			for _, tok := range artistTokens {
				if tok != "" && strings.Contains(strings.ToLower(s.Artist), strings.ToLower(tok)) {
					score += 15
				}
			}
		}
		if score > bestScore {
			best, bestScore = s, score
		}
	}
	if bestScore < 60 {
		return nil
	}
	return best
}

// artistTokens splits a normalized artist string into individual names.
func artistTokens(artist string) []string {
	parts := strings.Split(artist, " / ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
