package nativeapi

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/tests"

	// 本地存储的默认标签提取器（taglib）注册：cover 兜底要通过 storage 打开音频文件
	_ "github.com/navidrome/navidrome/adapters/gotaglib"
)

// 书目录里没有封面文件时，/cover 必须能识别音频文件本身内嵌的封面并返回图片——
// 老数据（入库早于内嵌封面识别）和只有音频的书都靠这条路径出图。

func TestCoverFallsBackToEmbeddedArt(t *testing.T) {
	tests.Init(t, false)
	libDir := t.TempDir()
	bookDir := filepath.Join(libDir, "三体")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mp3, err := os.ReadFile("tests/fixtures/test.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "01.mp3"), mp3, 0o644); err != nil {
		t.Fatal(err)
	}

	h, _, _ := setupCoverFixture(t, libDir)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "book-2")
	req := httptest.NewRequest("GET", "/audiobook/book-2/cover", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.cover(w, req)

	if w.Code != 200 {
		t.Fatalf("内嵌封面必须能从 /cover 返回, got %d (body: %s)", w.Code, w.Body.String())
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes())); err != nil {
		t.Fatalf("/cover 返回的必须是音频内嵌的图片: %v", err)
	}
}
