package scanner

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/audiobookcover"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// 入库必须识别音频文件本身内嵌的封面：书目录没有 cover.jpg/folder.jpg 时，
// 扫描器从音频文件提取内嵌封面并按上传/刮削同款规则落盘，记进 book.CoverPath。

func TestAudiobookScanRecognizesEmbeddedCoverLocal(t *testing.T) {
	tests.Init(t, false)

	libDir := path.Join("tests", "fixtures", "audiobooks-embedded-cover")
	t.Cleanup(func() { _ = os.RemoveAll(libDir) })
	bookDir := path.Join(libDir, "贝姨")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mp3, err := os.ReadFile("tests/fixtures/test.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	if err := os.WriteFile(path.Join(bookDir, "01.mp3"), mp3, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	if err := NewAudiobookScanner(ds).ScanLibrary(ctx, model.Library{ID: 987657, Name: "本地内嵌封面", Path: libDir}); err != nil {
		t.Fatal(err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	book := books[0]
	// 本地可写库：封面按惯例写进书目录，CoverPath 是库相对路径
	if filepath.Dir(book.CoverPath) != "贝姨" || !strings.HasPrefix(filepath.Base(book.CoverPath), "cover.") {
		t.Fatalf("CoverPath = %q, want 贝姨/cover.*（内嵌封面必须在入库时被识别）", book.CoverPath)
	}
	data, err := os.ReadFile(path.Join(libDir, book.CoverPath))
	if err != nil {
		t.Fatalf("内嵌封面必须落盘到书目录: %v", err)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("落盘的封面必须是合法图片: %v", err)
	}
}

func TestAudiobookScanRecognizesEmbeddedCoverCloud(t *testing.T) {
	tests.Init(t, false)
	conf.Server.DataFolder = conf.NewDir(t.TempDir())

	mp3, err := os.ReadFile("tests/fixtures/test.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	setupAudiobookFS(t, fstest.MapFS{
		"三体/01.mp3": {Data: mp3, ModTime: time.Now()},
	})

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	lib := audiobookTestLibrary()
	if err := NewAudiobookScanner(ds).ScanLibrary(ctx, lib); err != nil {
		t.Fatal(err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	book := books[0]
	// 云库不可写：封面写进本地覆盖目录，CoverPath 是约定的标记路径
	wantPrefix := filepath.Join("audiobook-covers", book.ID, "cover.")
	if !strings.HasPrefix(book.CoverPath, wantPrefix) {
		t.Fatalf("CoverPath = %q, want %s*（云库的内嵌封面必须进本地覆盖目录）", book.CoverPath, wantPrefix)
	}
	p := audiobookcover.OverrideCover(book.ID)
	if p == "" {
		t.Fatal("覆盖目录里的内嵌封面必须可被 cover 接口找到")
	}
	if data, err := os.ReadFile(p); err != nil {
		t.Fatalf("覆盖目录里的封面必须可读: %v", err)
	} else if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("覆盖目录里的封面必须是合法图片: %v", err)
	}
}

// 书目录有外部封面文件时外部优先，不能被内嵌封面顶掉。
func TestAudiobookScanExternalCoverWinsOverEmbedded(t *testing.T) {
	tests.Init(t, false)

	mp3, err := os.ReadFile("tests/fixtures/test.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	setupAudiobookFS(t, fstest.MapFS{
		"鬼吹灯/01.mp3":    {Data: mp3, ModTime: time.Now()},
		"鬼吹灯/cover.jpg": {Data: []byte("jpeg"), ModTime: time.Now()},
	})

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	if err := NewAudiobookScanner(ds).ScanLibrary(ctx, audiobookTestLibrary()); err != nil {
		t.Fatal(err)
	}

	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if got := books[0].CoverPath; got != "鬼吹灯/cover.jpg" {
		t.Fatalf("CoverPath = %q, want 鬼吹灯/cover.jpg（外部封面文件优先于内嵌封面）", got)
	}
}
