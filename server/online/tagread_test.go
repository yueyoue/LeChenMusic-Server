package online

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/navidrome/navidrome/model"
)

func TestReadEmbeddedMetadataFLAC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	cover := testPNG(t)
	meta := EmbeddedMeta{
		Title:     "海阔天空",
		Artist:    "BEYOND",
		Album:     "乐与怒",
		Year:      "1993",
		Cover:     cover,
		CoverMIME: "image/png",
		Lyrics:    "[00:01.00] 今天我",
	}
	if err := WriteEmbeddedMetadata(path, meta); err != nil {
		t.Fatalf("WriteEmbeddedMetadata: %v", err)
	}

	info, err := ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}
	if info.Title != "海阔天空" || info.Artist != "BEYOND" || info.Album != "乐与怒" {
		t.Errorf("unexpected tags: %+v", info)
	}
	if info.Year != "1993" {
		t.Errorf("unexpected year: %q", info.Year)
	}
	if !info.HasCover || !bytes.Equal(info.Cover, cover) {
		t.Errorf("cover mismatch: has=%v", info.HasCover)
	}
	if info.Lyrics != "[00:01.00] 今天我" {
		t.Errorf("unexpected lyrics: %q", info.Lyrics)
	}
	if !info.HasTags() {
		t.Error("HasTags should be true")
	}
}

func TestReadEmbeddedMetadataMP3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, minimalMP3(t), 0o644); err != nil {
		t.Fatal(err)
	}
	cover := testPNG(t)
	meta := EmbeddedMeta{
		Title:     "海阔天空",
		Artist:    "BEYOND",
		Album:     "乐与怒",
		Cover:     cover,
		CoverMIME: "image/png",
		Lyrics:    "[00:01.00] 今天我",
	}
	if err := WriteEmbeddedMetadata(path, meta); err != nil {
		t.Fatalf("WriteEmbeddedMetadata: %v", err)
	}

	info, err := ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}
	if info.Title != "海阔天空" || info.Artist != "BEYOND" || info.Album != "乐与怒" {
		t.Errorf("unexpected tags: %+v", info)
	}
	if !info.HasCover || !bytes.Equal(info.Cover, cover) {
		t.Errorf("cover mismatch: has=%v", info.HasCover)
	}
	if info.Lyrics != "[00:01.00] 今天我" {
		t.Errorf("unexpected lyrics: %q", info.Lyrics)
	}
}

func TestReadEmbeddedMetadataUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(path, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEmbeddedMetadata(path); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestRescrapeRewritesMetadata(t *testing.T) {
	cover := testPNG(t)
	coverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(cover)
	}))
	t.Cleanup(coverSrv.Close)

	newCover := testPNG(t)
	newCoverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(newCover)
	}))
	t.Cleanup(newCoverSrv.Close)

	lyricsSource := &staticLyricsSource{lyrics: "[00:02.00] 新歌词"}
	libRoot := t.TempDir()
	ing := NewIngestor(libRoot, lyricsSource, nil)

	task := &model.DownloadTask{
		ID:           "task1",
		SongName:     "海阔天空",
		Artist:       "BEYOND",
		AlbumName:    "乐与怒",
		CoverURL:     coverSrv.URL,
		ActualFormat: "flac",
	}
	tmp := filepath.Join(t.TempDir(), "in.flac")
	if err := os.WriteFile(tmp, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	finalPath, err := ing.Process(context.Background(), task, tmp)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Rescrape with corrections + a new cover source.
	task.FilePath = finalPath
	result, err := ing.Rescrape(context.Background(), task, RescrapeRequest{
		Title:    "海阔天空（修正）",
		CoverURL: newCoverSrv.URL,
	})
	if err != nil {
		t.Fatalf("Rescrape: %v", err)
	}
	if !result.CoverFetched || !result.LyricsFetched {
		t.Errorf("expected both cover and lyrics fetched: %+v", result)
	}
	if result.Info.Title != "海阔天空（修正）" {
		t.Errorf("title not corrected: %q", result.Info.Title)
	}
	// Untouched fields fall back to current values.
	if result.Info.Artist != "BEYOND" || result.Info.Album != "乐与怒" {
		t.Errorf("unexpected artist/album: %q/%q", result.Info.Artist, result.Info.Album)
	}

	// Verify on-disk state.
	info, err := ReadEmbeddedMetadata(finalPath)
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if info.Title != "海阔天空（修正）" {
		t.Errorf("on-disk title: %q", info.Title)
	}
	if !bytes.Equal(info.Cover, newCover) {
		t.Error("on-disk cover not updated")
	}
	if info.Lyrics != "[00:02.00] 新歌词" {
		t.Errorf("on-disk lyrics: %q", info.Lyrics)
	}
}

func TestRescrapeKeepsExistingOnFetchFailure(t *testing.T) {
	cover := testPNG(t)
	coverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(cover)
	}))
	t.Cleanup(coverSrv.Close)

	lyricsSource := &staticLyricsSource{lyrics: "[00:01.00] 今天我"}
	ing := NewIngestor(t.TempDir(), lyricsSource, nil)
	task := &model.DownloadTask{
		ID:           "task1",
		SongName:     "海阔天空",
		Artist:       "BEYOND",
		CoverURL:     coverSrv.URL,
		ActualFormat: "flac",
	}
	tmp := filepath.Join(t.TempDir(), "in.flac")
	if err := os.WriteFile(tmp, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	finalPath, err := ing.Process(context.Background(), task, tmp)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	task.FilePath = finalPath

	// Point at a dead cover server and a failing lyrics source: the existing
	// cover/lyrics must survive the failed re-fetch.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	dead.Close() // immediately dead
	task.CoverURL = dead.URL
	ing2 := NewIngestor(t.TempDir(), &mockSource{lyricsErr: os.ErrDeadlineExceeded}, nil)

	result, err := ing2.Rescrape(context.Background(), task, RescrapeRequest{})
	if err != nil {
		t.Fatalf("Rescrape: %v", err)
	}
	if result.CoverFetched || result.LyricsFetched {
		t.Errorf("fetch should have failed: %+v", result)
	}
	info, err := ReadEmbeddedMetadata(finalPath)
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if !info.HasCover {
		t.Error("existing cover should be kept")
	}
	if info.Lyrics != "[00:01.00] 今天我" {
		t.Errorf("existing lyrics should be kept: %q", info.Lyrics)
	}
}

func TestRescrapeMissingFile(t *testing.T) {
	ing := NewIngestor(t.TempDir(), &staticLyricsSource{}, nil)
	task := &model.DownloadTask{ID: "x", FilePath: filepath.Join(t.TempDir(), "nope.flac")}
	if _, err := ing.Rescrape(context.Background(), task, RescrapeRequest{}); err == nil {
		t.Fatal("expected error for missing file")
	}
}
