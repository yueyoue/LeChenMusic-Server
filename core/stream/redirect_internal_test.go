package stream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/model"
)

// The 302 playback mode of the cloud media source (docs/网盘媒体源方案设计.md §2):
// when a direct link is available the response is a redirect and this server never
// touches the media bytes.

func TestServeRedirectsToTheDirectLink(t *testing.T) {
	const direct = "https://cdn.example.com/file.flac?signature=SECRET"
	mf := &model.MediaFile{ID: "1", Title: "track", Suffix: "flac"}
	s := &Stream{ctx: context.Background(), mf: mf, format: "flac", redirectURL: direct}

	if !s.Redirected() {
		t.Fatal("a stream carrying a direct link must report Redirected()")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/rest/stream?id=1", nil)
	n, err := s.Serve(context.Background(), rec, req)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if n != 0 {
		t.Errorf("a redirect must not relay any bytes, got %d", n)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d (302)", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != direct {
		t.Errorf("Location = %q, want the direct link", got)
	}
	// The signed link must never leak into the response body (design doc §15.1): a plain
	// http.Redirect would write `<a href="<signed>">Found</a>.` here.
	if body := rec.Body.String(); body != "" {
		t.Errorf("redirect must not write a response body, got %q", body)
	}
}

func TestServeRedirectDoesNotForwardHeaders(t *testing.T) {
	const direct = "https://cdn.example.com/file.flac?signature=SECRET"
	mf := &model.MediaFile{ID: "1", Title: "track", Suffix: "flac"}
	s := &Stream{ctx: context.Background(), mf: mf, format: "flac", redirectURL: direct}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/rest/stream?id=1", nil)
	if _, err := s.Serve(context.Background(), rec, req); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	for _, h := range []string{"Authorization", "X-ND-Authorization", "Cookie"} {
		if v := rec.Header().Get(h); v != "" {
			t.Errorf("header %s must never be forwarded to the CDN, got %q", h, v)
		}
	}
}

func TestStreamCloseIsSafeWithoutAReader(t *testing.T) {
	// A redirected stream has no reader at all; the defer in the handlers must not panic.
	s := &Stream{ctx: context.Background(), mf: &model.MediaFile{ID: "1"}, format: "flac", redirectURL: "https://x/y"}
	if s.ReadCloser != nil {
		t.Fatal("a redirected stream must not hold a reader")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close on a redirected stream: %v", err)
	}
}

type nopReadCloser struct{}

func (nopReadCloser) Read([]byte) (int, error) { return 0, errors.New("nope") }
func (nopReadCloser) Close() error             { return nil }

func TestStreamClosePropagates(t *testing.T) {
	s := &Stream{ctx: context.Background(), mf: &model.MediaFile{ID: "1"}, format: "raw", ReadCloser: nopReadCloser{}}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestIsRemotePath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/vol1/music", false},
		{"C:/music", false},
		{"file:///vol1/music", false},
		{"openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90", true},
		{"webdav://nas/share", true},
	}
	for _, c := range cases {
		if got := isRemotePath(c.in); got != c.want {
			t.Errorf("isRemotePath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
