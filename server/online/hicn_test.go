package online

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// newTestHicn spins up a mock upstream and returns a source pointing at it.
func newTestHicn(t *testing.T, handler http.HandlerFunc) (*hicnSource, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	s := NewHicnSource(ts.URL)
	s.minInterval = 0
	return s, ts
}

const searchResponse = `{
	"code": 0,
	"msg": "ok",
	"data": {
		"list": [
			{
				"id": "5886682",
				"name": "海阔天空",
				"artist": "BEYOND",
				"album_name": "乐与怒",
				"pic_url": "https://img.example.com/cover.jpg",
				"duration": "324",
				"minfo": [
					{"level": "ff", "bitrate": "2000", "format": "flac", "size": "31.36Mb"},
					{"level": "p", "bitrate": "320", "format": "mp3", "size": "12.39Mb"},
					{"level": "h", "bitrate": "128", "format": "mp3", "size": "4.96Mb"}
				],
				"link": "https://www.kuwo.cn/play_detail/5886682",
				"hasHQ": true,
				"hasSQ": true,
				"time": 1794641926,
				"sign": "a109de62b3dc45c2c0efadba315d547c"
			}
		],
		"total": 1
	}
}`

func TestHicnSearchSong(t *testing.T) {
	var gotAct, gotPlatform, gotKeyword, gotPage, gotSize string
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotAct = r.URL.Query().Get("act")
		gotPlatform = r.FormValue("platform")
		gotKeyword = r.FormValue("keyword")
		gotPage = r.FormValue("page")
		gotSize = r.FormValue("size")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchResponse))
	})

	res, err := s.SearchSong(context.Background(), SearchRequest{Keyword: "海阔天空 BEYOND", Page: 2, PageSize: 10})
	if err != nil {
		t.Fatalf("SearchSong: %v", err)
	}

	if gotAct != "search" || gotPlatform != "kuwo" || gotKeyword != "海阔天空 BEYOND" || gotPage != "2" || gotSize != "10" {
		t.Errorf("unexpected request params: act=%q platform=%q keyword=%q page=%q size=%q",
			gotAct, gotPlatform, gotKeyword, gotPage, gotSize)
	}
	if res.Total != 1 || len(res.Songs) != 1 {
		t.Fatalf("unexpected result: total=%d songs=%d", res.Total, len(res.Songs))
	}

	song := res.Songs[0]
	if song.Name != "海阔天空" || song.Artist != "BEYOND" || song.AlbumName != "乐与怒" {
		t.Errorf("unexpected song fields: %+v", song)
	}
	if song.Duration != 324 {
		t.Errorf("expected duration 324, got %d", song.Duration)
	}
	if len(song.Qualities) != 3 {
		t.Fatalf("expected 3 qualities, got %d", len(song.Qualities))
	}
	if song.Qualities[0].Format != "flac" || song.Qualities[0].Bitrate != 2000 || song.Qualities[0].Size != "31.36Mb" {
		t.Errorf("unexpected first quality: %+v", song.Qualities[0])
	}

	ref := song.Ref()
	if ref.Source != "hicn" || ref.Platform != "kuwo" || ref.ID != "5886682" ||
		ref.Time != 1794641926 || ref.Sign != "a109de62b3dc45c2c0efadba315d547c" {
		t.Errorf("unexpected ref: %+v", ref)
	}
}

func TestHicnSearchSongEmptyKeyword(t *testing.T) {
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called")
	})
	if _, err := s.SearchSong(context.Background(), SearchRequest{}); err == nil {
		t.Fatal("expected error for empty keyword")
	}
}

func TestHicnSearchSongAPIError(t *testing.T) {
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"msg":"boom","data":null}`))
	})
	if _, err := s.SearchSong(context.Background(), SearchRequest{Keyword: "x"}); err == nil {
		t.Fatal("expected error for non-zero upstream code")
	}
}

func TestHicnResolveURLAndCache(t *testing.T) {
	var calls atomic.Int32
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if act := r.URL.Query().Get("act"); act != "getUrl" {
			t.Errorf("unexpected act %q", act)
		}
		_ = r.ParseForm()
		if r.FormValue("songid") != "5886682" || r.FormValue("format") != "flac" ||
			r.FormValue("bitrate") != "2000" || r.FormValue("sign") != "abc" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"url":"https://cdn.example.com/a.flac","format":"flac","bitrate":2000,"duration":324,"songid":"5886682"}}`))
	})

	ref := SongRef{Source: "hicn", Platform: "kuwo", ID: "5886682", Time: 1, Sign: "abc"}
	q := Quality{Format: "flac", Bitrate: 2000}

	media, err := s.ResolveURL(context.Background(), ref, q)
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}
	if media.URL != "https://cdn.example.com/a.flac" || media.Format != "flac" || media.Bitrate != 2000 || media.Duration != 324 {
		t.Errorf("unexpected media: %+v", media)
	}

	// Second call must be served from cache.
	if _, err := s.ResolveURL(context.Background(), ref, q); err != nil {
		t.Fatalf("ResolveURL (cached): %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("expected 1 upstream call, got %d", n)
	}
}

func TestHicnGetLyricsObject(t *testing.T) {
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		if act := r.URL.Query().Get("act"); act != "getLyric" {
			t.Errorf("unexpected act %q", act)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"lyric":"[00:00.00] 海阔天空"}}`))
	})

	lyric, err := s.GetLyrics(context.Background(), SongRef{Platform: "kuwo", ID: "1", Time: 2, Sign: "s"})
	if err != nil {
		t.Fatalf("GetLyrics: %v", err)
	}
	if lyric != "[00:00.00] 海阔天空" {
		t.Errorf("unexpected lyric: %q", lyric)
	}
}

func TestHicnGetLyricsString(t *testing.T) {
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":"[00:00.00] plain"}`))
	})

	lyric, err := s.GetLyrics(context.Background(), SongRef{Platform: "kuwo", ID: "1"})
	if err != nil {
		t.Fatalf("GetLyrics: %v", err)
	}
	if lyric != "[00:00.00] plain" {
		t.Errorf("unexpected lyric: %q", lyric)
	}
}

func TestHicnRegistered(t *testing.T) {
	src, ok := Get("hicn")
	if !ok {
		t.Fatal("hicn source not registered")
	}
	if src.Name() != "hicn" || src.DisplayName() == "" {
		t.Errorf("unexpected source identity: %q %q", src.Name(), src.DisplayName())
	}
}
