package cloudsource

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/navidrome/navidrome/adapters/gotaglib" // registers the real taglib extractor (ReadTags parity)
	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
	_ "github.com/navidrome/navidrome/core/storage/local" // registers the local (file://) backend
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Storage 一致性测试套件（评审 P2-6）
//
// 同一套 MusicFS 契约用例分别跑 local（os.DirFS 适配）与 cloud（fakeOpenList 网关）
// 两个适配器，保证“本地库行为不变、云库行为对齐”可回归；并以全链路用例验证
// ServeFile 的 302（→CDN）与中转两条路径字节一致。
// ---------------------------------------------------------------------------

const conformanceFixture = "../../tests/fixtures/01 Invisible (RED) Edit Version.mp3"

const conformanceTrack = "artist/album/track.mp3"

// conformanceTree is the shared fixture layout both backends must serve identically.
var conformanceTree = []string{
	conformanceTrack,
	"artist/album/cover.jpg",
	"artist/album2/extra.mp3",
}

func conformanceFiles(t *testing.T) map[string][]byte {
	t.Helper()
	mp3, err := os.ReadFile(filepath.FromSlash(conformanceFixture))
	require.NoError(t, err, "fixture missing")
	return map[string][]byte{
		conformanceTrack:          mp3,
		"artist/album/cover.jpg":  []byte("cover-bytes-not-an-image"),
		"artist/album2/extra.mp3": mp3,
	}
}

// localBackend writes the tree into a temp dir and serves it through the local
// storage adapter (the production path for local libraries).
func localBackend(t *testing.T, files map[string][]byte) (string, storage.MusicFS) {
	t.Helper()
	root := t.TempDir()
	for rel, data := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, data, 0o644))
	}
	st, err := storage.For(root)
	require.NoError(t, err)
	mfs, err := st.FS()
	require.NoError(t, err)
	return root, mfs
}

// buildCloudTree registers the tree with the fake gateway: file bytes for the CDN
// and parent entries (files + intermediate dirs) for listings.
func buildCloudTree(f *fakeOpenList, root string, files map[string][]byte) {
	dirs := map[string]bool{root: true}
	for rel := range files {
		dirs[path.Dir(path.Join(root, rel))] = true
	}
	for d := range dirs {
		if d == root {
			continue
		}
		f.mu.Lock()
		f.dirs[path.Dir(d)] = append(f.dirs[path.Dir(d)], openlist.Entry{
			Name: path.Base(d), IsDir: true, Modified: time.Unix(900, 0),
		})
		f.mu.Unlock()
	}
	for rel, data := range files {
		full := path.Join(root, rel)
		f.mu.Lock()
		f.files[full] = data
		f.dirs[path.Dir(full)] = append(f.dirs[path.Dir(full)], openlist.Entry{
			Name: path.Base(full), Size: int64(len(data)), Modified: time.Unix(1000, 0),
		})
		f.mu.Unlock()
	}
}

// cloudRelayBackend serves the tree through the OpenList adapter with
// DisableRedirect, so both table backends exercise the identical ServeFile relay
// path (302 behaviour is covered separately by TestServeFileFullChain302).
func cloudRelayBackend(t *testing.T, files map[string][]byte) (string, storage.MusicFS) {
	t.Helper()
	f := newFakeOpenList(t)
	buildCloudTree(f, testRoot, files)
	mfs := setup(t, f, func(o *conf.OpenListOptions) { o.DisableRedirect = true })
	uri, err := BuildURI(f.url(), "lib")
	require.NoError(t, err)
	return uri, mfs
}

// runFSContract asserts the shared MusicFS contract and returns the file's tags
// for cross-backend comparison.
func runFSContract(t *testing.T, fsys storage.MusicFS, files map[string][]byte) map[string][]string {
	t.Helper()

	// 1. Open returns the exact bytes.
	for rel, want := range files {
		fh, err := fsys.Open(rel)
		require.NoError(t, err, "Open %s", rel)
		got, err := io.ReadAll(fh)
		require.NoError(t, err)
		assert.Equal(t, want, got, "content mismatch for %s", rel)
		require.NoError(t, fh.Close())
	}

	// 2. Stat: file size + directory shape.
	info, err := fs.Stat(fsys, conformanceTrack)
	require.NoError(t, err)
	assert.Equal(t, int64(len(files[conformanceTrack])), info.Size())
	assert.False(t, info.IsDir())
	di, err := fs.Stat(fsys, "artist/album")
	require.NoError(t, err)
	assert.True(t, di.IsDir())

	// 3. ReadDir listings (names as a set) at two levels.
	namesOf := func(dir string) []string {
		entries, err := fs.ReadDir(fsys, dir)
		require.NoError(t, err, "ReadDir %s", dir)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		return names
	}
	assert.Equal(t, []string{"album", "album2"}, namesOf("artist"))
	assert.Equal(t, []string{"cover.jpg", "track.mp3"}, namesOf("artist/album"))

	// 4. Path safety: traversal and absolute paths must not be openable.
	_, err = fsys.Open("../outside.txt")
	assert.Error(t, err, "path traversal must be rejected")
	_, err = fsys.Open("/etc/passwd")
	assert.Error(t, err, "absolute path must be rejected")

	// 5. Missing entries surface as fs.ErrNotExist (so callers can errors.Is).
	_, err = fsys.Open("artist/album/missing.mp3")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "missing file must wrap fs.ErrNotExist, got %v", err)

	// 6. ReadTags: one entry keyed by the relative path, with a non-empty title.
	tags, err := fsys.ReadTags(conformanceTrack)
	require.NoError(t, err)
	entry, ok := tags[conformanceTrack]
	require.True(t, ok, "ReadTags must key by the relative path")
	require.NotEmpty(t, entry.Tags["title"], "title must be parsed from the audio tags")
	return entry.Tags
}

// runServeRelay asserts the ServeFile relay path serves identical bytes (with
// working Range) — the behaviour local libraries have always had.
func runServeRelay(t *testing.T, libPath string, files map[string][]byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, storage.ServeFile(context.Background(), rec, req, libPath, conformanceTrack))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, files[conformanceTrack], rec.Body.Bytes())

	// Range / seek (断点续传) must keep working through the relay.
	req = httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Range", "bytes=0-9")
	rec = httptest.NewRecorder()
	require.NoError(t, storage.ServeFile(context.Background(), rec, req, libPath, conformanceTrack))
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, files[conformanceTrack][:10], rec.Body.Bytes())
}

func TestStorageConformanceLocalAndCloud(t *testing.T) {
	files := conformanceFiles(t)

	backends := []struct {
		name  string
		build func(t *testing.T) (string, storage.MusicFS)
	}{
		{"local", func(t *testing.T) (string, storage.MusicFS) { return localBackend(t, files) }},
		{"cloud", func(t *testing.T) (string, storage.MusicFS) { return cloudRelayBackend(t, files) }},
	}

	collected := map[string]map[string][]string{}
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			libPath, fsys := b.build(t)
			collected[b.name] = runFSContract(t, fsys, files)
			runServeRelay(t, libPath, files)
		})
	}

	// Cross-backend parity: the same audio file must yield the same tags locally
	// and through the cloud adapter.
	assert.Equal(t, collected["local"][conformanceTrack],
		collected["cloud"][conformanceTrack],
		"local and cloud adapters must produce identical tags for the same file")
}

// TestRemoteFileReadSpanningTailBoundary is a focused regression for the panic the
// conformance suite caught: a read whose range starts in the middle window but ends
// inside the tail window must not be answered from the tail window alone
// (src[off-base:] with base > off crashed with a negative slice bound).
func TestRemoteFileReadSpanningTailBoundary(t *testing.T) {
	f := newFakeOpenList(t)
	data := pattern(20000) // setup() config: head=1024, tail=1024, window=512
	f.addFile(testFilePath, data)
	fsys := setup(t, f, nil)

	fh, err := fsys.Open(testRelPath)
	require.NoError(t, err)
	defer fh.Close()
	ra, ok := fh.(io.ReaderAt)
	require.True(t, ok)

	// tailBase = 20000-1024 = 18976; this read spans 18000..20000 across it.
	buf := make([]byte, 2000)
	n, err := ra.ReadAt(buf, 18000)
	require.NoError(t, err)
	assert.Equal(t, data[18000:20000], buf[:n])
}

// TestServeFileFullChain302 is the end-to-end chain for cloud playback:
// ServeFile answers 302 with the signed link, and following it yields the exact
// bytes of the local copy（媒体数据不过 NAS 的红线链路）.
func TestServeFileFullChain302(t *testing.T) {
	files := conformanceFiles(t)
	f := newFakeOpenList(t)
	buildCloudTree(f, testRoot, files)
	setup(t, f, nil) // redirect enabled (default)
	uri, err := BuildURI(f.url(), "lib")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, storage.ServeFile(context.Background(), rec, req, uri, conformanceTrack))
	require.Equal(t, http.StatusFound, rec.Code, "cloud playback must 302 to the CDN")
	loc := rec.Header().Get("Location")
	require.NotEmpty(t, loc)
	assert.Empty(t, rec.Body.Bytes(), "302 must not leak a response body (红线 §15.1)")

	resp, err := http.Get(loc) //nolint:gosec // test-local fake CDN
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, files[conformanceTrack], got, "CDN bytes must equal the local copy")
}
