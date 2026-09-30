package nativeapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 §4.6 P2-8: cross-source duplicate HINTS (size+duration coarse
// match across media sources). Pure metadata comparison — no hashing, no downloads, and
// above all: never auto-delete.

func crossSourceFixture(t *testing.T, mfs model.MediaFiles) *duplicateSongsHandler {
	t.Helper()
	tests.Init(t, false)
	mfRepo := tests.CreateMockMediaFileRepo()
	mfRepo.SetData(mfs)
	ds := &tests.MockDataStore{MockedMediaFile: mfRepo}
	return &duplicateSongsHandler{ds: ds}
}

func callCrossSource(t *testing.T, h *duplicateSongsHandler) []crossSourceGroup {
	t.Helper()
	w := httptest.NewRecorder()
	h.findCrossSourceDuplicates(w, httptest.NewRequest("GET", "/api/song/cross-source-duplicates", nil))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Data []crossSourceGroup `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.Data
}

func mf(id, title, path, libraryPath string, size int64, duration float32) model.MediaFile {
	return model.MediaFile{
		ID:          id,
		Title:       title,
		Artist:      "艺人",
		Path:        path,
		LibraryPath: libraryPath,
		Size:        size,
		Duration:    duration,
	}
}

func TestCrossSourceDuplicatesReportsSameFileInLocalAndCloud(t *testing.T) {
	h := crossSourceFixture(t, model.MediaFiles{
		mf("local-1", "第一章", "三体/01.mp3", "/vol1/music", 1000, 240.4),
		mf("cloud-1", "第一章", "三体/01.mp3", "openlist://192.168.1.10:5244/fnos/有声书", 1000, 240.0),
	})

	groups := callCrossSource(t, h)
	if len(groups) != 1 {
		t.Fatalf("expected exactly 1 cross-source group, got %d", len(groups))
	}
	g := groups[0]
	if g.Count != 2 || g.Size != 1000 {
		t.Fatalf("unexpected group %+v", g)
	}
	if len(g.Sources) != 2 || g.Sources[0] != "cloud" || g.Sources[1] != "local" {
		t.Fatalf("sources must be [cloud local], got %v", g.Sources)
	}
	// 本地排前（“优先保留本地”的提示），240.4 与 240.0 视为同一时长（粗匹配）
	if g.Songs[0].ID != "local-1" || g.Songs[1].ID != "cloud-1" {
		t.Fatalf("local copy must be listed first, got %s then %s", g.Songs[0].ID, g.Songs[1].ID)
	}
	if g.Songs[0].Source != "local" || g.Songs[1].Source != "cloud" {
		t.Fatalf("each song must carry its source kind, got %q / %q", g.Songs[0].Source, g.Songs[1].Source)
	}
}

func TestCrossSourceDuplicatesIgnoresSameLibraryDuplicates(t *testing.T) {
	h := crossSourceFixture(t, model.MediaFiles{
		mf("a", "第一章", "三体/01.mp3", "/vol1/music", 1000, 240),
		mf("b", "第一章(1)", "备份/01.mp3", "/vol1/music", 1000, 240),
	})
	if groups := callCrossSource(t, h); len(groups) != 0 {
		t.Fatalf("same-library duplicates must not be reported as cross-source, got %d groups", len(groups))
	}
}

func TestCrossSourceDuplicatesRequiresSameSize(t *testing.T) {
	h := crossSourceFixture(t, model.MediaFiles{
		mf("local-1", "第一章", "三体/01.mp3", "/vol1/music", 1000, 240),
		mf("cloud-1", "第一章", "三体/01.mp3", "openlist://h/p", 1001, 240),
	})
	if groups := callCrossSource(t, h); len(groups) != 0 {
		t.Fatalf("different size must not match, got %d groups", len(groups))
	}
}

func TestCrossSourceDuplicatesIgnoresZeroSize(t *testing.T) {
	h := crossSourceFixture(t, model.MediaFiles{
		mf("local-1", "第一章", "三体/01.mp3", "/vol1/music", 0, 240),
		mf("cloud-1", "第一章", "三体/01.mp3", "openlist://h/p", 0, 240),
	})
	if groups := callCrossSource(t, h); len(groups) != 0 {
		t.Fatalf("size 0 is not a fingerprint and must be ignored, got %d groups", len(groups))
	}
}

func TestCrossSourceDuplicatesReportsThreeWayMix(t *testing.T) {
	h := crossSourceFixture(t, model.MediaFiles{
		mf("l1", "第一章", "三体/01.mp3", "/vol1/music", 1000, 240),
		mf("c1", "第一章", "三体/01.mp3", "openlist://h/p", 1000, 240),
		mf("c2", "第一章", "三体/01.mp3", "openlist://h2/p2", 1000, 240),
		mf("other", "别的歌", "x.mp3", "/vol1/music", 2000, 100),
	})
	groups := callCrossSource(t, h)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Count != 3 {
		t.Fatalf("expected 3 copies, got %d", groups[0].Count)
	}
}
