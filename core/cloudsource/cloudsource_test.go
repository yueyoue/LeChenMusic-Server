package cloudsource

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
)

// ---------------------------------------------------------------------------
// BuildURI: the single source of truth for how a cloud library path is composed
// ---------------------------------------------------------------------------

func TestBuildURI(t *testing.T) {
	cases := []struct {
		name       string
		address    string
		remotePath string
		want       string
		wantErr    bool
	}{
		{
			name:       "simple address and path",
			address:    "http://192.168.1.10:5244",
			remotePath: "fnos/music",
			want:       "openlist://192.168.1.10:5244/fnos/music",
		},
		{
			name:       "chinese path is escaped per component",
			address:    "http://192.168.1.10:5244",
			remotePath: "fnos/音乐",
			want:       "openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90",
		},
		{
			name:       "surrounding slashes are trimmed",
			address:    "https://openlist.example.com/",
			remotePath: "/fnos/music/",
			want:       "openlist://openlist.example.com/fnos/music",
		},
		{
			name:       "no scheme in address",
			address:    "192.168.1.10:5244",
			remotePath: "music",
			want:       "openlist://192.168.1.10:5244/music",
		},
		{
			name:       "empty remote path keeps just the endpoint",
			address:    "http://192.168.1.10:5244",
			remotePath: "",
			want:       "openlist://192.168.1.10:5244",
		},
		{
			// Contract with the admin UI (ui/src/library/cloudPath.test.js): whitespace and
			// slashes around the remote path are trimmed before escaping.
			name:       "whitespace and slashes around the remote path are trimmed",
			address:    "http://h:1",
			remotePath: "  /fnos/  ",
			want:       "openlist://h:1/fnos",
		},
		{
			name:       "whitespace around the address is trimmed",
			address:    "  nas.local:5244  ",
			remotePath: "fnos",
			want:       "openlist://nas.local:5244/fnos",
		},
		{name: "empty address", address: "", remotePath: "music", wantErr: true},
		{name: "blank address", address: "   ", remotePath: "music", wantErr: true},
		{name: "scheme with no host", address: "http://", remotePath: "music", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := BuildURI(c.address, c.remotePath)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("BuildURI(%q, %q) = %q, want %q", c.address, c.remotePath, got, c.want)
			}
		})
	}
}

func TestRemotePathOf(t *testing.T) {
	cases := []struct {
		root, name, want string
	}{
		{"fnos/music", ".", "/fnos/music"},
		{"fnos/music", "", "/fnos/music"},
		{"fnos/music", "artist/album/01.flac", "/fnos/music/artist/album/01.flac"},
		{"", ".", "/"},
		{"", "a/b", "/a/b"},
	}
	for _, c := range cases {
		if got := remotePathOf(c.root, c.name); got != c.want {
			t.Errorf("remotePathOf(%q, %q) = %q, want %q", c.root, c.name, got, c.want)
		}
	}
}

func TestCanonicalHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://192.168.1.10:5244", "192.168.1.10:5244"},
		{"http://192.168.1.10:5244/", "192.168.1.10:5244"},
		{"192.168.1.10:5244", "192.168.1.10:5244"},
		{"HTTPS://OpenList.Example.com", "openlist.example.com"},
		{"  nas  ", "nas"},
		{"", ""},
		{"http://", ""}, // scheme with no host must not become the host "http:"
		{"https://", ""},
		{"http:///", ""},
	}
	for _, c := range cases {
		if got := canonicalHost(c.in); got != c.want {
			t.Errorf("canonicalHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// filenameInfo: the 艺人/专辑/曲目 folder convention of the design doc §5
// ---------------------------------------------------------------------------

func TestFilenameInfo(t *testing.T) {
	cases := []struct {
		name, path string
		want       map[string]string
	}{
		{
			name: "artist/album/track",
			path: "周杰伦/叶惠美/以父之名.flac",
			want: map[string]string{"artist": "周杰伦", "album": "叶惠美", "title": "以父之名"},
		},
		{
			name: "track number with dash is stripped into tracknumber",
			path: "artist/album/01 - Intro.mp3",
			want: map[string]string{"artist": "artist", "album": "album", "title": "Intro", "tracknumber": "01"},
		},
		{
			name: "track number with dot",
			path: "artist/album/12.something.mp3",
			want: map[string]string{"artist": "artist", "album": "album", "title": "something", "tracknumber": "12"},
		},
		{
			name: "plain number prefix",
			path: "artist/album/3 Third Track.m4a",
			want: map[string]string{"artist": "artist", "album": "album", "title": "Third Track", "tracknumber": "3"},
		},
		{
			name: "no album folder",
			path: "artist/loose file.ogg",
			want: map[string]string{"artist": "artist", "title": "loose file"},
		},
		{
			name: "bare file name",
			path: "lonely.flac",
			want: map[string]string{"title": "lonely"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := filenameInfo(c.path)
			for key, want := range c.want {
				got := info.Tags[key]
				if len(got) != 1 || got[0] != want {
					t.Errorf("tag %q = %v, want [%q]", key, got, want)
				}
			}
			for key, got := range info.Tags {
				if _, ok := c.want[key]; !ok {
					t.Errorf("unexpected tag %q = %v", key, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// dirCache
// ---------------------------------------------------------------------------

func TestDirCacheTTL(t *testing.T) {
	c := newDirCache(20 * time.Millisecond)
	c.Set("a", []openlist.Entry{{Name: "x"}})

	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected a cached entry")
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("unexpected cache hit")
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected the entry to expire with the TTL")
	}
}

func TestDirCacheEvictsWhenFull(t *testing.T) {
	c := newDirCache(time.Hour)
	for i := 0; i < dirCacheMaxItems+10; i++ {
		c.Set(strings.Repeat("k", 3)+string(rune(i)), []openlist.Entry{{Name: "x"}})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) > dirCacheMaxItems {
		t.Fatalf("cache grew past its bound: %d items", len(c.items))
	}
}

// ---------------------------------------------------------------------------
// misc
// ---------------------------------------------------------------------------

func TestOrDefault(t *testing.T) {
	if orDefault(0, 7) != 7 {
		t.Error("orDefault should replace zero with the default")
	}
	if orDefault(-1, 7) != 7 {
		t.Error("orDefault should replace negatives with the default")
	}
	if orDefault(3, 7) != 3 {
		t.Error("orDefault should keep explicit values")
	}
}

func TestErrReadBudgetIsSentinel(t *testing.T) {
	wrapped := errors.New("x")
	if errors.Is(wrapped, errReadBudget) {
		t.Error("a plain error must not match the budget sentinel")
	}
}
