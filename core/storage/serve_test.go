package storage_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"
	"time"

	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/core/storage/storagetest"
)

// directStorage is a cloud-like storage that can hand out signed direct links.
type directStorage struct {
	fs     *storagetest.FakeFS
	raw    string
	rawErr error
}

func (d *directStorage) FS() (storage.MusicFS, error) { return d.fs, nil }

func (d *directStorage) DirectURL(context.Context, string) (string, time.Time, error) {
	if d.rawErr != nil {
		return "", time.Time{}, d.rawErr
	}
	return d.raw, time.Now().Add(time.Hour), nil
}

// plainStorage is a local-like storage: it has an FS but cannot hand out direct links.
type plainStorage struct{ fs *storagetest.FakeFS }

func (p *plainStorage) FS() (storage.MusicFS, error) { return p.fs, nil }

func newFakeFS() *storagetest.FakeFS {
	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(fstest.MapFS{
		"书名/01 章节.mp3": {Data: []byte("0123456789abcdef"), ModTime: time.Unix(1700000000, 0)},
		"书名/cover.jpg": {Data: []byte("jpeg-bytes"), ModTime: time.Unix(1700000000, 0)},
	})
	return ffs
}

func registerScheme(t *testing.T, scheme string, st storage.Storage) string {
	t.Helper()
	storage.Register(scheme, func(url.URL) storage.Storage { return st })
	return scheme + ":///fnos/有声书"
}

func TestServeFileRedirectsToDirectLink(t *testing.T) {
	const signed = "https://cdn.example.com/file.mp3?sign=SECRET"
	libPath := registerScheme(t, "servedirect", &directStorage{fs: newFakeFS(), raw: signed})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)

	if err := storage.ServeFile(r.Context(), w, r, libPath, "书名/01 章节.mp3"); err != nil {
		t.Fatalf("ServeFile: %v", err)
	}

	res := w.Result()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if got := res.Header.Get("Location"); got != signed {
		t.Errorf("Location = %q, want the signed direct link", got)
	}
	// The signed link must never leak into the response body (design doc §15.1).
	if body := w.Body.String(); body != "" && body == signed {
		t.Errorf("signed link written to the response body: %q", body)
	}
}

func TestServeFileRelaysWhenNoDirectLink(t *testing.T) {
	libPath := registerScheme(t, "serverelay", &directStorage{fs: newFakeFS(), rawErr: errors.New("no link")})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)

	if err := storage.ServeFile(r.Context(), w, r, libPath, "书名/01 章节.mp3"); err != nil {
		t.Fatalf("ServeFile: %v", err)
	}

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body := w.Body.String(); body != "0123456789abcdef" {
		t.Errorf("body = %q, want the file contents", body)
	}
}

// Range support is what lets clients seek inside a long audiobook chapter. It has to keep
// working when we relay instead of redirecting.
func TestServeFileRelaySupportsRange(t *testing.T) {
	libPath := registerScheme(t, "serverange", &directStorage{fs: newFakeFS(), rawErr: errors.New("no link")})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)
	r.Header.Set("Range", "bytes=0-3")

	if err := storage.ServeFile(r.Context(), w, r, libPath, "书名/01 章节.mp3"); err != nil {
		t.Fatalf("ServeFile: %v", err)
	}

	res := w.Result()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206 for a Range request", res.StatusCode)
	}
	if body := w.Body.String(); body != "0123" {
		t.Errorf("body = %q, want %q", body, "0123")
	}
	if got := res.Header.Get("Content-Range"); got == "" {
		t.Error("missing Content-Range header on a partial response")
	}
}

// Local libraries have no direct-link provider at all; they must still serve files.
func TestServeFileLocalLibrary(t *testing.T) {
	ffs := newFakeFS()
	libPath := registerScheme(t, "servelocal", &plainStorage{fs: ffs}) // no DirectURL

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)

	if err := storage.ServeFile(r.Context(), w, r, libPath, "书名/cover.jpg"); err != nil {
		t.Fatalf("ServeFile: %v", err)
	}
	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
	if body := w.Body.String(); body != "jpeg-bytes" {
		t.Errorf("body = %q, want %q", body, "jpeg-bytes")
	}
}

func TestServeFileMissingFile(t *testing.T) {
	libPath := registerScheme(t, "servemissing", &directStorage{fs: newFakeFS(), rawErr: errors.New("no link")})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/stream", nil)

	err := storage.ServeFile(r.Context(), w, r, libPath, "书名/不存在.mp3")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
}
