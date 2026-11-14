package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
