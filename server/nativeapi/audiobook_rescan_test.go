package nativeapi

import (
	"context"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/storage/storagetest"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 §4 P0-1: rescan must never destroy existing chapters when the
// book folder cannot be read (cloud gateway down, permissions, …) and must preserve the
// identity/duration of chapters that still exist, so progress and bookmarks survive.

const rescanTestScheme = "rescantest"

func setupRescanFixture(t *testing.T, files fstest.MapFS) (*audiobookHandler, *tests.MockDataStore, *tests.MockAudiobookRepo) {
	t.Helper()
	tests.Init(t, false)

	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(files)
	storagetest.Register(rescanTestScheme, ffs)

	abRepo := tests.CreateMockAudiobookRepo()
	ds := &tests.MockDataStore{MockedAudiobook: abRepo}
	libRepo := &tests.MockLibraryRepo{}
	libRepo.SetData(model.Libraries{
		{ID: 7, Name: "云盘有声书", Path: rescanTestScheme + ":///fnos/有声书"},
	})
	ds.MockedLibrary = libRepo

	book := &model.Audiobook{
		ID:            "book-1",
		LibraryID:     7,
		Title:         "三体",
		Path:          "三体",
		ChapterCount:  2,
		Size:          200,
		TotalDuration: 5400,
	}
	abRepo.Books["book-1"] = book
	abRepo.Chapters["book-1"] = []*model.AudiobookChapter{
		{ID: "keep-01", AudiobookID: "book-1", Title: "01 旧标题", ChapterNumber: 1, Duration: 3600, Format: "mp3", FileSize: 100, Path: "01.mp3", CreatedAt: time.Unix(1000, 0)},
		{ID: "keep-02", AudiobookID: "book-1", Title: "02", ChapterNumber: 2, Duration: 1800, Format: "mp3", FileSize: 100, Path: "02.mp3", CreatedAt: time.Unix(2000, 0)},
	}

	return &audiobookHandler{ds: ds}, ds, abRepo
}

func callRescan(t *testing.T, h *audiobookHandler, bookID string) *httptest.ResponseRecorder {
	t.Helper()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", bookID)
	req := httptest.NewRequest("POST", "/audiobook/"+bookID+"/rescan", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.rescan(w, req)
	return w
}

func TestRescanKeepsChaptersWhenBookDirUnreadable(t *testing.T) {
	// The library root exists, but the book folder is missing: on a cloud source this is
	// what an unreachable/renamed remote folder looks like.
	h, _, abRepo := setupRescanFixture(t, fstest.MapFS{
		"别的书/01.mp3": {Data: []byte("fake"), ModTime: time.Now()},
	})

	w := callRescan(t, h, "book-1")
	if w.Code != 500 {
		t.Fatalf("expected 500 when the book folder cannot be read, got %d (body: %s)", w.Code, w.Body.String())
	}
	chapters := abRepo.Chapters["book-1"]
	if len(chapters) != 2 {
		t.Fatalf("existing chapters must be untouched on failure, got %d chapters", len(chapters))
	}
	if chapters[0].ID != "keep-01" || chapters[0].Duration != 3600 {
		t.Fatalf("chapter identity/duration must survive, got %+v", chapters[0])
	}
}

func TestRescanRefusesEmptyDirectory(t *testing.T) {
	// A folder without audio files must not wipe the chapter list either.
	h, _, abRepo := setupRescanFixture(t, fstest.MapFS{
		"三体/cover.jpg": {Data: []byte("jpeg"), ModTime: time.Now()},
	})

	w := callRescan(t, h, "book-1")
	if w.Code != 500 {
		t.Fatalf("expected 500 for an empty book folder, got %d", w.Code)
	}
	if len(abRepo.Chapters["book-1"]) != 2 {
		t.Fatalf("existing chapters must be kept, got %d", len(abRepo.Chapters["book-1"]))
	}
}

func TestRescanPreservesChapterIdentityAndDuration(t *testing.T) {
	h, _, abRepo := setupRescanFixture(t, fstest.MapFS{
		"三体/01.mp3": {Data: []byte("fake"), ModTime: time.Now()},
		"三体/02.mp3": {Data: []byte("fake"), ModTime: time.Now()},
		"三体/03.mp3": {Data: []byte("fake"), ModTime: time.Now()},
	})

	w := callRescan(t, h, "book-1")
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	chapters := abRepo.Chapters["book-1"]
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters after rescan, got %d", len(chapters))
	}
	byPath := map[string]*model.AudiobookChapter{}
	for _, ch := range chapters {
		byPath[ch.Path] = ch
	}

	// Chapters that still exist keep their ID and measured duration (progress/bookmarks
	// reference chapter IDs).
	if ch := byPath["01.mp3"]; ch == nil || ch.ID != "keep-01" || ch.Duration != 3600 || ch.Title != "01 旧标题" {
		t.Fatalf("01.mp3 must keep its identity and duration, got %+v", ch)
	}
	if ch := byPath["02.mp3"]; ch == nil || ch.ID != "keep-02" || ch.Duration != 1800 {
		t.Fatalf("02.mp3 must keep its identity and duration, got %+v", ch)
	}
	// New files get a fresh random ID (not the filename, which used to be the ID).
	if ch := byPath["03.mp3"]; ch == nil || ch.ID == "" || ch.ID == "03.mp3" {
		t.Fatalf("03.mp3 must get a fresh ID, got %+v", ch)
	}
	// Chapter numbers follow the sorted file order.
	for _, ch := range chapters {
		want := map[string]int{"01.mp3": 1, "02.mp3": 2, "03.mp3": 3}[ch.Path]
		if ch.ChapterNumber != want {
			t.Fatalf("chapter %s: expected number %d, got %d", ch.Path, want, ch.ChapterNumber)
		}
	}

	book := abRepo.Books["book-1"]
	if book.ChapterCount != 3 || book.TotalDuration != 5400 {
		t.Fatalf("book stats not updated: chapters=%d duration=%d", book.ChapterCount, book.TotalDuration)
	}
}
