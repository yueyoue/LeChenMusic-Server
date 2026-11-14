package online

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-flac/go-flac"
	"github.com/navidrome/navidrome/model"
)

func TestNormalizeSongKey(t *testing.T) {
	base := NormalizeSongKey("海阔天空", "BEYOND")
	cases := []struct {
		title, artist string
		same          bool
	}{
		{"海阔天空（Live）", "BEYOND", true},
		{"海阔天空 (Remastered)", "Beyond", true},
		{"海阔 天空", "BEYOND", true},
		{"海阔天空", "BEYOND ", true},
		{"海阔天空", "信乐团", false},
		{"真的爱你", "BEYOND", false},
	}
	for _, c := range cases {
		got := NormalizeSongKey(c.title, c.artist)
		if (got == base) != c.same {
			t.Errorf("NormalizeSongKey(%q, %q) = %q, want same=%v", c.title, c.artist, got, c.same)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := SanitizeFilename(`  曲名/含:非法*字符? " < > |  `)
	if strings.ContainsAny(got, `\/:*?"<>|`) {
		t.Errorf("illegal chars remain: %q", got)
	}
	if !strings.HasPrefix(got, "曲名_含_非法_字符") {
		t.Errorf("unexpected sanitize result: %q", got)
	}
}

func TestIngestorProcessEndToEnd(t *testing.T) {
	cover := testPNG(t)
	coverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(cover)
	}))
	t.Cleanup(coverSrv.Close)

	lyricsSource := &staticLyricsSource{lyrics: "[00:01.00] 今天我"}
	libRoot := t.TempDir()
	ing := NewIngestor(libRoot, lyricsSource, nil)

	var ingestedPath string
	ing.OnIngested(func(_ context.Context, p string) { ingestedPath = p })

	task := &model.DownloadTask{
		ID:           "task1",
		SongName:     "海阔天空",
		Artist:       "BEYOND",
		AlbumName:    "乐与怒",
		CoverURL:     coverSrv.URL + "/cover.jpg",
		ActualFormat: "flac",
	}
	tmpPath := filepath.Join(t.TempDir(), "dl-task1-xxx.part")
	if err := os.WriteFile(tmpPath, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}

	finalPath, err := ing.Process(context.Background(), task, tmpPath)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	wantSuffix := filepath.Join("BEYOND", "乐与怒", "海阔天空.flac")
	if !strings.HasSuffix(finalPath, wantSuffix) {
		t.Errorf("unexpected final path: %s (want suffix %s)", finalPath, wantSuffix)
	}
	if ingestedPath != finalPath {
		t.Errorf("OnIngested callback not called with final path")
	}

	f, err := flac.ParseFile(finalPath)
	if err != nil {
		t.Fatalf("parse ingested flac: %v", err)
	}
	found := map[string]bool{}
	for _, b := range f.Meta {
		switch b.Type {
		case flac.VorbisComment:
			tags := decodeVorbisComment(t, b.Data)
			if tags["TITLE"] != "海阔天空" || tags["ARTIST"] != "BEYOND" || tags["ALBUM"] != "乐与怒" {
				t.Errorf("unexpected tags: %v", tags)
			}
			if tags["LYRICS"] != "[00:01.00] 今天我" {
				t.Errorf("lyrics not embedded: %q", tags["LYRICS"])
			}
			found["tags"] = true
		case flac.Picture:
			found["cover"] = true
		}
	}
	if !found["tags"] || !found["cover"] {
		t.Errorf("missing embedded content: %v", found)
	}
}

func TestIngestorCollisionAndDefaults(t *testing.T) {
	libRoot := t.TempDir()
	ing := NewIngestor(libRoot, nil, nil)

	task := &model.DownloadTask{ID: "t1", SongName: "海阔天空", Artist: "BEYOND", ActualFormat: "flac"}
	tmp1 := filepath.Join(t.TempDir(), "a.part")
	if err := os.WriteFile(tmp1, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	p1, err := ing.Process(context.Background(), task, tmp1)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if !strings.Contains(p1, filepath.Join("BEYOND", "Unknown Album", "海阔天空.flac")) {
		t.Errorf("unexpected path for empty album: %s", p1)
	}

	tmp2 := filepath.Join(t.TempDir(), "b.part")
	if err := os.WriteFile(tmp2, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := ing.Process(context.Background(), task, tmp2)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if !strings.HasSuffix(p2, "海阔天空 (2).flac") {
		t.Errorf("expected collision rename, got %s", p2)
	}
}

func TestIngestorDedupMatcher(t *testing.T) {
	called := false
	matcher := MatcherFunc(func(_ context.Context, title, artist string) (bool, error) {
		called = true
		return NormalizeSongKey(title, artist) == NormalizeSongKey("海阔天空（Live）", "BEYOND"), nil
	})
	ing := NewIngestor(t.TempDir(), nil, matcher)

	ok, err := ing.Exists(context.Background(), "海阔天空", "BEYOND")
	if err != nil || !ok || !called {
		t.Fatalf("expected match, ok=%v err=%v called=%v", ok, err, called)
	}
	ok, _ = ing.Exists(context.Background(), "真的爱你", "BEYOND")
	if ok {
		t.Error("unexpected match")
	}
}

// staticLyricsSource is a Source that only serves lyrics.
type staticLyricsSource struct {
	mockSource
	lyrics string
}

func (s *staticLyricsSource) GetLyrics(ctx context.Context, song SongRef) (string, error) {
	return s.lyrics, nil
}
