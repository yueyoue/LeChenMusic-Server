package cloudsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
)

// errReadBudget is returned when a single file read would exceed the configured budget.
// It is what keeps a tag parser (or a buggy caller) from turning "read the header" into
// a full-file download, which is exactly the 下载器特征 the design doc forbids.
var errReadBudget = errors.New("cloudsource: read budget exceeded")

// linkRefreshMargin is how long before a signed link's expiry the next read starts
// re-resolving it. Generous enough to cover one round-trip to the gateway plus the
// throttling delay between reads.
const linkRefreshMargin = 2 * time.Minute

// remoteFile is a lazy, seekable reader over a file's direct link (raw_url).
//
// It never downloads the file: every read is served from one of three bounded windows
// (head / tail / sliding middle), each fetched with a single HTTP Range request. That is
// enough for tag parsing (tags live at the head; MP4/M4A moov boxes and cover art may
// live at the tail) and for the server-side streaming fallback, without ever pulling the
// whole object in one go.
type remoteFile struct {
	ctx    context.Context
	ep     *Endpoint
	root   string // remote root of the library (OpenList absolute path)
	name   string // fs-relative path, for error messages
	info   *cloudInfo
	budget int64 // max bytes this handle may fetch; 0 = unlimited (streaming)

	mu     sync.Mutex
	pos    int64
	rawURL string
	// rawExpires is the expiry of rawURL when the gateway reports one (zero = unknown).
	// Signed links are short-lived: a handle that stays open longer than the link (the
	// server-side relay of a multi-hour chapter, for one) must re-resolve mid-read
	// instead of failing every later window with a stale signature (评审 §4.4).
	rawExpires time.Time
	resolved   bool

	head    []byte // covers [0, len(head))
	tail    []byte // covers [tailOff, tailOff+len(tail))
	tailOff int64
	mid     []byte // covers [midOff, midOff+len(mid))
	midOff  int64

	fetched int64
	closed  bool
}

var (
	_ io.ReadSeeker = (*remoteFile)(nil)
	_ io.ReaderAt   = (*remoteFile)(nil)
	_ io.Closer     = (*remoteFile)(nil)
	_ fs.File       = (*remoteFile)(nil)
)

func newRemoteFile(ctx context.Context, ep *Endpoint, root, name string, info *cloudInfo) *remoteFile {
	return &remoteFile{ctx: ctx, ep: ep, root: root, name: name, info: info}
}

func (f *remoteFile) Stat() (fs.FileInfo, error) { return f.info, nil }

func (f *remoteFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *remoteFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, errors.New("cloudsource: read on closed file")
	}
	if len(p) == 0 {
		return 0, nil
	}
	n, err := f.readAtLocked(f.pos, p)
	f.pos += int64(n)
	return n, err
}

func (f *remoteFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, errors.New("cloudsource: seek on closed file")
	}
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.pos + offset
	case io.SeekEnd:
		abs = f.info.size + offset
	default:
		return 0, fmt.Errorf("cloudsource: invalid seek whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("cloudsource: negative seek position")
	}
	f.pos = abs
	return abs, nil
}

// ReadAt implements io.ReaderAt and does not affect the seek position.
func (f *remoteFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, errors.New("cloudsource: read on closed file")
	}
	return f.readAtLocked(off, p)
}

func (f *remoteFile) readAtLocked(off int64, p []byte) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("cloudsource: negative read offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if f.info.size == 0 || off >= f.info.size {
		return 0, io.EOF
	}

	want := len(p)
	if int64(want) > f.info.size-off {
		want = int(f.info.size - off)
	}

	written := 0
	for written < want {
		n, err := f.copyWindowLocked(off+int64(written), p[written:want])
		if err != nil {
			return written, err
		}
		if n == 0 {
			break
		}
		written += n
	}
	if written < len(p) {
		return written, io.EOF
	}
	return written, nil
}

// copyWindowLocked fills as much of dst as the next cached/fetchable window can cover.
func (f *remoteFile) copyWindowLocked(off int64, dst []byte) (int, error) {
	src, base, ok := f.cachedLocked(off, int64(len(dst)))
	if !ok {
		var err error
		src, base, err = f.loadWindowLocked(off, int64(len(dst)))
		if err != nil {
			return 0, err
		}
	}
	return copy(dst, src[off-base:]), nil
}

// cachedLocked returns a cached window covering [off, off+n).
func (f *remoteFile) cachedLocked(off, n int64) ([]byte, int64, bool) {
	windows := [...]struct {
		base int64
		data []byte
	}{
		{0, f.head},
		{f.midOff, f.mid},
		{f.tailOff, f.tail},
	}
	for _, w := range windows {
		if len(w.data) > 0 && off >= w.base && off+n <= w.base+int64(len(w.data)) {
			return w.data, w.base, true
		}
	}
	return nil, 0, false
}

// loadWindowLocked fetches a window able to cover [off, off+n), choosing the cheapest
// shape: the file head, the file tail, or a sliding middle window.
//
// Windows are fetched *on demand* in probe-sized pieces instead of eagerly pulling the
// full configured head/tail span (评审 P2-11: 云库有声书扫描提速). The configured
// HeadBytes/TailBytes remain the caps for eager extension, so existing configs keep
// their meaning — but a tag read on a file with a small tag now transfers kilobytes
// instead of megabytes, without changing a single byte of what the parser sees.
func (f *remoteFile) loadWindowLocked(off, n int64) ([]byte, int64, error) {
	size := f.info.size
	headCap := f.ep.headBytes
	tailCap := f.ep.tailBytes
	if headCap > size {
		headCap = size
	}
	if tailCap > size {
		tailCap = size
	}

	// Tail window base: [tailBase, size).
	tailBase := size - tailCap
	if tailBase < 0 {
		tailBase = 0
	}

	switch {
	case off < headCap:
		if err := f.ensureHeadLocked(off + n); err != nil {
			return nil, 0, err
		}
		return f.head, 0, nil

	case off >= tailBase:
		// Inside the tail window. Note the condition must anchor on `off` alone:
		// a request spanning the middle/tail boundary (off < tailBase < off+n)
		// must NOT return the tail window — its base would exceed off and the
		// caller's src[off-base:] would panic (found by the P2-6 conformance
		// suite streaming a whole file through tiny windows).
		if err := f.ensureTailLocked(size - off); err != nil {
			return nil, 0, err
		}
		return f.tail, f.tailOff, nil

	default:
		width := n
		if f.ep.windowBytes > width {
			width = f.ep.windowBytes
		}
		if off+width > size {
			width = size - off
		}
		if width <= 0 {
			return nil, 0, io.EOF
		}
		data, err := f.fetchLocked(off, width)
		if err != nil {
			return nil, 0, err
		}
		f.mid, f.midOff = data, off
		return data, off, nil
	}
}

// probe-sized first fetches for the two end windows (评审 P2-11). The configured
// head/tail bytes remain the maximum eager span; extension beyond the probe is driven
// by what the parser actually reads (plus the ID3v2 size hint for the head).
const (
	headProbeBytes = int64(64 << 10)
	tailProbeBytes = int64(64 << 10)
	id3FrameSlack  = int64(256 << 10) // first audio frames + Xing header, right after the tag
)

// id3TagEnd returns the byte offset just past the ID3v2 tag (header + tag body + footer),
// or 0 when the head does not start with an ID3v2 header. The 10-byte header carries the
// exact tag size, so one small probe read is enough to size the follow-up fetch.
func id3TagEnd(head []byte, size int64) int64 {
	if len(head) < 10 || head[0] != 'I' || head[1] != 'D' || head[2] != '3' {
		return 0
	}
	// Synchsafe 28-bit size in bytes 6..9.
	tagSize := int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
	end := 10 + tagSize
	if head[5]&0x10 != 0 { // footer present
		end += 10
	}
	if end > size {
		return 0
	}
	return end
}

// ensureHeadLocked grows the head window until it covers [0, need), fetching only the
// missing suffix. The first fetch is probe-sized; when the file starts with an ID3v2 tag
// the exact tag span is coalesced into one follow-up fetch, so parsing a 300KB tag never
// downloads the old fixed 2MB head window.
func (f *remoteFile) ensureHeadLocked(need int64) error {
	size := f.info.size
	probe := headProbeBytes
	if f.ep.headBytes < probe {
		probe = f.ep.headBytes
	}
	if probe > size {
		probe = size
	}
	want := need
	if want < probe {
		want = probe
	}
	if end := id3TagEnd(f.head, size); end > 0 && end+id3FrameSlack > want {
		want = end + id3FrameSlack
	}
	if want > size {
		want = size
	}
	if want <= int64(len(f.head)) {
		return nil
	}
	data, err := f.fetchLocked(int64(len(f.head)), want-int64(len(f.head)))
	if err != nil {
		return err
	}
	f.head = append(f.head, data...)
	return nil
}

// ensureTailLocked grows the tail window backwards until it covers [size-need, size),
// fetching only the missing prefix.
func (f *remoteFile) ensureTailLocked(need int64) error {
	size := f.info.size
	probe := tailProbeBytes
	if f.ep.tailBytes < probe {
		probe = f.ep.tailBytes
	}
	if probe > size {
		probe = size
	}
	want := need
	if want < probe {
		want = probe
	}
	if want > size {
		want = size
	}
	if want <= int64(len(f.tail)) {
		return nil
	}
	tailEnd := f.tailOff
	if len(f.tail) == 0 {
		tailEnd = size
	}
	newBase := size - want
	data, err := f.fetchLocked(newBase, tailEnd-newBase)
	if err != nil {
		return err
	}
	f.tail = append(data, f.tail...)
	f.tailOff = newBase
	return nil
}

// fetchLocked pulls [off, off+n) bytes through the throttled RangeFetcher.
func (f *remoteFile) fetchLocked(off, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, io.EOF
	}
	if off+n > f.info.size {
		n = f.info.size - off
	}
	if f.budget > 0 && f.fetched+n > f.budget {
		return nil, fmt.Errorf("%w: %s wants more than %d bytes", errReadBudget, f.name, f.budget)
	}
	if err := f.ensureRawURLLocked(); err != nil {
		return nil, err
	}
	data, err := f.ep.fetcher.FetchRange(f.ctx, f.rawURL, off, n)
	if err != nil {
		// The link may have expired between resolve and use (or mid-handle on long
		// streams). Re-resolve once and retry before giving up — the same signed URL is
		// never reused after a failure, so a stale signature cannot wedge the handle.
		if refreshErr := f.refreshRawURLForcedLocked(); refreshErr != nil {
			log.Debug(f.ctx, "[cloud][cloudsource] direct link refresh failed", "file", f.name, refreshErr)
			return nil, err
		}
		data, err = f.ep.fetcher.FetchRange(f.ctx, f.rawURL, off, n)
		if err != nil {
			return nil, err
		}
	}
	f.fetched += int64(len(data))
	return data, nil
}

// linkExpiringLocked reports whether the current link should be re-resolved before use.
// With no expiry info from the gateway the link is kept (and refreshed only when a fetch
// actually fails), so gateways that never report expiry don't pay an extra /api/fs/get
// per window.
func (f *remoteFile) linkExpiringLocked() bool {
	if !f.resolved {
		return true
	}
	if f.rawExpires.IsZero() {
		return false
	}
	return !time.Now().Add(linkRefreshMargin).Before(f.rawExpires)
}

// ensureRawURLLocked resolves the signed direct link, re-resolving it once it expired
// (or is about to). Links are short-lived (hours at most) and never cached across handles.
func (f *remoteFile) ensureRawURLLocked() error {
	if !f.linkExpiringLocked() {
		return nil
	}
	return f.refreshRawURLLocked()
}

// refreshRawURLLocked resolves a new signed direct link, replacing whatever the handle
// held before.
func (f *remoteFile) refreshRawURLLocked() error {
	return f.refreshRawURLFromLocked(f.ep.directURL)
}

// refreshRawURLForcedLocked re-resolves bypassing the shared link cache: it runs after a
// fetch failed, so the very same (dead) signature must never be served again.
func (f *remoteFile) refreshRawURLForcedLocked() error {
	return f.refreshRawURLFromLocked(f.ep.directURLFresh)
}

func (f *remoteFile) refreshRawURLFromLocked(resolve func(context.Context, string) (string, time.Time, error)) error {
	raw, expires, err := resolve(f.ctx, remotePathOf(f.root, f.name))
	if err != nil {
		return err
	}
	f.rawURL, f.rawExpires, f.resolved = raw, expires, true
	return nil
}

// String makes log lines readable. The direct link is deliberately left out of it: it is
// a signed URL and must never end up in logs.
func (f *remoteFile) String() string {
	return fmt.Sprintf("remoteFile{%s size=%d}", f.name, f.info.size)
}
