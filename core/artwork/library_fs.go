package artwork

import (
	"context"
	"path/filepath"

	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
)

// libraryView bundles the MusicFS for a library with its absolute root path,
// so readers can open library-relative paths through FS and compose absolute
// paths (for ffmpeg, which is path-based) via Abs.
type libraryView struct {
	FS      storage.MusicFS
	absRoot string
}

// Abs returns the absolute path for a library-relative path. Returns "" when
// there is no usable local path — an empty rel, or a remote (cloud) library
// root. ffmpeg is a path-based subprocess and cannot read openlist://… URIs, so
// callers (fromFFmpegTag) treat "" as "no path available" and fall through to
// the next artwork source instead of spawning a doomed subprocess.
func (v libraryView) Abs(rel string) string {
	if rel == "" || storage.IsRemoteURI(v.absRoot) {
		return ""
	}
	return filepath.Join(v.absRoot, rel)
}

// loadLibraryView resolves the MusicFS and absolute root path in a single
// library lookup.
func loadLibraryView(ctx context.Context, ds model.DataStore, libID int) (libraryView, error) {
	lib, err := ds.Library(ctx).Get(libID)
	if err != nil {
		return libraryView{}, err
	}
	s, err := storage.For(lib.Path)
	if err != nil {
		return libraryView{}, err
	}
	fs, err := s.FS()
	if err != nil {
		return libraryView{}, err
	}
	return libraryView{FS: fs, absRoot: lib.Path}, nil
}
