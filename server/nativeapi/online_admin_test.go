package nativeapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	onlinesvc "github.com/navidrome/navidrome/server/online"
	"github.com/navidrome/navidrome/tests"
)

// fakeOnlineOpLogRepo records entries in memory.
type fakeOnlineOpLogRepo struct {
	mu      sync.Mutex
	entries model.OnlineOpLogs
}

func (f *fakeOnlineOpLogRepo) Add(entry *model.OnlineOpLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entry.ID == "" {
		entry.ID = "log-" + strconv.Itoa(len(f.entries)+1)
	}
	f.entries = append(f.entries, *entry)
	return nil
}

func (f *fakeOnlineOpLogRepo) GetAll() (model.OnlineOpLogs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(model.OnlineOpLogs, len(f.entries))
	copy(out, f.entries)
	return out, nil
}

func (f *fakeOnlineOpLogRepo) DeleteAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
	return nil
}

// setupAdminHandler builds an onlineAdminHandler with fakes.
func setupAdminHandler(t *testing.T) (*onlineAdminHandler, *fakeDownloadTaskRepo, *fakeOnlineOpLogRepo, string) {
	t.Helper()

	// Deterministic lyrics source: no network in tests.
	stub := &stubOnlineSource{}
	onlinesvc.Register(stub)
	t.Cleanup(func() { onlinesvc.Register(onlinesvc.NewHicnSource("")) })

	taskRepo := newFakeDownloadTaskRepo()
	logRepo := &fakeOnlineOpLogRepo{}
	userRepo := tests.CreateMockUserRepo()
	_ = userRepo.Put(&model.User{ID: "u1", UserName: "alice"})
	_ = userRepo.Put(&model.User{ID: "admin", UserName: "root", IsAdmin: true})

	libDir := t.TempDir()

	ds := &tests.MockDataStore{
		MockedDownloadTask: taskRepo,
		MockedOnlineOpLog:  logRepo,
		MockedUser:         userRepo,
	}

	ing := onlinesvc.NewIngestor(libDir, onlinesvc.GetDefaultSource(), nil)
	h := &onlineAdminHandler{ds: ds, ingest: ing}
	return h, taskRepo, logRepo, libDir
}

func onlineAdminReq(t *testing.T, admin bool, target string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	user := model.User{ID: "u1", UserName: "alice"}
	if admin {
		user = model.User{ID: "admin", UserName: "root", IsAdmin: true}
	}
	return req.WithContext(request.WithUser(req.Context(), user))
}

func withRouteID(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// minimalFLACWithTags builds a small FLAC carrying embedded metadata.
func minimalFLACWithTags(t *testing.T, meta onlinesvc.EmbeddedMeta) []byte {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "x.flac")

	// STREAMINFO-only FLAC, same shape as the online package's test fixture.
	data := []byte{'f', 'L', 'a', 'C', 0x80, 0x00, 0x00, 0x22}
	streamInfo := make([]byte, 34)
	streamInfo[0], streamInfo[1] = 0x10, 0x00
	streamInfo[2], streamInfo[3] = 0x10, 0x00
	data = append(data, streamInfo...)
	data = append(data, 0xFF, 0xF8)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := onlinesvc.WriteEmbeddedMetadata(src, meta); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// makeDoneTaskFile writes a FLAC with embedded tags at path and registers a
// matching done task.
func makeDoneTaskFile(t *testing.T, repo *fakeDownloadTaskRepo, libDir, name string) *model.DownloadTask {
	t.Helper()
	path := filepath.Join(libDir, name+".flac")
	data := minimalFLACWithTags(t, onlinesvc.EmbeddedMeta{
		Title: name, Artist: "BEYOND", Album: "乐与怒", Lyrics: "[00:01.00] 今天我",
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		UserID:       "u1",
		SongName:     name,
		Artist:       "BEYOND",
		AlbumName:    "乐与怒",
		Status:       model.DownloadStatusDone,
		FilePath:     path,
		ActualFormat: "flac",
		Source:       "hicn",
	}
	if err := repo.Add(task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestAdminListDownloads(t *testing.T) {
	h, taskRepo, _, libDir := setupAdminHandler(t)
	_ = makeDoneTaskFile(t, taskRepo, libDir, "海阔天空")
	_ = taskRepo.Add(&model.DownloadTask{UserID: "u1", SongName: "pending", Status: model.DownloadStatusQueued})

	req := onlineAdminReq(t, true, "/api/online/downloads")
	w := httptest.NewRecorder()
	h.listDownloads(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp adminDownloadList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected only done tasks, got %d", resp.Total)
	}
	if resp.Data[0].SongName != "海阔天空" {
		t.Errorf("unexpected item: %+v", resp.Data[0])
	}
	if resp.Data[0].UserName != "alice" {
		t.Errorf("expected userName resolved, got %q", resp.Data[0].UserName)
	}
	if !resp.Data[0].FileExists {
		t.Error("file should exist")
	}
	if resp.Data[0].Scrape == nil || !resp.Data[0].Scrape.HasTags() {
		t.Error("scrape status expected")
	}
}

func TestAdminListDownloadsFilter(t *testing.T) {
	h, taskRepo, _, libDir := setupAdminHandler(t)
	_ = makeDoneTaskFile(t, taskRepo, libDir, "海阔天空")
	_ = makeDoneTaskFile(t, taskRepo, libDir, "真的爱你")

	req := onlineAdminReq(t, true, "/api/online/downloads?q=真的")
	w := httptest.NewRecorder()
	h.listDownloads(w, req)
	var resp adminDownloadList
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Fatalf("keyword filter expected 1, got %d", resp.Total)
	}

	req = onlineAdminReq(t, true, "/api/online/downloads?scrape=missing-lyrics")
	w = httptest.NewRecorder()
	h.listDownloads(w, req)
	resp = adminDownloadList{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Fatalf("all files have lyrics; missing-lyrics expected 0, got %d", resp.Total)
	}
}

func TestAdminDownloadsMiddlewareRejectsNonAdmin(t *testing.T) {
	mw := adminOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := onlineAdminReq(t, false, "/api/online/downloads")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin expected 403, got %d", w.Code)
	}
}

func TestAdminVerifyDownload(t *testing.T) {
	h, taskRepo, _, libDir := setupAdminHandler(t)
	task := makeDoneTaskFile(t, taskRepo, libDir, "海阔天空")

	req := withRouteID(onlineAdminReq(t, true, "/api/online/downloads/x/verify"), task.ID)
	w := httptest.NewRecorder()
	h.verifyDownload(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp verifyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.Exists {
		t.Fatal("file should exist")
	}
	if resp.Metadata == nil {
		t.Fatal("metadata expected")
	}
	if resp.Metadata.Title != "海阔天空" {
		t.Errorf("unexpected embedded title: %q", resp.Metadata.Title)
	}
}

func TestAdminDeleteDownload(t *testing.T) {
	h, taskRepo, logRepo, libDir := setupAdminHandler(t)
	task := makeDoneTaskFile(t, taskRepo, libDir, "海阔天空")

	req := withRouteID(onlineAdminReq(t, true, "/api/online/downloads/x"), task.ID)
	w := httptest.NewRecorder()
	h.deleteDownload(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp deleteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.DeletedFile {
		t.Error("file should be deleted")
	}
	if _, err := os.Stat(task.FilePath); !os.IsNotExist(err) {
		t.Error("file should be gone from disk")
	}
	if _, err := taskRepo.Get(task.ID); err == nil {
		t.Error("task record should be deleted")
	}
	logs, _ := logRepo.GetAll()
	if len(logs) != 1 || logs[0].Action != model.OpLogDelete {
		t.Errorf("expected delete op logged, got %+v", logs)
	}
}

func TestAdminRescrapeDownload(t *testing.T) {
	h, taskRepo, logRepo, libDir := setupAdminHandler(t)
	task := makeDoneTaskFile(t, taskRepo, libDir, "海阔天空")

	body := `{"title":"海阔天空（修正）"}`
	req := withRouteID(onlineAdminReq(t, true, "/api/online/downloads/x/rescrape"), task.ID)
	req.Method = http.MethodPost
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	h.rescrapeDownload(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	logs, _ := logRepo.GetAll()
	found := false
	for _, l := range logs {
		if l.Action == model.OpLogRescrape && l.Success {
			found = true
		}
	}
	if !found {
		t.Errorf("expected successful rescrape logged, got %+v", logs)
	}

	// Task record should reflect the corrected title.
	updated, err := taskRepo.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SongName != "海阔天空（修正）" {
		t.Errorf("task title not updated: %q", updated.SongName)
	}
}

func TestAdminLogs(t *testing.T) {
	h, _, logRepo, _ := setupAdminHandler(t)
	_ = logRepo.Add(&model.OnlineOpLog{Action: model.OpLogDownloadDone, SongName: "x"})

	req := onlineAdminReq(t, true, "/api/online/logs")
	w := httptest.NewRecorder()
	h.listLogs(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	req = onlineAdminReq(t, true, "/api/online/logs")
	w = httptest.NewRecorder()
	h.clearLogs(w, req)
	if w.Code != 200 {
		t.Fatalf("clear expected 200, got %d", w.Code)
	}
	logs, _ := logRepo.GetAll()
	if len(logs) != 0 {
		t.Errorf("logs should be cleared, got %d", len(logs))
	}
}
