package storage

import (
	"context"
	"io/fs"
	"time"

	"github.com/navidrome/navidrome/model/metadata"
)

type Storage interface {
	FS() (MusicFS, error)
}

// MusicFS is an interface that extends the fs.FS interface with the ability to read tags from files
type MusicFS interface {
	fs.FS
	ReadTags(path ...string) (map[string]metadata.Info, error)
}

// ContextualStorage is an optional interface for storages whose I/O can be canceled
// (e.g. network-backed cloud sources). When a Storage implements it, callers that own a
// context should prefer FSWithContext over FS, so that a canceled scan stops hitting the
// remote endpoint. Storages that only implement Storage keep working unchanged.
type ContextualStorage interface {
	FSWithContext(ctx context.Context) (MusicFS, error)
}

// DirectLinkProvider is an optional interface for storages whose files can be served
// straight from the origin (e.g. a cloud drive's CDN) instead of proxied through this
// server. It backs the "302 direct link" playback mode of the cloud media source feature.
//
// The returned URL is a *signed, short-lived* link: callers must fetch a fresh one for
// every playback and must never log or cache it.
type DirectLinkProvider interface {
	// DirectURL returns a direct download URL for the file at the given
	// library-relative path, plus its expiry time (zero when unknown).
	DirectURL(ctx context.Context, path string) (rawURL string, expiresAt time.Time, err error)
}

// Watcher is a storage with the ability watch the FS and notify changes
type Watcher interface {
	// Start starts a watcher on the whole FS and returns a channel to send detected changes.
	// The watcher must be stopped when the context is done.
	Start(context.Context) (<-chan string, error)
}
