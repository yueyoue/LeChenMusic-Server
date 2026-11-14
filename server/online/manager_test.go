package online

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

// ---- fakes ----

type fakeTaskRepo struct {
	mu     sync.Mutex
	tasks  map[string]*model.DownloadTask
	status map[string]string // id -> last persisted status snapshot
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{tasks: map[string]*model.DownloadTask{}, status: map[string]string{}}
}

func (f *fakeTaskRepo) Add(task *model.DownloadTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if task.ID == "" {
		task.ID = "t" + time.Now().Format("150405.000000000")
	}
	if task.Status == "" {
		task.Status = model.DownloadStatusQueued
	}
	cp := *task
	f.tasks[task.ID] = &cp
	return nil
}

func (f *fakeTaskRepo) Update(task *model.DownloadTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tasks[task.ID]; !ok {
		return model.ErrNotFound
	}
	cp := *task
	f.tasks[task.ID] = &cp
	return nil
}

func (f *fakeTaskRepo) Get(id string) (*model.DownloadTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, model.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTaskRepo) GetAll() (model.DownloadTasks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res model.DownloadTasks
	for _, t := range f.tasks {
		res = append(res, *t)
	}
	return res, nil
}

func (f *fakeTaskRepo) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.tasks, id)
	return nil
}

// mockSource is a Source whose ResolveURL/GetLyrics behavior is scripted.
type mockSource struct {
	resolveErr  error
	resolveURL  string
	lyrics      string
	lyricsErr   error
	resolveChan chan struct{}
}

func (s *mockSource) Name() string        { return "mock" }
func (s *mockSource) DisplayName() string { return "Mock Source" }

func (s *mockSource) SearchSong(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	return &SearchResult{}, nil
}

func (s *mockSource) ResolveURL(ctx context.Context, song SongRef, q Quality) (*ResolvedMedia, error) {
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return &ResolvedMedia{URL: s.resolveURL, Format: q.Format, Bitrate: q.Bitrate}, nil
}

func (s *mockSource) GetLyrics(ctx context.Context, song SongRef) (string, error) {
	return s.lyrics, s.lyricsErr
}

// ---- tests ----

func setupManager(t *testing.T, src Source, mediaHandler http.HandlerFunc) (*Manager, *fakeTaskRepo, string) {
	t.Helper()
	media := httptest.NewServer(mediaHandler)
	t.Cleanup(media.Close)

	if src != nil {
		if ms, ok := src.(*mockSource); ok && ms.resolveURL == "" {
			ms.resolveURL = media.URL + "/song.flac"
		}
		Register(src)
		t.Cleanup(func() { unregister(src.Name()) })
	}

	repo := newFakeTaskRepo()
	tmpDir := filepath.Join(t.TempDir(), "tmp")
	finalDir := filepath.Join(t.TempDir(), "final")
	processor := func(_ context.Context, task *model.DownloadTask, tmpPath string) (string, error) {
		if err := os.MkdirAll(finalDir, 0o755); err != nil {
			return "", err
		}
		final := filepath.Join(finalDir, task.ID+".flac")
		if err := os.Rename(tmpPath, final); err != nil {
			return "", err
		}
		return final, nil
	}
	return NewManager(repo, tmpDir, processor), repo, tmpDir
}

func unregister(name string) {
	delete(sources, name)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestManagerHappyPath(t *testing.T) {
	body := make([]byte, 64*1024)
	mgr, repo, tmpDir := setupManager(t, &mockSource{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "65536")
		_, _ = w.Write(body)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{
		Source: "mock", Platform: "mock", SongID: "1",
		SongName: "Test Song", Artist: "Tester",
		RequestedQuality: "flac/2000",
	}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var done *model.DownloadTask
	waitFor(t, "task done", func() bool {
		got, err := repo.Get(task.ID)
		if err != nil {
			return false
		}
		if got.Status == model.DownloadStatusDone || got.Status == model.DownloadStatusFailed {
			done = got
			return true
		}
		return false
	})

	if done.Status != model.DownloadStatusDone {
		t.Fatalf("task failed: %s (%s)", done.Error, done.Status)
	}
	if done.ActualFormat != "flac" || done.ActualBitrate != 2000 {
		t.Errorf("unexpected actual quality: %s/%d", done.ActualFormat, done.ActualBitrate)
	}
	if done.TotalBytes != 65536 || done.ProgressBytes != 65536 {
		t.Errorf("unexpected progress: %d/%d", done.ProgressBytes, done.TotalBytes)
	}
	if done.FilePath == "" {
		t.Error("expected final file path")
	}
	if _, err := os.Stat(done.FilePath); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	// temp dir must be empty after success (file moved by processor)
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("temp dir not clean: %v", entries)
	}
}

func TestManagerDownloadFailureCleansUp(t *testing.T) {
	mgr, repo, tmpDir := setupManager(t, &mockSource{}, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "mock", Platform: "mock", SongID: "2", SongName: "Fails"}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var failed *model.DownloadTask
	waitFor(t, "task failed", func() bool {
		got, err := repo.Get(task.ID)
		if err != nil {
			return false
		}
		if got.Status == model.DownloadStatusFailed {
			failed = got
			return true
		}
		return false
	})
	if failed.Error == "" {
		t.Error("expected error message on failed task")
	}
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("temp dir not clean after failure: %v", entries)
	}
}

func TestManagerRetry(t *testing.T) {
	var fail atomicBool
	fail.Set(true)
	mgr, repo, _ := setupManager(t, &mockSource{}, func(w http.ResponseWriter, r *http.Request) {
		if fail.Get() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("audio-bytes"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "mock", Platform: "mock", SongID: "3", SongName: "Retry Me"}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	waitFor(t, "first failure", func() bool {
		got, _ := repo.Get(task.ID)
		return got != nil && got.Status == model.DownloadStatusFailed
	})

	fail.Set(false)
	if err := mgr.Retry(ctx, task.ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	waitFor(t, "retry succeeds", func() bool {
		got, _ := repo.Get(task.ID)
		return got != nil && got.Status == model.DownloadStatusDone
	})
}

func TestManagerResumeQueuedTasks(t *testing.T) {
	mgr, repo, _ := setupManager(t, &mockSource{}, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio-bytes"))
	})

	// Simulate tasks left over from a previous run.
	seed := &model.DownloadTask{Source: "mock", Platform: "mock", SongID: "4", SongName: "Resumed", Status: model.DownloadStatusQueued}
	if err := repo.Add(seed); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	waitFor(t, "resumed task done", func() bool {
		got, _ := repo.Get(seed.ID)
		return got != nil && got.Status == model.DownloadStatusDone
	})
}

func TestManagerResolveFallbackChain(t *testing.T) {
	// First ResolveURL call for flac fails, mp3 succeeds via the chain.
	src := &flakySource{failFor: map[Quality]bool{{Format: "flac", Bitrate: 2000}: true}}
	Register(src)
	t.Cleanup(func() { unregister(src.Name()) })

	mgr := NewManager(newFakeTaskRepo(), t.TempDir(), nil)
	media, q, err := mgr.resolveWithFallback(context.Background(), src, &model.DownloadTask{
		Source: "flaky", Platform: "p", SongID: "1",
	})
	if err != nil {
		t.Fatalf("resolveWithFallback: %v", err)
	}
	if media == nil || q.Format != "mp3" || q.Bitrate != 320 {
		t.Fatalf("expected mp3/320 fallback, got %+v %+v", media, q)
	}
}

type flakySource struct {
	mockSource
	failFor map[Quality]bool
}

func (s *flakySource) Name() string { return "flaky" }

func (s *flakySource) ResolveURL(ctx context.Context, song SongRef, q Quality) (*ResolvedMedia, error) {
	if s.failFor[q] {
		return nil, errors.New("quality unavailable")
	}
	return &ResolvedMedia{URL: "http://example.com/x", Format: q.Format, Bitrate: q.Bitrate}, nil
}

// atomicBool is a tiny mutex-guarded bool for tests.
type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) Set(v bool) { b.mu.Lock(); b.v = v; b.mu.Unlock() }
func (b *atomicBool) Get() bool  { b.mu.Lock(); defer b.mu.Unlock(); return b.v }
