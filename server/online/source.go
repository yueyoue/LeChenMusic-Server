// Package online provides pluggable download sources for the "search & download
// missing songs" feature. Each Source knows how to search songs on an external
// platform, resolve a direct media URL and fetch lyrics.
package online

import "context"

// Quality describes an audio quality option (format + bitrate).
type Quality struct {
	Format  string `json:"format"`  // "flac" or "mp3"
	Bitrate int    `json:"bitrate"` // kbps, e.g. 2000, 320, 128
}

// IsZero reports whether the quality is unset (user wants the default chain).
func (q Quality) IsZero() bool { return q.Format == "" && q.Bitrate == 0 }

// QualityOption is one quality variant a source offers for a song.
type QualityOption struct {
	Format  string `json:"format"`
	Bitrate int    `json:"bitrate"`
	Level   string `json:"level,omitempty"` // source-specific level code, e.g. "ff", "p", "h"
	Size    string `json:"size,omitempty"`  // human-readable size as reported by the source
}

// SongRef identifies a song on a specific source platform. It carries whatever
// signature material the upstream needs to resolve a media URL later.
type SongRef struct {
	Source   string `json:"source"`   // source name, e.g. "hicn"
	Platform string `json:"platform"` // upstream platform, e.g. "kuwo"
	ID       string `json:"id"`
	Time     int64  `json:"time,omitempty"` // upstream signature parameter
	Sign     string `json:"sign,omitempty"` // upstream signature parameter
}

// Song is a normalized search result entry.
type Song struct {
	Source    string          `json:"source"`
	Platform  string          `json:"platform"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Artist    string          `json:"artist"`
	AlbumName string          `json:"albumName,omitempty"`
	AlbumID   string          `json:"albumId,omitempty"` // upstream album id, used to resolve album artist + release date
	CoverURL  string          `json:"coverUrl,omitempty"`
	Duration  int             `json:"duration"` // seconds
	Qualities []QualityOption `json:"qualities,omitempty"`
	Link      string          `json:"link,omitempty"` // page URL on the source platform
	Time      int64           `json:"time,omitempty"`
	Sign      string          `json:"sign,omitempty"`
}

// Ref returns the SongRef needed for ResolveURL / GetLyrics.
func (s Song) Ref() SongRef {
	return SongRef{
		Source:   s.Source,
		Platform: s.Platform,
		ID:       s.ID,
		Time:     s.Time,
		Sign:     s.Sign,
	}
}

// SearchRequest describes a song search.
type SearchRequest struct {
	Keyword  string
	Page     int // 1-based
	PageSize int
}

// SearchResult is a page of search results.
type SearchResult struct {
	Songs []Song `json:"songs"`
	Total int    `json:"total"`
}

// ResolvedMedia is a playable/downloadable direct media URL.
type ResolvedMedia struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Bitrate  int    `json:"bitrate"`
	Duration int    `json:"duration,omitempty"` // seconds
}

// Source is the interface every download source must satisfy.
type Source interface {
	// Name returns the unique source name (e.g. "hicn").
	Name() string
	// DisplayName returns a human-readable name (e.g. "无损音乐下载").
	DisplayName() string
	// SearchSong searches for songs by keyword.
	SearchSong(ctx context.Context, req SearchRequest) (*SearchResult, error)
	// ResolveURL resolves a direct media URL for the given song and quality.
	// Sources may fall back to a lower quality if the requested one is
	// unavailable; the actual quality is reported in the result.
	ResolveURL(ctx context.Context, song SongRef, quality Quality) (*ResolvedMedia, error)
	// GetLyrics returns LRC lyrics for the song (may be empty).
	GetLyrics(ctx context.Context, song SongRef) (string, error)
}

// GetDefaultSource returns the primary download source (currently hicn).
func GetDefaultSource() Source {
	if s, ok := Get("hicn"); ok {
		return s
	}
	return nil
}

// Registry holds all registered download sources.
var sources = make(map[string]Source)

// Register adds a source to the registry.
func Register(s Source) {
	sources[s.Name()] = s
}

// Get returns a source by name.
func Get(name string) (Source, bool) {
	s, ok := sources[name]
	return s, ok
}

// GetAll returns all registered sources.
func GetAll() []Source {
	result := make([]Source, 0, len(sources))
	for _, s := range sources {
		result = append(result, s)
	}
	return result
}
