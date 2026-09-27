package nativeapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 §4 P1-2: uploading/scraping a cover for a book in a cloud
// library must work (it used to os.WriteFile into the library path and fail), and the
// cover must be served back through /api/audiobook/{id}/cover.

func setupCoverFixture(t *testing.T, libPath string) (*audiobookHandler, *tests.MockDataStore, *tests.MockAudiobookRepo) {
	t.Helper()
	tests.Init(t, false)
	tempDir := t.TempDir()
	conf.Server.DataFolder = conf.NewDir(tempDir)

	abRepo := tests.CreateMockAudiobookRepo()
	ds := &tests.MockDataStore{MockedAudiobook: abRepo}
	libRepo := &tests.MockLibraryRepo{}
	libRepo.SetData(model.Libraries{
		{ID: 8, Name: "有声书", Path: libPath},
	})
	ds.MockedLibrary = libRepo

	abRepo.Books["book-2"] = &model.Audiobook{
		ID:        "book-2",
		LibraryID: 8,
		Title:     "三体",
		Path:      "三体",
	}
	return &audiobookHandler{ds: ds}, ds, abRepo
}

func coverRequest(t *testing.T, h *audiobookHandler, bookID string, imageData []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "cover.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(imageData); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = mw.Close()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", bookID)
	req := httptest.NewRequest("POST", "/audiobook/"+bookID+"/cover", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.uploadCover(w, req)
	return w
}

func TestSaveAudiobookCoverLocalLibraryUnchanged(t *testing.T) {
	tests.Init(t, false)
	libDir := t.TempDir()
	bookDir := filepath.Join(libDir, "三体")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing cover must be replaced, as before.
	if err := os.WriteFile(filepath.Join(bookDir, "cover.png"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &model.Audiobook{ID: "b1", Path: "三体"}
	lib := &model.Library{Path: libDir}
	relCover, err := saveAudiobookCover(book, lib, []byte("new-cover-bytes"), ".jpg")
	if err != nil {
		t.Fatalf("saveAudiobookCover: %v", err)
	}
	if relCover != filepath.Join("三体", "cover.jpg") {
		t.Fatalf("unexpected relCover %q", relCover)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "cover.jpg")); err != nil {
		t.Fatalf("local cover must be written next to the audio files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "cover.png")); !os.IsNotExist(err) {
		t.Fatal("old cover must be removed")
	}
}

func TestSaveAudiobookCoverCloudLibraryUsesOverrideDir(t *testing.T) {
	setupCoverFixture(t, "openlist://192.168.1.10:5244/fnos/有声书")
	book := &model.Audiobook{ID: "book-2", Path: "三体"}
	lib := &model.Library{Path: "openlist://192.168.1.10:5244/fnos/有声书"}

	relCover, err := saveAudiobookCover(book, lib, []byte("cloud-cover-bytes"), ".jpg")
	if err != nil {
		t.Fatalf("cloud cover must not fail: %v", err)
	}
	want := filepath.Join("audiobook-covers", "book-2", "cover.jpg")
	if relCover != want {
		t.Fatalf("unexpected coverPath marker %q (want %q)", relCover, want)
	}
	if p := audiobookCoverOverride("book-2"); p == "" {
		t.Fatal("override cover must be discoverable")
	} else if data, err := os.ReadFile(p); err != nil || string(data) != "cloud-cover-bytes" {
		t.Fatalf("override cover content mismatch: %q %v", data, err)
	}
}

func TestUploadCoverWorksForCloudLibrary(t *testing.T) {
	h, _, abRepo := setupCoverFixture(t, "openlist://192.168.1.10:5244/fnos/有声书")

	w := coverRequest(t, h, "book-2", bytes.Repeat([]byte("x"), 200))
	if w.Code != 200 {
		t.Fatalf("cover upload for a cloud book must succeed, got %d (body: %s)", w.Code, w.Body.String())
	}
	if abRepo.Books["book-2"].CoverPath == "" {
		t.Fatal("book.CoverPath must be set after upload")
	}

	// And the cover must be served back through the API.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "book-2")
	req := httptest.NewRequest("GET", "/audiobook/book-2/cover", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w = httptest.NewRecorder()
	h.cover(w, req)
	if w.Code != 200 {
		t.Fatalf("cover must be served for a cloud book, got %d", w.Code)
	}
	if got := w.Body.String(); got != string(bytes.Repeat([]byte("x"), 200)) {
		t.Fatalf("unexpected cover body: %q", got)
	}
}

func TestDownloadCoverBlocksInternalURL(t *testing.T) {
	book := &model.Audiobook{ID: "book-2", Path: "三体"}
	lib := &model.Library{Path: "openlist://192.168.1.10:5244/fnos/有声书"}
	if err := downloadCover("http://169.254.169.254/latest/meta-data/", "", book, *lib); err == nil {
		t.Fatal("scrape cover download must refuse internal URLs")
	}
}
