package online

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// scrapeStubSource serves fixed search results and lyrics for scrape tests.
type scrapeStubSource struct {
	mockSource
	songs []Song
}

func (s *scrapeStubSource) Name() string { return "scrape-stub" }

func (s *scrapeStubSource) SearchSong(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	return &SearchResult{Songs: s.songs, Total: len(s.songs)}, nil
}

func (s *scrapeStubSource) GetLyrics(ctx context.Context, song SongRef) (string, error) {
	return "[00:01.00] 刮削歌词", nil
}

func TestBestMatch(t *testing.T) {
	stub := &scrapeStubSource{songs: []Song{
		{Source: "scrape-stub", ID: "1", Name: "相思风雨中-《出位江湖》电视剧插曲", Artist: "张学友&汤宝如"},
		{Source: "scrape-stub", ID: "2", Name: "相思风雨中", Artist: "陈慧娴"},
		{Source: "scrape-stub", ID: "3", Name: "吻别", Artist: "张学友"},
	}}
	ing := NewIngestor(t.TempDir(), stub, nil)

	// Exact normalized match (id 1 matches even with the long raw title).
	got := ing.bestMatch(context.Background(), "相思风雨中", "张学友 • 汤宝如")
	if got == nil || got.ID != "1" {
		t.Fatalf("expected match id 1, got %+v", got)
	}

	// Title match with artist overlap wins over title-only.
	got = ing.bestMatch(context.Background(), "相思风雨中", "陈慧娴")
	if got == nil || got.ID != "2" {
		t.Fatalf("expected match id 2, got %+v", got)
	}

	// No title match → nil.
	if got := ing.bestMatch(context.Background(), "不存在的歌", "xxx"); got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
}

func TestScrapeSongRewritesLibraryFile(t *testing.T) {
	cover := testPNG(t)
	coverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(cover)
	}))
	t.Cleanup(coverSrv.Close)

	stub := &scrapeStubSource{songs: []Song{{
		Source: "scrape-stub", ID: "1",
		Name: "相思风雨中-《出位江湖》电视剧插曲", Artist: "张学友&汤宝如",
		AlbumName: "梦中的你", CoverURL: coverSrv.URL + "/c.png",
	}}}
	ing := NewIngestor(t.TempDir(), stub, nil)

	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ing.ScrapeSong(context.Background(), path, SongScrapeRequest{
		Title:  "相思风雨中-《出位江湖》电视剧插曲",
		Artist: "张学友&汤宝如",
	})
	if err != nil {
		t.Fatalf("ScrapeSong: %v", err)
	}
	if !res.Matched || res.Match.ID != "1" {
		t.Fatalf("expected online match, got %+v", res)
	}
	if !res.CoverFetched || !res.LyricsFetched {
		t.Errorf("expected cover+lyrics fetched: cover=%v lyrics=%v", res.CoverFetched, res.LyricsFetched)
	}
	if res.Info.Title != "相思风雨中" {
		t.Errorf("expected cleaned title, got %q", res.Info.Title)
	}
	if res.Info.Artist != "张学友 • 汤宝如" {
		t.Errorf("expected normalized artist, got %q", res.Info.Artist)
	}
	if res.Info.Album != "梦中的你" {
		t.Errorf("expected album from match, got %q", res.Info.Album)
	}
	if !res.Info.HasCover || res.Info.Lyrics == "" {
		t.Errorf("embedded cover/lyrics missing: cover=%v lyrics=%q", res.Info.HasCover, res.Info.Lyrics)
	}
}

func TestScrapeSongKeepsExistingWithoutMatch(t *testing.T) {
	stub := &scrapeStubSource{} // no results
	ing := NewIngestor(t.TempDir(), stub, nil)

	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ing.ScrapeSong(context.Background(), path, SongScrapeRequest{Title: "新标题", Artist: "新歌手"})
	if err != nil {
		t.Fatalf("ScrapeSong: %v", err)
	}
	if res.Matched {
		t.Error("expected no match")
	}
	if res.Info.Title != "新标题" || res.Info.Artist != "新歌手" {
		t.Errorf("expected cleaned keywords written, got %q/%q", res.Info.Title, res.Info.Artist)
	}
}
