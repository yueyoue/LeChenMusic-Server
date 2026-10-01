package cloudsource

import (
	"fmt"
	"io"
	"testing"

	"github.com/navidrome/navidrome/conf"
)

// 评审 P2-11 (云库有声书扫描提速): the head window is fetched probe-sized first and
// extended to the *exact* ID3v2 tag span when the header says how big the tag is —
// parsing a 300KB tag must never download the configured multi-MB head window.
// Whatever the parser sees stays byte-identical (the conformance suite guards that).

// id3File builds a file whose ID3v2 tag spans tagSize bytes (after the 10-byte header),
// followed by audioSize bytes of audio pattern.
func id3File(tagSize, audioSize int) []byte {
	synchsafe := func(n int) []byte {
		return []byte{byte(n>>21)&0x7f, byte(n>>14)&0x7f, byte(n>>7)&0x7f, byte(n) & 0x7f}
	}
	header := append([]byte{'I', 'D', '3', 4, 0, 0}, synchsafe(tagSize)...)
	out := append(header, pattern(tagSize)...)
	return append(out, pattern(audioSize)...)
}

func TestHeadWindowGrowsToTagSpanOnly(t *testing.T) {
	const tagSize = 300 << 10
	data := id3File(tagSize, 2<<20)

	f := newFakeOpenList(t)
	f.addFile(testFilePath, data)
	// 2MB configured head window: the OLD behavior would fetch 2MB on the first read.
	fsys := setup(t, f, func(o *conf.OpenListOptions) {
		o.HeadBytes = 2 << 20
		o.TailBytes = 2 << 20
	})

	rf, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rf.Close() }()

	// First read: probe-sized fetch of the head.
	buf := make([]byte, 100)
	if _, err := io.ReadFull(rf, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}

	// A read deeper into the tag (past the probe): exactly one coalesced follow-up
	// fetch covering the tag span + frame slack.
	rat, ok := rf.(io.ReaderAt)
	if !ok {
		t.Fatalf("remote file must implement io.ReaderAt")
	}
	deep := make([]byte, 256)
	off := int64(100 << 10)
	if _, err := rat.ReadAt(deep, off); err != nil && err != io.EOF {
		t.Fatalf("ReadAt: %v", err)
	}
	for i := range deep {
		want := byte((int(off)+i-10) % 251) // tag body is pattern() after the 10-byte header
		if deep[i] != want {
			t.Fatalf("byte %d at off %d = %d, want %d", i, off, deep[i], want)
		}
	}

	ranges := f.requestedRanges()
	if len(ranges) != 2 {
		t.Fatalf("expected probe + 1 coalesced tag fetch, got %d: %v", len(ranges), ranges)
	}
	if ranges[0] != "bytes=0-65535" {
		t.Errorf("probe fetch = %s, want bytes=0-65535", ranges[0])
	}
	// The follow-up must cover the exact tag span (10+tagSize) plus frame slack, and
	// stop there — nowhere near the configured 2MB window.
	var from, to int
	if _, err := fmt.Sscanf(ranges[1], "bytes=%d-%d", &from, &to); err != nil {
		t.Fatalf("bad range %q: %v", ranges[1], err)
	}
	if from != 65536 {
		t.Errorf("follow-up fetch starts at %d, want 65536 (append after probe)", from)
	}
	tagEnd := 10 + tagSize
	if to+1 != tagEnd+int(id3FrameSlack) {
		t.Errorf("follow-up fetch ends at %d, want tag end %d + slack %d", to+1, tagEnd, id3FrameSlack)
	}
	if int64(to+1) > 2<<20 {
		t.Errorf("tag-sized fetch (%d bytes) must stay well below the configured 2MB window", to+1)
	}

	// And the tail window is untouched by a head-only tag read.
	for _, r := range ranges {
		if r == fmt.Sprintf("bytes=%d-%d", len(data)-int(tailProbeBytes), len(data)-1) {
			t.Fatalf("tag read must not fetch the tail window: %v", ranges)
		}
	}
}

func TestHeadProbeWithoutID3DoesNotGrowBeyondNeed(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(20000))
	fsys := setup(t, f, func(o *conf.OpenListOptions) {
		o.HeadBytes = 2 << 20
	})

	rf, err := fsys.Open(testRelPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rf.Close() }()

	buf := make([]byte, 100)
	if _, err := io.ReadFull(rf, buf); err != nil {
		t.Fatalf("Read: %v", err)
	}

	// No ID3 header → no speculative extension: the probe fetch is capped by the
	// smaller of the configured window and the probe size.
	ranges := f.requestedRanges()
	if len(ranges) != 1 {
		t.Fatalf("expected exactly 1 probe fetch, got %v", ranges)
	}
	if ranges[0] != fmt.Sprintf("bytes=0-%d", min64(headProbeBytes, 20000)-1) {
		t.Errorf("probe fetch = %s, want probe-sized", ranges[0])
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
