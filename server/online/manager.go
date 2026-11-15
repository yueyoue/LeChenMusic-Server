package online

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// PostProcessor finalizes a downloaded temp file: tagging, cover, lyrics and
// moving it into the media library. It returns the final file path.
// (Implemented in M3; until then the default keeps the file as-is.)
type PostProcessor func(ctx context.Context, task *model.DownloadTask, tmpPath string) (string, error)

// SettledObserver is notified after a task reaches a terminal state
// (done or failed), so callers can e.g. write operation logs (M5).
type SettledObserver func(ctx context.Context, task *model.DownloadTask, err error)

// Manager owns the download task queue: it persists tasks, processes them with
// a single worker goroutine (configurable later), reports progress and cleans
// up temp files on failure.
type Manager struct {
	tasks     model.DownloadTaskRepository
	tmpDir    string
	processor PostProcessor
	client    *http.Client
	settled   SettledObserver

	queue chan string
	wg    sync.WaitGroup

	// TaskJitter is the base delay spread randomly between queued tasks
	// (actual delay ∈ [J/2, 3J/2)) to avoid hammering the upstream source
	// during batch downloads. Set 0 to disable.
	TaskJitter time.Duration
}

// NewManager creates a download task manager. tmpDir is where partial
// downloads are stored; processor finalizes completed downloads (may be nil).
func NewManager(tasks model.DownloadTaskRepository, tmpDir string, processor PostProcessor) *Manager {
	if processor == nil {
		processor = func(_ context.Context, _ *model.DownloadTask, tmpPath string) (string, error) {
			return tmpPath, nil
		}
	}
	return &Manager{
		tasks:      tasks,
		tmpDir:     tmpDir,
		processor:  processor,
		client:     &http.Client{Timeout: 30 * time.Minute},
		queue:      make(chan string, 1024),
		TaskJitter: 5 * time.Second,
	}
}

// Start launches the worker and re-queues tasks left over from previous runs.
// The worker stops when ctx is cancelled.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.worker(ctx)

	if all, err := m.tasks.GetAll(); err == nil {
		for _, t := range all {
			if t.Status == model.DownloadStatusQueued {
				m.enqueueID(t.ID)
			}
		}
	}
}

// Wait blocks until the worker has stopped (for tests/shutdown).
func (m *Manager) Wait() { m.wg.Wait() }

// OnSettled registers an observer invoked when a task finishes (done or failed).
func (m *Manager) OnSettled(fn SettledObserver) { m.settled = fn }

// Enqueue persists a new task and schedules it.
func (m *Manager) Enqueue(task *model.DownloadTask) error {
	task.Status = model.DownloadStatusQueued
	if err := m.tasks.Add(task); err != nil {
		return err
	}
	m.enqueueID(task.ID)
	return nil
}

// Retry re-queues a failed task, keeping its original parameters.
func (m *Manager) Retry(ctx context.Context, taskID string) error {
	task, err := m.tasks.Get(taskID)
	if err != nil {
		return err
	}
	if task.Status != model.DownloadStatusFailed {
		return fmt.Errorf("task %s is not failed (status: %s)", taskID, task.Status)
	}
	task.Status = model.DownloadStatusQueued
	task.Error = ""
	task.ProgressBytes = 0
	if err := m.tasks.Update(task); err != nil {
		return err
	}
	m.enqueueID(taskID)
	log.Info(ctx, "Download task re-queued", "task", taskID)
	return nil
}

func (m *Manager) enqueueID(taskID string) {
	select {
	case m.queue <- taskID:
	default:
		// Queue full: the task stays "queued" in the DB and will be resumed
		// on the next Start().
		log.Warn(context.Background(), "Download queue full, task deferred", "task", taskID)
	}
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case taskID := <-m.queue:
			// Batch-friendly pacing: when more tasks are already waiting, pause a
			// random amount between tasks so downloads don't fire back-to-back.
			if len(m.queue) > 0 && m.TaskJitter > 0 {
				d := m.TaskJitter/2 + time.Duration(rand.Int63n(int64(m.TaskJitter)))
				select {
				case <-ctx.Done():
					return
				case <-time.After(d):
				}
			}
			m.process(ctx, taskID)
		}
	}
}

func (m *Manager) process(ctx context.Context, taskID string) {
	task, err := m.tasks.Get(taskID)
	if err != nil {
		log.Error(ctx, "Could not load download task", "task", taskID, err)
		return
	}
	if task.Status != model.DownloadStatusQueued {
		return
	}
	// Keep the run error in its own variable: `if err := ...` would shadow the
	// outer err and the settled observer would always see nil (M5 log bug).
	runErr := m.run(ctx, task)
	if runErr != nil {
		task.Status = model.DownloadStatusFailed
		task.Error = runErr.Error()
		task.FinishedAt = time.Now()
		if updateErr := m.tasks.Update(task); updateErr != nil {
			log.Error(ctx, "Could not mark download task as failed", "task", taskID, updateErr)
		}
		log.Warn(ctx, "Download task failed", "task", taskID, "song", task.SongName, runErr)
	}
	if m.settled != nil {
		m.settled(ctx, task, runErr)
	}
}

// run executes one task through: resolve → download → post-process.
func (m *Manager) run(ctx context.Context, task *model.DownloadTask) error {
	src, ok := Get(task.Source)
	if !ok {
		return fmt.Errorf("unknown download source %q", task.Source)
	}

	task.Status = model.DownloadStatusResolving
	if err := m.tasks.Update(task); err != nil {
		return err
	}

	media, quality, err := m.resolveWithFallback(ctx, src, task)
	if err != nil {
		return fmt.Errorf("resolve media URL: %w", err)
	}
	task.ActualFormat = media.Format
	task.ActualBitrate = media.Bitrate
	if task.ActualFormat == "" {
		task.ActualFormat = quality.Format
		task.ActualBitrate = quality.Bitrate
	}
	task.Status = model.DownloadStatusDownloading
	if err := m.tasks.Update(task); err != nil {
		return err
	}

	tmpPath, err := m.download(ctx, media, task)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}

	task.Status = model.DownloadStatusTagging
	if err := m.tasks.Update(task); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	finalPath, err := m.processor(ctx, task, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("post-process: %w", err)
	}

	task.Status = model.DownloadStatusDone
	task.FilePath = finalPath
	task.FinishedAt = time.Now()
	return m.tasks.Update(task)
}

// resolveWithFallback tries the requested quality first (if any), then the
// default chain: FLAC → MP3 320k → MP3 128k.
func (m *Manager) resolveWithFallback(ctx context.Context, src Source, task *model.DownloadTask) (*ResolvedMedia, Quality, error) {
	want := parseQuality(task.RequestedQuality)
	chain := make([]Quality, 0, 4)
	if !want.IsZero() {
		chain = append(chain, want)
	}
	for _, q := range DefaultQualityChain {
		if q != want {
			chain = append(chain, q)
		}
	}

	ref := taskRef(task)
	var lastErr error
	for _, q := range chain {
		media, err := src.ResolveURL(ctx, ref, q)
		if err == nil {
			return media, q, nil
		}
		lastErr = err
	}
	return nil, Quality{}, lastErr
}

// download fetches media.URL into a temp file, reporting progress to the task.
// On error the temp file is removed and an empty path is returned.
func (m *Manager) download(ctx context.Context, media *ResolvedMedia, task *model.DownloadTask) (string, error) {
	if err := os.MkdirAll(m.tmpDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(m.tmpDir, "dl-"+task.ID+"-*.part")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer tmp.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, media.URL, nil)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("media server returned status %d", resp.StatusCode)
	}

	task.TotalBytes = resp.ContentLength
	progress := &progressReader{
		Reader: resp.Body,
		total:  resp.ContentLength,
		onProgress: func(read, total int64) {
			task.ProgressBytes = read
			task.TotalBytes = total
			if err := m.tasks.Update(task); err != nil {
				log.Trace(ctx, "Could not persist download progress", "task", task.ID, err)
			}
		},
	}
	if _, err := io.Copy(tmp, progress); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	task.ProgressBytes = progress.read
	if err := m.tasks.Update(task); err != nil {
		log.Trace(ctx, "Could not persist download progress", "task", task.ID, err)
	}
	return tmpPath, nil
}

func taskRef(task *model.DownloadTask) SongRef {
	return SongRef{
		Source:   task.Source,
		Platform: task.Platform,
		ID:       task.SongID,
		Time:     task.UpstreamTime,
		Sign:     task.Sign,
	}
}

// parseQuality parses "flac/2000" into a Quality. Empty string → zero Quality.
func parseQuality(s string) Quality {
	s = strings.TrimSpace(s)
	if s == "" {
		return Quality{}
	}
	format, bitrateStr, ok := strings.Cut(s, "/")
	if !ok {
		return Quality{Format: strings.ToLower(format)}
	}
	bitrate, err := strconv.Atoi(strings.TrimSpace(bitrateStr))
	if err != nil {
		return Quality{Format: strings.ToLower(format)}
	}
	return Quality{Format: strings.ToLower(strings.TrimSpace(format)), Bitrate: bitrate}
}

// FormatQuality renders a Quality as "flac/2000" (used for task persistence).
func FormatQuality(q Quality) string {
	if q.IsZero() {
		return ""
	}
	return q.Format + "/" + strconv.Itoa(q.Bitrate)
}

// progressReader reports download progress at most every 500ms.
type progressReader struct {
	io.Reader
	total      int64
	read       int64
	lastReport time.Time
	onProgress func(read, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.Reader.Read(b)
	p.read += int64(n)
	if p.onProgress != nil && time.Since(p.lastReport) >= 500*time.Millisecond {
		p.lastReport = time.Now()
		p.onProgress(p.read, p.total)
	}
	return n, err
}
