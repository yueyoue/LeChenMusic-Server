package online

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/navidrome/navidrome/model"
)

// scriptedSource resolves URLs from a script: the first adCalls resolve calls
// return the observed upstream ad placeholder (mp3, bitrate=1, ~12.8s), the
// rest return a normal-looking media URL.
type scriptedSource struct {
	name     string
	mediaURL string

	mu       sync.Mutex
	calls    int
	adCalls  int // resolves that return the ad payload
	invCount int // InvalidateURL calls
	trackInv bool
	resolved func(call int, q Quality) (*ResolvedMedia, error)
}

func (s *scriptedSource) Name() string        { return s.name }
func (s *scriptedSource) DisplayName() string { return s.name }

func (s *scriptedSource) SearchSong(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	return &SearchResult{}, nil
}

func (s *scriptedSource) GetLyrics(ctx context.Context, song SongRef) (string, error) {
	return "", nil
}

func (s *scriptedSource) ResolveURL(ctx context.Context, song SongRef, q Quality) (*ResolvedMedia, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if s.resolved != nil {
		return s.resolved(call, q)
	}
	if call <= s.adCalls {
		return &ResolvedMedia{URL: s.mediaURL + "/ad.mp3", Format: "mp3", Bitrate: 1, Duration: 12}, nil
	}
	return &ResolvedMedia{URL: s.mediaURL + "/song.flac", Format: q.Format, Bitrate: q.Bitrate, Duration: 245}, nil
}

func (s *scriptedSource) InvalidateURL(song SongRef, q Quality) {
	if !s.trackInv {
		return
	}
	s.mu.Lock()
	s.invCount++
	s.mu.Unlock()
}

func (s *scriptedSource) invalidations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invCount
}

// adBytes is the audio of the observed upstream ad: ~12.8s of MP3 frames.
func adBytes(t *testing.T) []byte { return mp3Frames(t, 493) }

// goodBytes is a full-length track: 245s FLAC (duration lives in STREAMINFO).
func goodBytes(t *testing.T) []byte { return flacWithDuration(t, 44100, 245*44100) }

// setupPlaceholderManager wires a Manager to a scripted source plus a media
// server whose body for /ad.mp3 and /song.flac is controlled per test.
func setupPlaceholderManager(t *testing.T, src *scriptedSource, ad, good http.HandlerFunc) (*Manager, *fakeTaskRepo, *[]string) {
	t.Helper()
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ad.mp3") && ad != nil:
			ad(w, r)
		case strings.HasSuffix(r.URL.Path, "/song.flac") && good != nil:
			good(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(media.Close)
	src.mediaURL = media.URL

	Register(src)
	t.Cleanup(func() { unregister(src.Name()) })

	repo := newFakeTaskRepo()
	var processed []string
	var mu sync.Mutex
	processor := func(_ context.Context, task *model.DownloadTask, tmpPath string) (string, error) {
		mu.Lock()
		processed = append(processed, tmpPath)
		mu.Unlock()
		return tmpPath, nil
	}
	mgr := NewManager(repo, filepath.Join(t.TempDir(), "tmp"), processor)
	mgr.PlaceholderRetryDelay = 0 // no waiting in tests
	return mgr, repo, &processed
}

func waitSettled(t *testing.T, repo *fakeTaskRepo, id string) *model.DownloadTask {
	t.Helper()
	var got *model.DownloadTask
	waitFor(t, "task settled", func() bool {
		task, err := repo.Get(id)
		if err != nil {
			return false
		}
		if task.Status == model.DownloadStatusDone || task.Status == model.DownloadStatusFailed {
			got = task
			return true
		}
		return false
	})
	return got
}

func TestManagerAdResolveIsRetriedThenIngested(t *testing.T) {
	// First two resolves (one full quality chain) return the ad payload; the
	// retry re-resolves and gets the real track.
	src := &scriptedSource{name: "ad-retry", adCalls: 2, trackInv: true}
	mgr, repo, processed := setupPlaceholderManager(t, src,
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(adBytes(t)) },
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(goodBytes(t)) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "ad-retry", Platform: "p", SongID: "1", SongName: "真心", Artist: "叶蒨文", Duration: 245}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := waitSettled(t, repo, task.ID)
	if got.Status != model.DownloadStatusDone {
		t.Fatalf("task should succeed after retry: %s (%s)", got.Error, got.Status)
	}
	if len(*processed) != 1 {
		t.Fatalf("processor should run once, got %d", len(*processed))
	}
	if src.invalidations() == 0 {
		t.Error("expected cached URLs to be invalidated on placeholder hits")
	}
}

func TestManagerPersistentAdResolveFailsWithoutIngest(t *testing.T) {
	src := &scriptedSource{name: "ad-stuck", adCalls: 1 << 30}
	mgr, repo, processed := setupPlaceholderManager(t, src,
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(adBytes(t)) },
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(goodBytes(t)) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "ad-stuck", Platform: "p", SongID: "2", SongName: "不要说走就走", Artist: "郭峰", Duration: 245}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := waitSettled(t, repo, task.ID)
	if got.Status != model.DownloadStatusFailed {
		t.Fatal("task must fail when the upstream keeps serving the ad")
	}
	if !strings.Contains(got.Error, "广告占位音频") {
		t.Errorf("error should explain the ad placeholder, got: %s", got.Error)
	}
	if len(*processed) != 0 {
		t.Errorf("ad audio must never be ingested, processor ran %d times", len(*processed))
	}
}

func TestManagerAdFileDownloadIsRetriedThenIngested(t *testing.T) {
	// Resolve responses look plausible; the bytes are the ad. The post-download
	// duration probe must catch it and retry.
	var mu sync.Mutex
	adServed := 0
	src := &scriptedSource{name: "ad-file", adCalls: 0}
	mgr, repo, processed := setupPlaceholderManager(t, src,
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			adServed++
			mu.Unlock()
			_, _ = w.Write(adBytes(t))
		},
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(goodBytes(t)) },
	)

	// Serve the ad for the first download only: the media URL path decides.
	src.resolved = func(call int, q Quality) (*ResolvedMedia, error) {
		mu.Lock()
		first := adServed == 0
		mu.Unlock()
		suffix := "/song.flac"
		if first {
			suffix = "/ad.mp3"
		}
		return &ResolvedMedia{URL: src.mediaURL + suffix, Format: q.Format, Bitrate: q.Bitrate, Duration: 245}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "ad-file", Platform: "p", SongID: "3", SongName: "真心", Artist: "叶蒨文", Duration: 245}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := waitSettled(t, repo, task.ID)
	if got.Status != model.DownloadStatusDone {
		t.Fatalf("task should succeed after redownload: %s (%s)", got.Error, got.Status)
	}
	if len(*processed) != 1 {
		t.Fatalf("processor should run once (never on the ad file), got %d", len(*processed))
	}
}

func TestManagerPersistentAdFileFailsWithoutIngest(t *testing.T) {
	// Resolve always looks fine, the downloaded bytes never do.
	src := &scriptedSource{name: "ad-file-stuck"}
	src.resolved = func(call int, q Quality) (*ResolvedMedia, error) {
		return &ResolvedMedia{URL: src.mediaURL + "/ad.mp3", Format: q.Format, Bitrate: q.Bitrate, Duration: 245}, nil
	}
	mgr, repo, processed := setupPlaceholderManager(t, src,
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(adBytes(t)) },
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(goodBytes(t)) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "ad-file-stuck", Platform: "p", SongID: "4", SongName: "真心", Artist: "叶蒨文", Duration: 245}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := waitSettled(t, repo, task.ID)
	if got.Status != model.DownloadStatusFailed {
		t.Fatal("task must fail when every download is the ad")
	}
	if !strings.Contains(got.Error, "广告占位音频") {
		t.Errorf("error should explain the ad placeholder, got: %s", got.Error)
	}
	if len(*processed) != 0 {
		t.Errorf("ad audio must never be ingested, processor ran %d times", len(*processed))
	}
}

func TestManagerShortClipWithoutExpectationFails(t *testing.T) {
	// Older clients don't send an expected duration: the short-clip heuristic
	// still refuses to ingest a 12.8s placeholder as a full track.
	src := &scriptedSource{name: "ad-short"}
	src.resolved = func(call int, q Quality) (*ResolvedMedia, error) {
		// Payload carries no duration → resolve-time check passes.
		return &ResolvedMedia{URL: src.mediaURL + "/ad.mp3", Format: q.Format, Bitrate: q.Bitrate}, nil
	}
	mgr, repo, processed := setupPlaceholderManager(t, src,
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(adBytes(t)) },
		func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(goodBytes(t)) },
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	task := &model.DownloadTask{Source: "ad-short", Platform: "p", SongID: "5", SongName: "未知时长", Artist: "Tester"}
	if err := mgr.Enqueue(task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got := waitSettled(t, repo, task.ID)
	if got.Status != model.DownloadStatusFailed {
		t.Fatal("short placeholder clip must not be ingested")
	}
	if len(*processed) != 0 {
		t.Errorf("placeholder must never be ingested, processor ran %d times", len(*processed))
	}
}
