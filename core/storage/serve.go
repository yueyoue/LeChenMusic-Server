package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
)

// FSFor returns a context-aware MusicFS handle for the given library path. Network-backed
// storages (cloud sources) implement ContextualStorage so that cancelling ctx stops the
// handle from hitting the remote endpoint mid-request.
func FSFor(ctx context.Context, libraryPath string) (MusicFS, error) {
	st, err := For(libraryPath)
	if err != nil {
		return nil, err
	}
	if cs, ok := st.(ContextualStorage); ok {
		return cs.FSWithContext(ctx)
	}
	return st.FS()
}

// DirectURL returns the signed, short-lived download link for relPath when the library is a
// cloud source whose gateway can hand one out. The URL is temporary credential material:
// it must only ever be used to answer a request (302 Location) and must never be logged,
// cached or written to a response body (design doc §15.1).
func DirectURL(ctx context.Context, libraryPath, relPath string) (string, bool) {
	st, err := For(libraryPath)
	if err != nil {
		return "", false
	}
	dlp, ok := st.(DirectLinkProvider)
	if !ok {
		return "", false
	}
	raw, _, err := dlp.DirectURL(ctx, relPath)
	if err != nil || raw == "" {
		return "", false
	}
	return raw, true
}

// ServeFile serves a single file from a library over HTTP, transparently handling local
// folders and cloud sources (openlist://...) with the same call.
//
// Ladder (design doc §2.2):
//
//  1. Cloud source with a usable direct link -> 302 to the drive's CDN. The media bytes
//     never pass through this server, and no auth headers of ours are forwarded.
//  2. Otherwise -> relay through this server using http.ServeContent, which keeps
//     Range/seek support working (clients depend on it for seeking in long chapters).
//
// relPath is the library-relative, slash-separated path as stored in the database.
func ServeFile(ctx context.Context, w http.ResponseWriter, r *http.Request, libraryPath, relPath string) error {
	if raw, ok := DirectURL(ctx, libraryPath, relPath); ok {
		http.Redirect(w, r, raw, http.StatusFound)
		return nil
	}

	fsys, err := FSFor(ctx, libraryPath)
	if err != nil {
		return err
	}
	info, err := fs.Stat(fsys, relPath)
	if err != nil {
		return err
	}
	f, err := fsys.Open(relPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return fmt.Errorf("storage: %s is not seekable", relPath)
	}

	name := info.Name()
	if name == "" || name == "." || name == "/" {
		name = path.Base(relPath)
	}
	http.ServeContent(w, r, name, info.ModTime(), rs)
	return nil
}
