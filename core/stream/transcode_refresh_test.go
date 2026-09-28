package stream

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/assert"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// fakeLinkStorage hands out incrementing signed links, like a cloud gateway would.
type fakeLinkStorage struct{ n int }

func (f *fakeLinkStorage) FS() (storage.MusicFS, error) { return nil, nil }
func (f *fakeLinkStorage) DirectURL(_ context.Context, _ string) (string, time.Time, error) {
	f.n++
	return "https://cdn.example/link-" + strconv.Itoa(f.n), time.Time{}, nil
}

// recordingTranscoder is a ffmpeg.FFmpeg stub that records the input it got and
// returns scripted outputs (per-call).
type recordingTranscoder struct {
	inputs  []string
	outputs []string // "" means empty output (expired-link symptom)
	errs    []error  // per-call error (optional)
	calls   int
}

func (r *recordingTranscoder) Transcode(_ context.Context, opts ffmpeg.TranscodeOptions) (io.ReadCloser, error) {
	r.inputs = append(r.inputs, opts.FilePath)
	var err error
	if r.calls < len(r.errs) {
		err = r.errs[r.calls]
	}
	out := ""
	if r.calls < len(r.outputs) {
		out = r.outputs[r.calls]
	}
	r.calls++
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(out)), nil
}
func (r *recordingTranscoder) ExtractImage(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}
func (r *recordingTranscoder) ConvertAnimatedImage(context.Context, io.Reader, int, int) (io.ReadCloser, error) {
	return nil, nil
}
func (r *recordingTranscoder) Probe(context.Context, []string) (string, error) { return "", nil }
func (r *recordingTranscoder) ProbeAudioStream(context.Context, string) (*ffmpeg.AudioProbeResult, error) {
	return nil, nil
}
func (r *recordingTranscoder) CmdPath() (string, error) { return "ffmpeg", nil }
func (r *recordingTranscoder) IsAvailable() bool        { return true }
func (r *recordingTranscoder) IsProbeAvailable() bool   { return true }
func (r *recordingTranscoder) Version() string          { return "fake" }

const fakeScheme = "testcloudtranscode"

var sharedLinks = &fakeLinkStorage{}

func init() {
	storage.Register(fakeScheme, func(url.URL) storage.Storage { return sharedLinks })
}

func cloudJob(t *testing.T, tr ffmpeg.FFmpeg) (*mediaStreamer, *streamJob) {
	t.Helper()
	sharedLinks.n = 0 // fresh link counter per test
	ms := &mediaStreamer{transcoder: tr}
	job := &streamJob{
		ms:       ms,
		mf:       &model.MediaFile{ID: "mf1", LibraryPath: fakeScheme + "://fake/x", Path: "a/b.mp3"},
		filePath: "https://cdn.example/stale-link",
		remote:   true,
	}
	return ms, job
}

// ── tests (整改方案 §4.4 / P1-4) ─────────────────────────────────────────────

func TestTranscodeCloudInputUsesFreshLinkAtStart(t *testing.T) {
	tr := &recordingTranscoder{outputs: []string{"DATA"}}
	ms, job := cloudJob(t, tr)

	out, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.NoError(t, err)
	body, _ := io.ReadAll(out)
	assert.Equal(t, "DATA", string(body))

	// The input must be a freshly resolved link, NOT the link captured at request time.
	assert.Equal(t, []string{"https://cdn.example/link-1"}, tr.inputs)
}

func TestTranscodeRetriesOnceWithFreshLinkOnEmptyOutput(t *testing.T) {
	tr := &recordingTranscoder{outputs: []string{"", "RECOVERED"}} // first: expired link
	ms, job := cloudJob(t, tr)

	out, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.NoError(t, err)
	body, _ := io.ReadAll(out)
	assert.Equal(t, "RECOVERED", string(body))

	assert.Equal(t, 2, tr.calls)
	assert.Equal(t, "https://cdn.example/link-1", tr.inputs[0])
	assert.Equal(t, "https://cdn.example/link-2", tr.inputs[1]) // second attempt re-resolved
}

func TestTranscodeGivesUpAfterOneRetry(t *testing.T) {
	tr := &recordingTranscoder{outputs: []string{"", ""}} // both empty
	ms, job := cloudJob(t, tr)

	_, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.Error(t, err)
	assert.Equal(t, 2, tr.calls) // exactly one retry, no loops
}

func TestTranscodeLocalInputIsUntouched(t *testing.T) {
	tr := &recordingTranscoder{outputs: []string{""}} // empty output
	ms := &mediaStreamer{transcoder: tr}
	job := &streamJob{ms: ms, mf: &model.MediaFile{ID: "mf2"}, filePath: "/music/a.mp3"}

	out, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.NoError(t, err) // passed through as-is (no peek, no retry)
	assert.NotNil(t, out)
	assert.Equal(t, 1, tr.calls) // local jobs never retry
	assert.Equal(t, "/music/a.mp3", tr.inputs[0])
}

func TestTranscodeLocalInputErrorIsNotRetried(t *testing.T) {
	tr := &recordingTranscoder{errs: []error{assert.AnError}}
	ms := &mediaStreamer{transcoder: tr}
	job := &streamJob{ms: ms, mf: &model.MediaFile{ID: "mf3"}, filePath: "/music/a.mp3"}

	_, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.Error(t, err)
	assert.Equal(t, 1, tr.calls)
}

func TestTranscodePeekPreservesOutput(t *testing.T) {
	tr := &recordingTranscoder{outputs: []string{"0123456789"}}
	ms, job := cloudJob(t, tr)

	out, err := transcodeWithRefresh(context.Background(), ms, job, ffmpeg.TranscodeOptions{})
	assert.NoError(t, err)
	body, _ := io.ReadAll(out)
	assert.Equal(t, "0123456789", string(body)) // peeked byte is re-assembled
}
