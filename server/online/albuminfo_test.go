package online

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const albumPageFixture = `<html><head><title>乐与怒_BEYOND_无损音乐专辑_酷我音乐</title></head>
<body><div data-v-eba2071e="0">
<p class="song_name">乐与怒</p>
<p class="artist_name">
  <span class="artist"><a href="/singer/1">BEYOND</a></span>
  <span class="iconfont has_mv"></span>
</p>
<p class="song_info">发行时间：1993-05-14<br/>语种：粤语<br/>唱片公司：星光</p>
</div></body></html>`

const songPageFixture = `<html><body><div data-v-eba2071e="7">
<p class="song_name">海阔天空</p>
<p class="song_info">发行时间：1993<br/>语种：粤语</p>
</div></body></html>`

func TestParseKuwoAlbumInfo(t *testing.T) {
	info := parseKuwoAlbumInfo(albumPageFixture)
	if info.Artist != "BEYOND" {
		t.Errorf("expected album artist BEYOND, got %q", info.Artist)
	}
	if info.ReleaseDate != "1993-05-14" || info.Year != 1993 {
		t.Errorf("expected 1993-05-14/1993, got %q/%d", info.ReleaseDate, info.Year)
	}
	if info.AlbumName != "乐与怒" {
		t.Errorf("expected album name 乐与怒, got %q", info.AlbumName)
	}
}

// The markup around the label carries ids like data-v-eba2071e whose digits
// must not be mistaken for a year.
func TestKuwoReleaseDateIgnoresMarkupDigits(t *testing.T) {
	body := `<p class="song_info">发行时间：<span data-v-eba2071e="2071">1993</span></p>`
	if got := kuwoReleaseDate(body); got != "1993" {
		t.Fatalf("expected 1993, got %q", got)
	}
}

func TestNormalizeAlbumID(t *testing.T) {
	cases := map[string]string{
		"2676": "2676", " 42 ": "42", "0": "", "": "", "abc": "", "12a": "",
	}
	for in, want := range cases {
		if got := NormalizeAlbumID(in); got != want {
			t.Errorf("NormalizeAlbumID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKuwoAlbumInfoCachesAlbumPages(t *testing.T) {
	var albumHits, songHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/album_detail/"):
			atomic.AddInt32(&albumHits, 1)
			_, _ = w.Write([]byte(albumPageFixture))
		case strings.HasPrefix(r.URL.Path, "/play_detail/"):
			atomic.AddInt32(&songHits, 1)
			_, _ = w.Write([]byte(songPageFixture))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	p := NewKuwoAlbumInfo(srv.URL)
	for i := 0; i < 2; i++ {
		info, err := p.AlbumInfo(context.Background(), "2676", "")
		if err != nil {
			t.Fatalf("AlbumInfo: %v", err)
		}
		if info.Year != 1993 {
			t.Errorf("expected year 1993, got %d", info.Year)
		}
	}
	if got := atomic.LoadInt32(&albumHits); got != 1 {
		t.Errorf("expected 1 album fetch (cache hit on second call), got %d", got)
	}

	// Placeholder album id ("0") falls back to the song page (date only).
	song := NewKuwoAlbumInfo(srv.URL)
	info, err := song.AlbumInfo(context.Background(), "0", "5214162")
	if err != nil {
		t.Fatalf("AlbumInfo(song fallback): %v", err)
	}
	if info.ReleaseDate != "1993" || info.Artist != "" {
		t.Errorf("expected date-only song fallback, got %+v", info)
	}
	if got := atomic.LoadInt32(&songHits); got != 1 {
		t.Errorf("expected 1 song page fetch, got %d", got)
	}
}

func TestAlbumInfoForLooksUpAlbumIDWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/album_detail/") {
			_, _ = w.Write([]byte(albumPageFixture))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	// Old clients send neither album id nor a usable one: the source must be
	// queried again to recover it from the song id.
	stub := &scrapeStubSource{songs: []Song{
		{Source: "scrape-stub", ID: "5214162", Name: "海阔天空", Artist: "BEYOND", AlbumID: "2676"},
	}}
	ing := NewIngestor(t.TempDir(), stub, nil).WithAlbumInfo(NewKuwoAlbumInfo(srv.URL))

	info := ing.albumInfoFor(context.Background(), "0", "5214162", "海阔天空", "BEYOND")
	if info == nil || info.Artist != "BEYOND" || info.Year != 1993 {
		t.Fatalf("expected enriched album info, got %+v", info)
	}
}

func TestApplyAlbumInfoKeepsExistingValues(t *testing.T) {
	meta := EmbeddedMeta{Artist: "BEYOND", AlbumArtist: "群星", Year: "1991", Album: "旧专辑"}
	applyAlbumInfo(&meta, &AlbumInfo{AlbumName: "乐与怒", Artist: "BEYOND", ReleaseDate: "1993-05-14", Year: 1993})
	if meta.AlbumArtist != "群星" || meta.Year != "1991" || meta.Album != "旧专辑" {
		t.Errorf("existing values must win, got %+v", meta)
	}

	empty := EmbeddedMeta{Artist: "BEYOND"}
	applyAlbumInfo(&empty, &AlbumInfo{AlbumName: "乐与怒", Artist: "BEYOND", ReleaseDate: "1993-05-14", Year: 1993})
	if empty.AlbumArtist != "BEYOND" || empty.Year != "1993-05-14" || empty.Album != "乐与怒" {
		t.Errorf("expected album-level fields filled, got %+v", empty)
	}
}

func TestScrapeSongWritesAlbumArtistAndYear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(albumPageFixture))
	}))
	t.Cleanup(srv.Close)

	stub := &scrapeStubSource{songs: []Song{{
		Source: "scrape-stub", ID: "5214162", Name: "海阔天空", Artist: "BEYOND",
		AlbumName: "", AlbumID: "2676",
	}}}
	ing := NewIngestor(t.TempDir(), stub, nil).WithAlbumInfo(NewKuwoAlbumInfo(srv.URL))

	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ing.ScrapeSong(context.Background(), path, SongScrapeRequest{Title: "海阔天空", Artist: "BEYOND"})
	if err != nil {
		t.Fatalf("ScrapeSong: %v", err)
	}
	if res.Info.AlbumArtist != "BEYOND" {
		t.Errorf("expected album artist written, got %q", res.Info.AlbumArtist)
	}
	if res.Info.Year != "1993-05-14" {
		t.Errorf("expected release date written, got %q", res.Info.Year)
	}
	if res.Info.Album != "乐与怒" {
		t.Errorf("expected album filled from album page, got %q", res.Info.Album)
	}
}

func TestScrapeSongWithoutAlbumProviderStillWorks(t *testing.T) {
	stub := &scrapeStubSource{songs: []Song{{
		Source: "scrape-stub", ID: "1", Name: "海阔天空", Artist: "BEYOND",
	}}}
	ing := NewIngestor(t.TempDir(), stub, nil) // no album provider

	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ing.ScrapeSong(context.Background(), path, SongScrapeRequest{Title: "海阔天空", Artist: "BEYOND"})
	if err != nil {
		t.Fatalf("ScrapeSong: %v", err)
	}
	if res.Info.AlbumArtist != "" || res.Info.Year != "" {
		t.Errorf("expected no album-level fields, got %+v", res.Info)
	}
	if res.Info.Title != "海阔天空" {
		t.Errorf("expected title written, got %q", res.Info.Title)
	}
}
