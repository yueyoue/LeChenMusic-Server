package audiobookcover_test

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"testing"
	"testing/fstest"

	"github.com/navidrome/navidrome/core/audiobookcover"
	"github.com/navidrome/navidrome/tests"
)

// 有内嵌封面的音频文件（tests/fixtures/test.mp3 自带一张内嵌图片）
const artFixture = "test.mp3"

// 不含内嵌封面的音频文件
const noArtFixture = "no_replaygain.mp3"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("tests/fixtures/" + name)
	if err != nil {
		t.Fatalf("missing audio fixture %s: %v", name, err)
	}
	return data
}

// 音频文件本身内嵌的封面必须能被识别并提取出来。
func TestEmbeddedCoverExtractsArt(t *testing.T) {
	tests.Init(t, false)
	fsys := fstest.MapFS{
		"书名/01.mp3": {Data: fixture(t, artFixture)},
	}

	data, ext, err := audiobookcover.EmbeddedCover(fsys, "书名")
	if err != nil {
		t.Fatalf("内嵌封面必须被识别: %v", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("提取出的封面必须是合法图片: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Errorf("封面尺寸异常: %dx%d", cfg.Width, cfg.Height)
	}
	want := map[string]string{"jpeg": ".jpg", "png": ".png"}[format]
	if want == "" || ext != want {
		t.Errorf("ext = %q, want %q (图片格式 %q)", ext, want, format)
	}
}

// 封面只嵌在后面的章节文件里时也要能识别（探测窗口内的后序文件兜底）。
func TestEmbeddedCoverFallsBackToLaterFiles(t *testing.T) {
	tests.Init(t, false)
	fsys := fstest.MapFS{
		"书名/01.mp3": {Data: fixture(t, noArtFixture)},
		"书名/02.mp3": {Data: fixture(t, artFixture)},
	}

	if _, _, err := audiobookcover.EmbeddedCover(fsys, "书名"); err != nil {
		t.Fatalf("第二个音频文件里的内嵌封面也必须被识别: %v", err)
	}
}

func TestEmbeddedCoverNoAudio(t *testing.T) {
	tests.Init(t, false)
	fsys := fstest.MapFS{"书名/cover.jpg": {Data: []byte("jpeg")}}

	if _, _, err := audiobookcover.EmbeddedCover(fsys, "书名"); err == nil {
		t.Fatal("书目录里没有音频文件时不能识别出封面")
	}
}

func TestEmbeddedCoverNoArt(t *testing.T) {
	tests.Init(t, false)
	fsys := fstest.MapFS{"书名/01.mp3": {Data: fixture(t, noArtFixture)}}

	if _, _, err := audiobookcover.EmbeddedCover(fsys, "书名"); err == nil {
		t.Fatal("音频文件没有内嵌封面时不能识别出封面")
	}
}
