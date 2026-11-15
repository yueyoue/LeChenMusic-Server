package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	onlinesvc "github.com/navidrome/navidrome/server/online"
	"github.com/navidrome/navidrome/tests"
)

// ---- test doubles ----

type stubOnlineSource struct {
	songs []onlinesvc.Song
}

func (s *stubOnlineSource) Name() string        { return "hicn" }
func (s *stubOnlineSource) DisplayName() string { return "stub" }

func (s *stubOnlineSource) SearchSong(ctx context.Context, req onlinesvc.SearchRequest) (*onlinesvc.SearchResult, error) {
	return &onlinesvc.SearchResult{Songs: s.songs, Total: len(s.songs)}, nil
}

func (s *stubOnlineSource) ResolveURL(ctx context.Context, song onlinesvc.SongRef, q onlinesvc.Quality) (*onlinesvc.ResolvedMedia, error) {
	return nil, errors.New("resolve not available in tests")
}

func (s *stubOnlineSource) GetLyrics(ctx context.Context, song onlinesvc.SongRef) (string, error) {
	return "", nil
}

type fakeDownloadTaskRepo struct {
	mu     sync.Mutex
	tasks  map[string]*model.DownloadTask
	nextID int
}

func newFakeDownloadTaskRepo() *fakeDownloadTaskRepo {
	return &fakeDownloadTaskRepo{tasks: map[string]*model.DownloadTask{}}
}

func (f *fakeDownloadTaskRepo) Add(task *model.DownloadTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if task.ID == "" {
		f.nextID++
		task.ID = "task-" + strconv.Itoa(f.nextID)
	}
	if task.Status == "" {
		task.Status = model.DownloadStatusQueued
	}
	cp := *task
	f.tasks[task.ID] = &cp
	return nil
}

func (f *fakeDownloadTaskRepo) Update(task *model.DownloadTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *task
	f.tasks[task.ID] = &cp
	return nil
}

func (f *fakeDownloadTaskRepo) Get(id string) (*model.DownloadTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, model.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeDownloadTaskRepo) GetAll() (model.DownloadTasks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res model.DownloadTasks
	for _, t := range f.tasks {
		res = append(res, *t)
	}
	return res, nil
}

func (f *fakeDownloadTaskRepo) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tasks, id)
	return nil
}

// setupOnlineHandler builds an onlineHandler with fakes and a stub source.
func setupOnlineHandler(t *testing.T, songs []onlinesvc.Song) (*onlineHandler, *fakeDownloadTaskRepo, *tests.MockMediaFileRepo) {
	t.Helper()

	stub := &stubOnlineSource{songs: songs}
	onlinesvc.Register(stub)
	t.Cleanup(func() { onlinesvc.Register(onlinesvc.NewHicnSource("")) })

	taskRepo := newFakeDownloadTaskRepo()
	mediaRepo := tests.CreateMockMediaFileRepo()

	ds := &tests.MockDataStore{
		MockedMediaFile:    mediaRepo,
		MockedDownloadTask: taskRepo,
	}

	conf.Server.OnlineDownload.Enabled = true
	conf.Server.OnlineDownload.Permission = "admin-or-approved"
	conf.Server.OnlineDownload.DailyQuota = 0

	h := newOnlineHandler(ds, nil)
	return h, taskRepo, mediaRepo
}

func onlineRequest(t *testing.T, user model.User) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/online/search", nil)
	return req.WithContext(request.WithUser(req.Context(), user))
}

func TestOnlineSearchMarksInLibrary(t *testing.T) {
	songs := []onlinesvc.Song{
		{Source: "hicn", Platform: "kuwo", ID: "1", Name: "海阔天空", Artist: "BEYOND"},
		{Source: "hicn", Platform: "kuwo", ID: "2", Name: "真的爱你", Artist: "BEYOND"},
	}
	h, _, mediaRepo := setupOnlineHandler(t, songs)
	mediaRepo.SetData(model.MediaFiles{{ID: "mf1", Title: "海阔天空（Live）", Artist: "Beyond"}})

	req := onlineRequest(t, model.User{ID: "u1"})
	q := req.URL.Query()
	q.Set("keyword", "海阔天空")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.search(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp onlineSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Songs) != 2 {
		t.Fatalf("expected 2 songs, got %d", len(resp.Songs))
	}
	if !resp.Songs[0].InLibrary {
		t.Error("fuzzy-identical song should be marked as in library")
	}
	if resp.Songs[1].InLibrary {
		t.Error("song not in library should not be marked")
	}
}

// Search results must tell the clients "already downloading" / "already
// downloaded" so the app and the web console stop offering the download button
// for a song that is already in flight or on disk (before the scan indexes it).
func TestOnlineSearchMarksDownloadingAndDownloaded(t *testing.T) {
	songs := []onlinesvc.Song{
		{Source: "hicn", Platform: "kuwo", ID: "1", Name: "海阔天空", Artist: "BEYOND"},
		{Source: "hicn", Platform: "kuwo", ID: "2", Name: "真的爱你", Artist: "BEYOND"},
		{Source: "hicn", Platform: "kuwo", ID: "3", Name: "光辉岁月", Artist: "BEYOND"},
	}
	h, taskRepo, _ := setupOnlineHandler(t, songs)

	_ = taskRepo.Add(&model.DownloadTask{
		UserID: "u1", Source: "hicn", SongID: "1", SongName: "海阔天空", Artist: "BEYOND",
		Status: model.DownloadStatusDownloading,
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "真的爱你.flac")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = taskRepo.Add(&model.DownloadTask{
		UserID: "u1", Source: "hicn", SongID: "2", SongName: "真的爱你", Artist: "BEYOND",
		Status: model.DownloadStatusDone, FilePath: path,
	})

	req := onlineRequest(t, model.User{ID: "u1"})
	q := req.URL.Query()
	q.Set("keyword", "BEYOND")
	req.URL.RawQuery = q.Encode()
	w := httptest.NewRecorder()
	h.search(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp onlineSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(resp.Songs) != 3 {
		t.Fatalf("expected 3 songs, got %d", len(resp.Songs))
	}
	if !resp.Songs[0].Downloading || resp.Songs[0].InLibrary {
		t.Errorf("downloading song flags wrong: %+v", resp.Songs[0])
	}
	if !resp.Songs[1].InLibrary || resp.Songs[1].Downloading {
		t.Errorf("downloaded song flags wrong: %+v", resp.Songs[1])
	}
	if resp.Songs[2].Downloading || resp.Songs[2].InLibrary {
		t.Errorf("unknown song must be downloadable: %+v", resp.Songs[2])
	}
}

func TestOnlineDownloadRequiresAdmin(t *testing.T) {
	h, taskRepo, _ := setupOnlineHandler(t, nil)

	body := `{"source":"hicn","platform":"kuwo","songId":"1","name":"海阔天空","artist":"BEYOND"}`
	req := httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "u1", UserName: "alice"}))
	w := httptest.NewRecorder()
	h.download(w, req)
	if w.Code != 403 {
		t.Fatalf("non-admin expected 403, got %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "u2", UserName: "root", IsAdmin: true}))
	w = httptest.NewRecorder()
	h.download(w, req)
	if w.Code != 201 {
		t.Fatalf("admin expected 201, got %d: %s", w.Code, w.Body.String())
	}
	tasks, _ := taskRepo.GetAll()
	if len(tasks) != 1 || tasks[0].SongName != "海阔天空" {
		t.Fatalf("task not created correctly: %+v", tasks)
	}
}

func TestOnlineDownloadDedupGate(t *testing.T) {
	h, taskRepo, mediaRepo := setupOnlineHandler(t, nil)
	mediaRepo.SetData(model.MediaFiles{{ID: "mf1", Title: "海阔天空", Artist: "BEYOND"}})

	admin := model.User{ID: "u2", IsAdmin: true}

	body := `{"source":"hicn","platform":"kuwo","songId":"1","name":"海阔天空","artist":"BEYOND"}`
	req := httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), admin))
	w := httptest.NewRecorder()
	h.download(w, req)
	if w.Code != 409 {
		t.Fatalf("duplicate expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// Fuzzy allows other versions.
	body = `{"source":"hicn","platform":"kuwo","songId":"1","name":"海阔天空","artist":"BEYOND","fuzzy":true}`
	req = httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), admin))
	w = httptest.NewRecorder()
	h.download(w, req)
	if w.Code != 201 {
		t.Fatalf("fuzzy download expected 201, got %d: %s", w.Code, w.Body.String())
	}
	tasks, _ := taskRepo.GetAll()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

// The dedup gate must also work before the library scan has indexed a freshly
// downloaded file: the task table is authoritative for "already downloaded".
// One click = one download, from either the web console or the app.
func TestOnlineDownloadDedupGateUsesTaskTable(t *testing.T) {
	h, taskRepo, _ := setupOnlineHandler(t, nil)
	admin := model.User{ID: "u2", IsAdmin: true}

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
		req = req.WithContext(request.WithUser(req.Context(), admin))
		w := httptest.NewRecorder()
		h.download(w, req)
		return w
	}

	// 1) Same upstream song already downloading: rejected even with fuzzy on.
	_ = taskRepo.Add(&model.DownloadTask{
		UserID: "u2", Source: "hicn", SongID: "7", SongName: "海阔天空", Artist: "BEYOND",
		Status: model.DownloadStatusDownloading,
	})
	w := post(`{"source":"hicn","platform":"kuwo","songId":"7","name":"海阔天空","artist":"BEYOND","fuzzy":true}`)
	if w.Code != 409 {
		t.Fatalf("same song while downloading expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "正在下载") {
		t.Errorf("expected a downloading reason, got %q", w.Body.String())
	}

	// 2) Same title+artist already downloading (different upstream id): also a
	//    duplicate click, not a new version.
	_ = taskRepo.Add(&model.DownloadTask{
		UserID: "u2", Source: "hicn", SongID: "8", SongName: "真的爱你", Artist: "BEYOND",
		Status: model.DownloadStatusQueued,
	})
	w = post(`{"source":"hicn","platform":"kuwo","songId":"88","name":"真的爱你","artist":"BEYOND","fuzzy":true}`)
	if w.Code != 409 {
		t.Fatalf("same title/artist while queued expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// 3) Finished download, file still on disk, library not indexed yet -> 409.
	dir := t.TempDir()
	existing := filepath.Join(dir, "光辉岁月.flac")
	if err := os.WriteFile(existing, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = taskRepo.Add(&model.DownloadTask{
		UserID: "u2", Source: "hicn", SongID: "9", SongName: "光辉岁月", Artist: "BEYOND",
		Status: model.DownloadStatusDone, FilePath: existing,
	})
	w = post(`{"source":"hicn","platform":"kuwo","songId":"9","name":"光辉岁月","artist":"BEYOND"}`)
	if w.Code != 409 {
		t.Fatalf("already downloaded expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "已下载") {
		t.Errorf("expected an already-downloaded reason, got %q", w.Body.String())
	}

	// Fuzzy still allows downloading a *different* version (other upstream id).
	w = post(`{"source":"hicn","platform":"kuwo","songId":"10","name":"光辉岁月","artist":"BEYOND","fuzzy":true}`)
	if w.Code != 201 {
		t.Fatalf("fuzzy other version expected 201, got %d: %s", w.Code, w.Body.String())
	}
	// Undo that queued task so it can't mask the next check.
	queued, _ := taskRepo.GetAll()
	for _, tk := range queued {
		if tk.SongID == "10" {
			_ = taskRepo.Delete(tk.ID)
		}
	}
	// Without fuzzy the same title/artist is still a duplicate.
	w = post(`{"source":"hicn","platform":"kuwo","songId":"11","name":"光辉岁月","artist":"BEYOND"}`)
	if w.Code != 409 {
		t.Fatalf("same song without fuzzy expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// 4) Song deleted from the NAS (file gone): downloadable again.
	if err := os.Remove(existing); err != nil {
		t.Fatal(err)
	}
	w = post(`{"source":"hicn","platform":"kuwo","songId":"9","name":"光辉岁月","artist":"BEYOND"}`)
	if w.Code != 201 {
		t.Fatalf("deleted song should be downloadable again, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOnlineDownloadQuota(t *testing.T) {
	h, taskRepo, _ := setupOnlineHandler(t, nil)
	conf.Server.OnlineDownload.Permission = "quota"
	conf.Server.OnlineDownload.DailyQuota = 1

	user := model.User{ID: "u1"}
	_ = taskRepo.Add(&model.DownloadTask{UserID: "u1", SongName: "first", CreatedAt: time.Now()})

	body := `{"source":"hicn","platform":"kuwo","songId":"9","name":"second","artist":"x"}`
	req := httptest.NewRequest("POST", "/api/online/download", strings.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), user))
	w := httptest.NewRecorder()
	h.download(w, req)
	if w.Code != 429 {
		t.Fatalf("expected 429 quota reached, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOnlineTasksOwnershipAndDelete(t *testing.T) {
	h, taskRepo, _ := setupOnlineHandler(t, nil)

	_ = taskRepo.Add(&model.DownloadTask{UserID: "u1", SongName: "mine", Status: model.DownloadStatusFailed})
	_ = taskRepo.Add(&model.DownloadTask{UserID: "u2", SongName: "theirs", Status: model.DownloadStatusFailed})

	req := onlineRequest(t, model.User{ID: "u1"})
	w := httptest.NewRecorder()
	h.listTasks(w, req)
	var tasks model.DownloadTasks
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("parse tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].SongName != "mine" {
		t.Fatalf("non-admin should only see own tasks: %+v", tasks)
	}

	req = onlineRequest(t, model.User{ID: "admin", IsAdmin: true})
	w = httptest.NewRecorder()
	h.listTasks(w, req)
	tasks = nil
	_ = json.Unmarshal(w.Body.Bytes(), &tasks)
	if len(tasks) != 2 {
		t.Fatalf("admin should see all tasks: %+v", tasks)
	}

	// Delete: owner can delete own task via route param
	var target string
	for _, tk := range tasks {
		if tk.SongName == "mine" {
			target = tk.ID
		}
	}
	req = httptest.NewRequest("DELETE", "/api/online/tasks/"+target, nil)
	req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "u1"}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", target)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w = httptest.NewRecorder()
	h.deleteTask(w, req)
	if w.Code != 200 {
		t.Fatalf("delete expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := taskRepo.Get(target); err == nil {
		t.Error("task should be deleted")
	}
}

// deleteTaskWithID drives the per-user delete handler (the path the mobile app
// uses) against a task id.
func deleteTaskWithID(t *testing.T, h *onlineHandler, id string, user model.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", "/api/online/tasks/"+id, nil)
	req = req.WithContext(request.WithUser(req.Context(), user))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.deleteTask(w, req)
	return w
}

// A finished download deleted from the app must delete the audio file on the NAS
// (and its library record), not just the download record.
func TestOnlineDeleteTaskRemovesFileFromDisk(t *testing.T) {
	libDir := t.TempDir()
	oldLib, oldMusic := conf.Server.OnlineDownload.LibPath, conf.Server.MusicFolder
	conf.Server.OnlineDownload.LibPath = libDir
	conf.Server.MusicFolder = libDir
	t.Cleanup(func() {
		conf.Server.OnlineDownload.LibPath = oldLib
		conf.Server.MusicFolder = oldMusic
	})

	h, taskRepo, _ := setupOnlineHandler(t, nil)
	path := filepath.Join(libDir, "海阔天空.flac")
	if err := os.WriteFile(path, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		UserID:   "u1",
		SongName: "海阔天空",
		Artist:   "BEYOND",
		Status:   model.DownloadStatusDone,
		FilePath: path,
		Source:   "hicn",
	}
	if err := taskRepo.Add(task); err != nil {
		t.Fatal(err)
	}

	w := deleteTaskWithID(t, h, task.ID, model.User{ID: "u1"})
	if w.Code != 200 {
		t.Fatalf("delete expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("audio file should be gone from disk, stat err = %v", err)
	}
	if _, err := taskRepo.Get(task.ID); err == nil {
		t.Error("task record should be deleted")
	}
	var resp deleteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.DeletedFile {
		t.Errorf("response should report the file as deleted: %+v", resp)
	}

	// Idempotent: a second delete of an already-removed file must not fail.
	task2 := &model.DownloadTask{
		UserID: "u1", SongName: "真的爱你", Status: model.DownloadStatusDone, FilePath: path,
	}
	if err := taskRepo.Add(task2); err != nil {
		t.Fatal(err)
	}
	w = deleteTaskWithID(t, h, task2.ID, model.User{ID: "u1"})
	if w.Code != 200 {
		t.Fatalf("idempotent delete expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := taskRepo.Get(task2.ID); err == nil {
		t.Error("task record should be deleted even when the file is already gone")
	}
}

// Regression: the 201 response of POST /api/online/download must advertise
// application/json on the wire.
//
// The handler used to call w.WriteHeader(201) and only then let writeJSON set
// Content-Type. WriteHeader commits the headers, so Go's content sniffer
// labelled the task JSON as "text/plain; charset=utf-8"; the Android app's
// converter rejects text/plain, so a download that had actually been queued was
// reported to the user as "服务器返回了非JSON响应".
//
// A real server is used on purpose: httptest.ResponseRecorder shares the
// handler's header map, so it would report application/json even with the bug.
func TestOnlineDownloadCreatedResponseIsJSON(t *testing.T) {
	h, _, _ := setupOnlineHandler(t, nil)
	admin := model.User{ID: "u2", IsAdmin: true}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.download(w, r.WithContext(request.WithUser(r.Context(), admin)))
	}))
	defer srv.Close()

	body := `{"source":"hicn","platform":"kuwo","songId":"501","name":"大梦归","artist":"周深"}`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("created response must be JSON, got Content-Type %q", ct)
	}
	var task model.DownloadTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		t.Fatalf("response body should be a JSON task: %v", err)
	}
	if task.ID == "" {
		t.Error("response should carry the created task id")
	}
}
