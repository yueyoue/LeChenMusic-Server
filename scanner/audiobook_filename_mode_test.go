package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// 评审 P2-11 快速模式：TagMode=filename 的云库，有声书扫描绝不读取任何文件内容——
// 章节标题取文件名、时长留空；默认模式保持精确标签读取（MusicTag 写入的标题/时长
// 一字不差）。用真实带标签的 fixture 音频区分两种行为。

func audiobookFixture(t *testing.T, name string) []byte {
	t.Helper()
	// runtime.Caller keeps this immune to tests.Init's os.Chdir (see §8.16 lesson).
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "tests", "fixtures", name))
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	return data
}

func scanOneBook(t *testing.T, tagMode string) (model.Audiobook, model.AudiobookChapters) {
	t.Helper()
	old := tagModeForLibrary
	tagModeForLibrary = func(string) string { return tagMode }
	t.Cleanup(func() { tagModeForLibrary = old })

	audio := audiobookFixture(t, "01 Invisible (RED) Edit Version.mp3")
	setupAudiobookFS(t, fstest.MapFS{
		"测试书/01 Invisible (RED) Edit Version.mp3": {Data: audio, ModTime: time.Now()},
	})

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	s := NewAudiobookScanner(ds)
	if err := s.ScanLibrary(ctx, audiobookTestLibrary()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil || len(books) != 1 {
		t.Fatalf("expected 1 book, got %d (%v)", len(books), err)
	}
	chapters, err := ds.Audiobook(ctx).GetChapters(books[0].ID)
	if err != nil {
		t.Fatalf("chapters: %v", err)
	}
	return books[0], chapters
}

func TestAudiobookFilenameModeNeverTouchesContent(t *testing.T) {
	book, chapters := scanOneBook(t, "filename")

	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(chapters))
	}
	ch := chapters[0]
	if ch.Title != "01 Invisible (RED) Edit Version" {
		t.Fatalf("filename mode: chapter title must be the file name, got %q", ch.Title)
	}
	if ch.Duration != 0 {
		t.Fatalf("filename mode: duration must stay 0 (no content reads), got %d", ch.Duration)
	}
	if book.Narrator != "" || book.Year != 0 {
		t.Fatalf("filename mode: no tag-derived book fields, got narrator=%q year=%d", book.Narrator, book.Year)
	}
}

func TestAudiobookScanModeReadsExactTags(t *testing.T) {
	_, chapters := scanOneBook(t, "")

	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(chapters))
	}
	ch := chapters[0]
	if ch.Duration <= 0 {
		t.Fatalf("scan mode: duration must be measured from the tag stream, got %d", ch.Duration)
	}
	if ch.Title == "01 Invisible (RED) Edit Version" {
		t.Fatalf("scan mode: title must come from the file's tag, got the file name")
	}
}
