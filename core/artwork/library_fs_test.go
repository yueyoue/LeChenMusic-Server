package artwork

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLibraryViewAbs(t *testing.T) {
	t.Run("local library joins the relative path", func(t *testing.T) {
		v := libraryView{absRoot: "/music"}
		assert.Equal(t, "/music/a/b.mp3", v.Abs("a/b.mp3"))
	})

	t.Run("empty rel yields no path", func(t *testing.T) {
		v := libraryView{absRoot: "/music"}
		assert.Equal(t, "", v.Abs(""))
	})

	t.Run("remote (cloud) library yields no path for ffmpeg", func(t *testing.T) {
		// ffmpeg is a path-based subprocess and cannot read openlist:// URIs;
		// Abs must return "" so fromFFmpegTag short-circuits instead of
		// spawning a doomed subprocess (see sources.go).
		v := libraryView{absRoot: "openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90"}
		assert.Equal(t, "", v.Abs("a/b.mp3"))
	})
}
