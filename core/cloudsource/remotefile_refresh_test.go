package cloudsource

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// Signed-link lifetime regression tests (评审 §4.4 / P1-4 的中转流侧).
//
// A remoteFile handle can outlive the signed direct link it was opened with: the
// server-side relay (中转流) of a multi-hour audiobook chapter keeps one handle open
// for the whole chapter, and tag/cover reads on a long-lived handle hit the same
// wall. The handle must re-resolve the link instead of failing every later window.

// newTestRemoteFile builds a handle over the fake gateway's file, bypassing the
// listing cache so /api/fs/get counters stay meaningful.
func newTestRemoteFile(t *testing.T, fsys *cloudFS, size int64) *remoteFile {
	t.Helper()
	return newRemoteFile(context.Background(), fsys.ep, testRoot[1:],
		testRelPath, &cloudInfo{name: "track.flac", size: size})
}

func readWindow(t *testing.T, rf *remoteFile, off int64) []byte {
	t.Helper()
	buf := make([]byte, 100)
	if _, err := rf.ReadAt(buf, off); err != nil {
		t.Fatalf("ReadAt(%d): %v", off, err)
	}
	return buf
}

// With no expiry info from the gateway the link is kept for the handle's lifetime
// (unchanged behaviour: no extra /api/fs/get per window).
func TestRemoteFileUnknownExpiryKeepsSingleLink(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	fsys := setup(t, f, nil)

	rf := newTestRemoteFile(t, fsys, 200000)
	defer func() { _ = rf.Close() }()

	for _, off := range []int64{5000, 6000, 7000} {
		buf := readWindow(t, rf, off)
		for i := range buf {
			if want := byte((off + int64(i)) % 251); buf[i] != want {
				t.Fatalf("byte %d @%d = %d, want %d", i, off, buf[i], want)
			}
		}
	}
	if f.gets != 1 {
		t.Fatalf("expected exactly 1 link resolution with no expiry info, got %d", f.gets)
	}
}

// A link that expires within the refresh margin must be replaced *before* it is used,
// so a long read never fires a request the gateway is about to reject.
func TestRemoteFileRefreshesLinkBeforeExpiry(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	f.expiry = time.Now().Add(1 * time.Minute) // inside the 2-minute margin
	fsys := setup(t, f, nil)

	rf := newTestRemoteFile(t, fsys, 200000)
	defer func() { _ = rf.Close() }()

	readWindow(t, rf, 5000)
	if f.gets != 1 {
		t.Fatalf("first read must resolve the link once, got %d resolutions", f.gets)
	}
	readWindow(t, rf, 6000)
	if f.gets != 2 {
		t.Fatalf("expiring link must be re-resolved before the next window, got %d resolutions", f.gets)
	}
}

// A link that dies mid-handle (here: another playback resolves a newer link and the
// gateway invalidates the old one) must not wedge the handle: the failed fetch
// re-resolves and retries exactly the same window.
func TestRemoteFileRefreshesLinkAfterMidReadFailure(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	fsys := setup(t, f, nil)

	rfA := newTestRemoteFile(t, fsys, 200000)
	defer func() { _ = rfA.Close() }()
	readWindow(t, rfA, 5000) // link v=1

	// A second handle (e.g. another playback) gets the newest link and invalidates v=1.
	rfB := newTestRemoteFile(t, fsys, 200000)
	readWindow(t, rfB, 5000)
	_ = rfB.Close()

	buf := readWindow(t, rfA, 6000) // stale link → 403 → refresh → retry
	for i := range buf {
		if want := byte((6000 + int64(i)) % 251); buf[i] != want {
			t.Fatalf("byte %d = %d, want %d", i, buf[i], want)
		}
	}
	// 1 (A) + 1 (B) + 1 (A's refresh) — the retry reuses the refreshed link, it must
	// not resolve twice.
	if f.gets != 3 {
		t.Fatalf("expected 3 link resolutions (A, B, A-refresh), got %d", f.gets)
	}
}

// When the refresh itself fails, the original fetch error is what the caller sees:
// a gateway outage must not be masked by its own follow-up failure.
func TestRemoteFileKeepsOriginalErrorWhenRefreshFails(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	fsys := setup(t, f, nil)

	rfA := newTestRemoteFile(t, fsys, 200000)
	defer func() { _ = rfA.Close() }()
	readWindow(t, rfA, 5000)

	rfB := newTestRemoteFile(t, fsys, 200000)
	readWindow(t, rfB, 5000)
	_ = rfB.Close()

	// Break the gateway: the refresh (/api/fs/get) now fails too.
	f.mu.Lock()
	f.files = map[string][]byte{}
	f.mu.Unlock()

	buf := make([]byte, 100)
	if _, err := rfA.ReadAt(buf, 6000); err == nil {
		t.Fatal("expected an error when both the link and the refresh fail")
	}
	if !bytes.Equal(buf, make([]byte, 100)) {
		t.Fatal("a failed read must not write output")
	}
}
