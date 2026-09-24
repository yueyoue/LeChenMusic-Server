package cloudsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
)

// ---------------------------------------------------------------------------
// A scriptable OpenList + CDN fake: it records every byte range that gets read,
// so tests can assert that a tag/cover read never downloads a whole file.
// ---------------------------------------------------------------------------

type fakeOpenList struct {
	t *testing.T

	srv *httptest.Server

	mu     sync.Mutex
	lists  int      // /api/fs/list call count
	gets   int      // /api/fs/get call count
	ranges []string // byte ranges requested from the CDN
	files  map[string][]byte
	dirs   map[string][]openlist.Entry

	rawPath string // CDN path prefix
}

func newFakeOpenList(t *testing.T) *fakeOpenList {
	f := &fakeOpenList{
		t:       t,
		files:   map[string][]byte{},
		dirs:    map[string][]openlist.Entry{},
		rawPath: "/cdn",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", f.handleLogin)
	mux.HandleFunc("/api/fs/list", f.handleList)
	mux.HandleFunc("/api/fs/get", f.handleGet)
	mux.HandleFunc(f.rawPath+"/", f.handleCDN)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenList) url() string { return f.srv.URL }

func (f *fakeOpenList) rangeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ranges)
}

func (f *fakeOpenList) requestedRanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ranges...)
}

// addFile registers a file's bytes *and* its parent directory entry, so both Stat
// (served from listings) and the CDN (serving Range reads) can find it.
func (f *fakeOpenList) addFile(path string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = data
	slash := strings.LastIndex(path, "/")
	dir, name := path[:slash], path[slash+1:]
	f.dirs[dir] = []openlist.Entry{{Name: name, Size: int64(len(data)), Modified: time.Unix(1000, 0)}}
}

func (f *fakeOpenList) handleLogin(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, map[string]any{"token": "test-token"})
}

func (f *fakeOpenList) handleList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Page int    `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.lists++
	entries := f.dirs[req.Path]
	f.mu.Unlock()

	// Real gateways page; the client stops once the reported total is reached.
	// Everything lives on page 1 here, so page >= 2 must come back empty.
	if req.Page > 1 {
		entries = nil
	}
	writeEnvelope(w, map[string]any{"content": entries, "total": len(entries)})
}

func (f *fakeOpenList) handleGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.gets++
	data, ok := f.files[req.Path]
	f.mu.Unlock()
	if !ok {
		writeEnvelope(w, map[string]any{})
		return
	}
	name := req.Path
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	writeEnvelope(w, map[string]any{
		"name":    name,
		"size":    len(data),
		"is_dir":  false,
		"raw_url": f.srv.URL + f.rawPath + req.Path,
		// No expiry field on purpose: FileInfo must cope with its absence.
	})
}

func (f *fakeOpenList) handleCDN(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, f.rawPath)
	f.mu.Lock()
	data, ok := f.files[name]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	rng := r.Header.Get("Range")
	if rng == "" {
		_, _ = w.Write(data)
		return
	}

	var start, end int
	if _, err := fmt.Sscanf(strings.TrimPrefix(rng, "bytes="), "%d-%d", &start, &end); err != nil {
		http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if end >= len(data) {
		end = len(data) - 1
	}

	f.mu.Lock()
	f.ranges = append(f.ranges, rng)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(data[start : end+1])
}

func writeEnvelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "", "data": data})
}

// ---------------------------------------------------------------------------
// test setup helpers
// ---------------------------------------------------------------------------

// pattern returns deterministic content: byte i == i % 251.
func pattern(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

const (
	testRoot     = "/lib"
	testFilePath = testRoot + "/artist/album/track.flac"
	testRelPath  = "artist/album/track.flac"
)

// setup wires conf to point at the fake gateway and returns a cloudFS rooted at
// testRoot. Sizes are kept small so window/budget arithmetic stays easy to follow.
func setup(t *testing.T, f *fakeOpenList, tweak func(*conf.OpenListOptions)) *cloudFS {
	t.Helper()

	saved := conf.Server.OpenList
	t.Cleanup(func() {
		conf.Server.OpenList = saved
		resetEndpoints()
	})

	opts := conf.OpenListOptions{
		URL:              f.url(),
		Username:         "admin",
		Password:         "secret",
		ListInterval:     time.Nanosecond,
		ReadInterval:     time.Nanosecond,
		RetryBaseDelay:   time.Nanosecond,
		CircuitOpenFor:   time.Minute,
		HeadBytes:        1024,
		TailBytes:        1024,
		WindowBytes:      512,
		MaxTagReadBytes:  4096,
		DirCacheTTL:      time.Hour,
		FailureThreshold: 5,
	}
	if tweak != nil {
		tweak(&opts)
	}
	conf.Server.OpenList = map[string]conf.OpenListOptions{"test": opts}
	resetEndpoints()

	uri, err := BuildURI(f.url(), strings.TrimPrefix(testRoot, "/"))
	if err != nil {
		t.Fatalf("BuildURI: %v", err)
	}
	st, err := storage.For(uri)
	if err != nil {
		t.Fatalf("storage.For(%q): %v", uri, err)
	}
	cs, ok := st.(storage.ContextualStorage)
	if !ok {
		t.Fatal("cloud storage must implement storage.ContextualStorage")
	}
	if _, ok := st.(storage.DirectLinkProvider); !ok {
		t.Fatal("cloud storage must implement storage.DirectLinkProvider")
	}
	mfs, err := cs.FSWithContext(context.Background())
	if err != nil {
		t.Fatalf("FSWithContext: %v", err)
	}
	return mfs.(*cloudFS)
}

// ---------------------------------------------------------------------------
// listing / stat / open
// ---------------------------------------------------------------------------

func TestReadDirListsOnceAndSorts(t *testing.T) {
	f := newFakeOpenList(t)
	f.dirs[testRoot] = []openlist.Entry{
		{Name: "zzz.flac", Size: 10, Modified: time.Unix(1000, 0)},
		{Name: "sub", IsDir: true, Modified: time.Unix(900, 0)},
		{Name: "aaa.flac", Size: 20, Modified: time.Unix(1100, 0)},
	}
	fsys := setup(t, f, nil)

	entries, err := fsys.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"aaa.flac", "sub", "zzz.flac"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("entries = %v, want %v (sorted by name)", names, want)
	}

	// Second call must be served from the directory cache (设计文档 §4: 目录列表强缓存),
	// and pagination must not cost a speculative extra request past the end.
	if _, err := fsys.ReadDir("."); err != nil {
		t.Fatalf("ReadDir (2nd): %v", err)
	}
	if f.lists != 1 {
		t.Fatalf("expected exactly 1 /api/fs/list call (total-driven pagination + cache), got %d", f.lists)
	}
}

func TestStatFileAndRoot(t *testing.T) {
	f := newFakeOpenList(t)
	f.dirs[testRoot] = []openlist.Entry{
		{Name: "artist", IsDir: true, Modified: time.Unix(900, 0)},
	}
	f.dirs[testRoot+"/artist"] = []openlist.Entry{
		{Name: "track.flac", Size: 42, Modified: time.Unix(1234, 0)},
	}
	fsys := setup(t, f, nil)

	info, err := fsys.Stat("artist/track.flac")
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if info.Size() != 42 || info.IsDir() {
		t.Errorf("Stat file = size %d dir %v, want size 42 dir false", info.Size(), info.IsDir())
	}

	root, err := fsys.Stat(".")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !root.IsDir() {
		t.Error("root must be a directory")
	}

	if _, err := fsys.Stat("artist/missing.flac"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestOpenDirectoryIsReadable(t *testing.T) {
	f := newFakeOpenList(t)
	f.dirs[testRoot] = []openlist.Entry{{Name: "artist", IsDir: true}}
	f.dirs[testRoot+"/artist"] = []openlist.Entry{{Name: "track.flac", Size: 42}}
	fsys := setup(t, f, nil)

	file, err := fsys.Open("artist")
	if err != nil {
		t.Fatalf("Open dir: %v", err)
	}
	defer func() { _ = file.Close() }()

	// The scanner asserts on fs.ReadDirFile, so a directory handle must implement it.
	dir, ok := file.(fs.ReadDirFile)
	if !ok {
		t.Fatalf("a directory handle must implement fs.ReadDirFile, got %T", file)
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		t.Fatalf("ReadDir from handle: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "track.flac" {
		t.Fatalf("entries = %v, want [track.flac]", entries)
	}
}

// ---------------------------------------------------------------------------
// remoteFile: windowed, bounded, lazy reads
// ---------------------------------------------------------------------------

func TestRemoteFileReadsOnlyTheHeadWindow(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, nil)

	rf, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rf.Close() }()

	buf := make([]byte, 100)
	if _, err := io.ReadFull(rf, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}
	for i := range buf {
		if buf[i] != byte(i%251) {
			t.Fatalf("byte %d = %d, want %d", i, buf[i], byte(i%251))
		}
	}

	ranges := f.requestedRanges()
	if len(ranges) != 1 {
		t.Fatalf("expected exactly 1 range request for a 100-byte read, got %v", ranges)
	}
	if ranges[0] != "bytes=0-1023" {
		t.Errorf("head window = %s, want bytes=0-1023", ranges[0])
	}
	// One direct-link resolution per handle, not one per read.
	if f.gets != 1 {
		t.Errorf("expected 1 /api/fs/get call, got %d", f.gets)
	}
}

func TestRemoteFileTailWindowForTrailingMetadata(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, nil)

	file, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()

	rs, ok := file.(io.ReadSeeker)
	if !ok {
		t.Fatalf("a file handle must be seekable (http.ServeContent relies on it), got %T", file)
	}

	// M4A puts the moov box (and often the cover) at the tail: seek to the end.
	if _, err := rs.Seek(-100, io.SeekEnd); err != nil {
		t.Fatalf("Seek end: %v", err)
	}
	buf := make([]byte, 100)
	if _, err := io.ReadFull(file, buf); err != nil {
		t.Fatalf("Read tail: %v", err)
	}
	for i := range buf {
		if want := byte((20000 - 100 + i) % 251); buf[i] != want {
			t.Fatalf("tail byte %d = %d, want %d", i, buf[i], want)
		}
	}

	ranges := f.requestedRanges()
	if len(ranges) != 1 {
		t.Fatalf("expected exactly 1 range request for a tail read, got %v", ranges)
	}
	if ranges[0] != "bytes=18976-19999" {
		t.Errorf("tail window = %s, want bytes=18976-19999 (size-1024 .. size-1)", ranges[0])
	}
}

func TestRemoteFileSlidingWindowInTheMiddle(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, nil)

	file, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()

	ra, ok := file.(io.ReaderAt)
	if !ok {
		t.Fatalf("a file handle must implement io.ReaderAt, got %T", file)
	}

	buf := make([]byte, 100)
	if _, err := ra.ReadAt(buf, 5000); err != nil {
		t.Fatalf("ReadAt middle: %v", err)
	}
	for i := range buf {
		if want := byte((5000 + i) % 251); buf[i] != want {
			t.Fatalf("mid byte %d = %d, want %d", i, buf[i], want)
		}
	}

	ranges := f.requestedRanges()
	if len(ranges) != 1 {
		t.Fatalf("expected exactly 1 range request, got %v", ranges)
	}
	// width = max(100, WindowBytes 512) = 512
	if ranges[0] != "bytes=5000-5511" {
		t.Errorf("middle window = %s, want bytes=5000-5511", ranges[0])
	}
}

func TestRemoteFileHonoursTheReadBudget(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	fsys := setup(t, f, nil)

	rf := newRemoteFile(context.Background(), fsys.ep, strings.TrimPrefix(testRoot, "/"),
		testRelPath, &cloudInfo{name: "track.flac", size: 200000})
	// Smaller than the tail window (1024): the read must be refused before any I/O.
	rf.budget = 500

	if _, err := rf.Seek(199000, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 100)
	_, err := rf.Read(buf)
	if !strings.Contains(fmt.Sprint(err), "read budget exceeded") {
		t.Fatalf("expected a read-budget error, got %v", err)
	}
	if f.rangeCount() != 0 {
		t.Errorf("a refused read must not hit the CDN, got %v", f.requestedRanges())
	}
}

func TestRemoteFileStreamingHasNoBudget(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	fsys := setup(t, f, nil)

	rf, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rf.Close() }()

	// The 中转流 fallback must be able to serve the whole file (budget == 0).
	fr := rf.(*remoteFile)
	fr.mu.Lock()
	budget := fr.budget
	fr.mu.Unlock()
	if budget != 0 {
		t.Fatalf("a plain Open() must not carry a read budget, got %d", budget)
	}
}

func TestBudgetFSWrapsOpen(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	fsys := setup(t, f, nil)

	bfs := &budgetFS{fsys: fsys, budget: 123}
	file, err := bfs.Open(testRelPath)
	if err != nil {
		t.Fatalf("budgetFS.Open: %v", err)
	}
	defer func() { _ = file.Close() }()

	fr := file.(*remoteFile)
	fr.mu.Lock()
	budget := fr.budget
	fr.mu.Unlock()
	if budget != 123 {
		t.Fatalf("budgetFS must impose its budget, got %d", budget)
	}
}

// ---------------------------------------------------------------------------
// direct links (M3's 302 mode)
// ---------------------------------------------------------------------------

func TestDirectURL(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(1024))
	fsys := setup(t, f, nil)

	raw, expires, err := fsys.DirectURL(context.Background(), testRelPath)
	if err != nil {
		t.Fatalf("DirectURL: %v", err)
	}
	if !strings.HasPrefix(raw, f.url()+f.rawPath+"/") {
		t.Errorf("unexpected direct link: %q", raw)
	}
	if !expires.IsZero() {
		t.Errorf("no expiry info was returned, want zero time, got %s", expires)
	}
}

func TestDirectURLRespectsDisableRedirect(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(1024))
	fsys := setup(t, f, func(o *conf.OpenListOptions) { o.DisableRedirect = true })

	// Playback must fall back to the 中转流, but lazy tag reads may still use ranges.
	if _, _, err := fsys.DirectURL(context.Background(), testRelPath); err == nil {
		t.Fatal("expected an error when DisableRedirect is set")
	}
}

func TestDirectURLRejectsDirectories(t *testing.T) {
	f := newFakeOpenList(t)
	f.dirs[testRoot] = []openlist.Entry{{Name: "artist", IsDir: true}}
	fsys := setup(t, f, nil)

	if _, _, err := fsys.DirectURL(context.Background(), "artist"); err == nil {
		t.Fatal("expected an error when asking for a directory's direct link")
	}
}

func TestRemoteFileStillReadsWhenRedirectDisabled(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, func(o *conf.OpenListOptions) { o.DisableRedirect = true })

	rf, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rf.Close() }()

	buf := make([]byte, 10)
	if _, err := io.ReadFull(rf, buf); err != nil {
		t.Fatalf("tag reads must keep working when 302 is disabled: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReadTags
// ---------------------------------------------------------------------------

func TestReadTagsFilenameModeNeverTouchesContent(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, func(o *conf.OpenListOptions) { o.TagMode = "filename" })

	tags, err := fsys.ReadTags(testRelPath)
	if err != nil {
		t.Fatalf("ReadTags: %v", err)
	}
	info := tags[testRelPath]
	if got := info.Tags["title"]; len(got) != 1 || got[0] != "track" {
		t.Errorf("title = %v, want [track]", got)
	}
	if got := info.Tags["artist"]; len(got) != 1 || got[0] != "artist" {
		t.Errorf("artist = %v, want [artist]", got)
	}
	if got := info.Tags["album"]; len(got) != 1 || got[0] != "album" {
		t.Errorf("album = %v, want [album]", got)
	}
	if info.FileInfo == nil {
		t.Error("FileInfo must be populated for the scanner")
	}
	if f.rangeCount() != 0 {
		t.Errorf("TagMode=filename must not read any file content, got ranges %v", f.requestedRanges())
	}
	if f.gets != 0 {
		t.Errorf("TagMode=filename must not even resolve a direct link, got %d /api/fs/get calls", f.gets)
	}
}

func TestReadTagsEmptyInput(t *testing.T) {
	f := newFakeOpenList(t)
	fsys := setup(t, f, nil)
	tags, err := fsys.ReadTags()
	if err != nil || len(tags) != 0 {
		t.Fatalf("ReadTags() = %v, %v; want empty result and no error", tags, err)
	}
}

// ---------------------------------------------------------------------------
// endpoint resolution
// ---------------------------------------------------------------------------

func TestEndpointForMatchesConfiguredURL(t *testing.T) {
	f := newFakeOpenList(t)
	setup(t, f, nil)

	ep, err := EndpointFor(strings.TrimPrefix(f.url(), "http://"))
	if err != nil {
		t.Fatalf("EndpointFor: %v", err)
	}
	if ep.Name != "test" {
		t.Errorf("resolved endpoint = %q, want test", ep.Name)
	}
}

func TestEndpointForRejectsUnknownEndpoint(t *testing.T) {
	saved := conf.Server.OpenList
	t.Cleanup(func() {
		conf.Server.OpenList = saved
		resetEndpoints()
	})
	conf.Server.OpenList = map[string]conf.OpenListOptions{
		"a": {URL: "http://a:1"},
		"b": {URL: "http://b:2"},
	}
	resetEndpoints()

	if _, err := EndpointFor("c:3"); err == nil {
		t.Fatal("expected an error for an unconfigured endpoint among several")
	}
}

func TestEndpointForNoConfigAtAll(t *testing.T) {
	saved := conf.Server.OpenList
	t.Cleanup(func() {
		conf.Server.OpenList = saved
		resetEndpoints()
	})
	conf.Server.OpenList = nil
	resetEndpoints()

	if _, err := EndpointFor("anything"); err == nil {
		t.Fatal("expected an error when no OpenList gateway is configured")
	}
}
