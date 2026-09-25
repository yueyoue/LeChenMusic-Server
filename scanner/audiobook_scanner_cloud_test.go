package scanner

import (
	"context"
	"os"
	"path"
	"testing"
	"testing/fstest"
	"time"

	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/core/storage/storagetest"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"

	// Registering a "file" scheme also requires the default tag extractor to be registered
	// (core/storage/local looks it up in init-driven registries), which is what the binary
	// gets for free from its import graph.
	_ "github.com/navidrome/navidrome/adapters/gotaglib"
)

// The whole point of M5 is that the audiobook scanner must go through core/storage, so a
// cloud library (openlist://...) is scanned exactly like a local folder. These tests pin
// that down by scanning a library whose path is *not* a local directory at all.

const abTestScheme = "abtest"

func setupAudiobookFS(t *testing.T, files fstest.MapFS) *storagetest.FakeFS {
	t.Helper()
	ffs := &storagetest.FakeFS{}
	ffs.SetFiles(files)
	storagetest.Register(abTestScheme, ffs)
	return ffs
}

func audiobookTestLibrary() model.Library {
	return model.Library{ID: 987654, Name: "云盘有声书", Path: abTestScheme + ":///fnos/有声书"}
}

func TestAudiobookScanGoesThroughStorage(t *testing.T) {
	tests.Init(t, false)

	setupAudiobookFS(t, fstest.MapFS{
		"鬼吹灯/01 引子.mp3":   {Data: []byte("fake-mp3"), ModTime: time.Now()},
		"鬼吹灯/02 黄皮子坟.mp3": {Data: []byte("fake-mp3"), ModTime: time.Now()},
		"鬼吹灯/03 云顶天宫.mp3": {Data: []byte("fake-mp3"), ModTime: time.Now()},
		"鬼吹灯/cover.jpg":   {Data: []byte("jpeg"), ModTime: time.Now()},
		"三体/01.mp3":       {Data: []byte("fake-mp3"), ModTime: time.Now()},
		"三体/02.mp3":       {Data: []byte("fake-mp3"), ModTime: time.Now()},
		// Not a book: no audio files
		"杂项/readme.txt": {Data: []byte("x"), ModTime: time.Now()},
		// Hidden dirs must be pruned
		".cache/01.mp3": {Data: []byte("fake-mp3"), ModTime: time.Now()},
	})

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	lib := audiobookTestLibrary()

	s := NewAudiobookScanner(ds)
	if err := s.ScanLibrary(ctx, lib); err != nil {
		t.Fatalf("ScanLibrary failed on a non-local library: %v", err)
	}

	repo := ds.Audiobook(ctx)
	books, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	byTitle := map[string]model.Audiobook{}
	for _, b := range books {
		if b.LibraryID == lib.ID {
			byTitle[b.Title] = b
		}
	}
	if len(byTitle) != 2 {
		t.Fatalf("expected 2 books from the cloud library, got %d (%+v)", len(byTitle), byTitle)
	}

	ghost, ok := byTitle["鬼吹灯"]
	if !ok {
		t.Fatalf("book 鬼吹灯 not found in %v", byTitle)
	}
	if ghost.Author != "" {
		t.Errorf("expected no author parsed from dir name, got %q", ghost.Author)
	}
	if ghost.Genre != "有声读物" {
		t.Errorf("expected genre 有声读物, got %q", ghost.Genre)
	}
	// Library-relative, slash-separated — this is what playback and covers key off.
	if ghost.Path != "鬼吹灯" {
		t.Errorf("book path should be library-relative with '/', got %q", ghost.Path)
	}
	if ghost.CoverPath != "鬼吹灯/cover.jpg" {
		t.Errorf("cover path should be library-relative with '/', got %q", ghost.CoverPath)
	}

	chapters, err := repo.GetChapters(ghost.ID)
	if err != nil {
		t.Fatalf("GetChapters: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	wantTitles := []string{"01 引子", "02 黄皮子坟", "03 云顶天宫"}
	for i, ch := range chapters {
		if ch.ChapterNumber != i+1 {
			t.Errorf("chapter %d has ChapterNumber %d", i, ch.ChapterNumber)
		}
		if ch.Title != wantTitles[i] {
			t.Errorf("chapter %d: title = %q, want %q (order must follow the filename)", i, ch.Title, wantTitles[i])
		}
		if ch.Path != wantTitles[i]+".mp3" {
			t.Errorf("chapter %d: path = %q, want %q (relative to the book dir)", i, ch.Path, wantTitles[i]+".mp3")
		}
		if ch.Format != "mp3" {
			t.Errorf("chapter %d: format = %q, want mp3", i, ch.Format)
		}
	}

	three, ok := byTitle["三体"]
	if !ok {
		t.Fatalf("book 三体 not found in %v", byTitle)
	}
	if three.ChapterCount != 2 {
		t.Errorf("三体: ChapterCount = %d, want 2", three.ChapterCount)
	}

	// "杂项" has no audio files and ".cache" is hidden: neither may become a book.
	for _, b := range books {
		if b.LibraryID != lib.ID {
			continue
		}
		if b.Title == "杂项" || b.Path == "杂项" {
			t.Errorf("directory without audio files must not become a book: %+v", b)
		}
		if b.Path == ".cache" || b.Title == ".cache" {
			t.Errorf("hidden directory must be pruned: %+v", b)
		}
	}
}

func TestAudiobookScanLocalLibraryUnchanged(t *testing.T) {
	tests.Init(t, false)

	// A plain local folder must keep working through the very same code path.
	lib := model.Library{ID: 987655, Name: "本地有声书", Path: "tests/fixtures/audiobooks"}
	t.Cleanup(func() { _ = os.RemoveAll(lib.Path) })

	bookDir := path.Join(lib.Path, "贝姨")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01.mp3", "02.mp3"} {
		data, err := os.ReadFile("tests/fixtures/01 Invisible (RED) Edit Version.mp3")
		if err != nil {
			t.Fatalf("missing audio fixture: %v", err)
		}
		if err := os.WriteFile(path.Join(bookDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	s := NewAudiobookScanner(ds)
	if err := s.ScanLibrary(ctx, lib); err != nil {
		t.Fatalf("ScanLibrary failed on a local library: %v", err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	var found *model.Audiobook
	for i := range books {
		if books[i].LibraryID == lib.ID {
			found = &books[i]
			break
		}
	}
	if found == nil {
		t.Fatal("local library scanned 0 books — local behaviour regressed")
	}
	if found.Title == "" {
		t.Error("book title must not be empty")
	}
	// The fixture carries a TITLE tag, so the tag wins over the directory name — that is the
	// long-standing behaviour and it must not change for local libraries.
	if found.Title == "贝姨" {
		t.Errorf("title = %q, expected the ID3 TITLE tag to override the directory name", found.Title)
	}
	if found.Path != "贝姨" {
		t.Errorf("path = %q, want 贝姨", found.Path)
	}

	chapters, err := ds.Audiobook(ctx).GetChapters(found.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	for _, ch := range chapters {
		// Real audio fixtures: the duration must be read from the tag, not left at 0.
		if ch.Duration <= 0 {
			t.Errorf("chapter %q: duration = %d, want > 0 (read from the real file tag)", ch.Title, ch.Duration)
		}
		if ch.FileSize <= 0 {
			t.Errorf("chapter %q: file size = %d, want > 0", ch.Title, ch.FileSize)
		}
	}

	// A second scan must not duplicate the book: "book already exists" is what keeps a
	// rescan from re-reading (and for cloud sources, re-fetching) anything.
	if err := s.ScanLibrary(ctx, lib); err != nil {
		t.Fatalf("rescan failed: %v", err)
	}
	books, err = ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, b := range books {
		if b.LibraryID == lib.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("after rescan found %d books for the library, want 1 (rescan must not duplicate)", count)
	}
}

// TestAudiobookScanReadsRealTags makes sure a chapter's duration comes from the audio tag
// even when the file lives behind a storage abstraction (cloud sources rely on this: the
// lazy Range reader must hand taglib a seekable handle).
func TestAudiobookScanReadsRealTags(t *testing.T) {
	tests.Init(t, false)

	data, err := os.ReadFile("tests/fixtures/01 Invisible (RED) Edit Version.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	setupAudiobookFS(t, fstest.MapFS{
		"云端书/01.mp3": {Data: data, ModTime: time.Now()},
	})

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	lib := model.Library{ID: 987656, Name: "云盘标签", Path: abTestScheme + ":///fnos/有声书"}
	if err := NewAudiobookScanner(ds).ScanLibrary(ctx, lib); err != nil {
		t.Fatal(err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range books {
		if b.LibraryID != lib.ID {
			continue
		}
		chapters, err := ds.Audiobook(ctx).GetChapters(b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(chapters) != 1 {
			t.Fatalf("expected 1 chapter, got %d", len(chapters))
		}
		if chapters[0].Duration <= 0 {
			t.Errorf("duration = %d, want > 0 — taglib must get a seekable handle through the storage layer",
				chapters[0].Duration)
		}
		return
	}
	t.Fatal("cloud library scanned 0 books")
}

// Ensure the helper the scanner relies on really resolves through the storage registry.
func TestAudiobookFSResolvesThroughStorage(t *testing.T) {
	setupAudiobookFS(t, fstest.MapFS{"a/01.mp3": {Data: []byte("x"), ModTime: time.Now()}})
	fsys, err := audiobookFS(context.Background(), audiobookTestLibrary())
	if err != nil {
		t.Fatalf("audiobookFS: %v", err)
	}
	if fsys == nil {
		t.Fatal("audiobookFS returned nil FS")
	}
	if _, ok := fsys.(storage.MusicFS); !ok {
		t.Fatalf("audiobookFS returned %T, which is not a storage.MusicFS", fsys)
	}
}
