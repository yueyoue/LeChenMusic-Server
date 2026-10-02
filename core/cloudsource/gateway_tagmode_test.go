package cloudsource

import (
	"testing"

	"github.com/navidrome/navidrome/conf"
)

// 评审 P2-11: the audiobook scanner asks the gateway's tag mode to decide between
// exact tag reads and the zero-download "filename" fast mode.

func TestTagModeForLibrary(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{
		"nas":   {URL: "http://192.168.1.10:5244", Token: "t", TagMode: "filename"},
		"quark": {URL: "http://pan.example.com:5244", Token: "t"},
	})

	if got := TagModeForLibrary("openlist://192.168.1.10:5244/books"); got != "filename" {
		t.Fatalf("expected filename mode for nas, got %q", got)
	}
	if got := TagModeForLibrary("openlist://pan.example.com:5244/books"); got != "" {
		t.Fatalf("unset TagMode must read as scan (empty), got %q", got)
	}
	if got := TagModeForLibrary("/vol1/music"); got != "" {
		t.Fatalf("local libraries have no gateway tag mode, got %q", got)
	}
}
