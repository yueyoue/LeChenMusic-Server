package cloudsource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/navidrome/navidrome/core/storage"
)

// errRedirectDisabled is returned when the endpoint is configured with DisableRedirect,
// i.e. the operator wants every playback relayed through this server instead of 302'ing
// to the drive's CDN.
var errRedirectDisabled = errors.New("cloudsource: 302 direct links disabled by configuration")

// cloudStorage is the storage.Storage registered for the "openlist" scheme.
//
// A cloud library's Path is a storage URI of the form
//
//	openlist://<host:port>/<remote path>
//
// e.g. "openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90". The host:port selects
// the configured OpenList gateway (credentials live in the server config only), and the
// path is the folder inside that gateway to use as the library root.
type cloudStorage struct {
	u url.URL
}

func newCloudStorage(u url.URL) storage.Storage {
	return &cloudStorage{u: u}
}

// FS implements storage.Storage. It uses a background context; callers that own a
// context should use FSWithContext (storage.ContextualStorage) instead.
func (s *cloudStorage) FS() (storage.MusicFS, error) {
	return s.FSWithContext(context.Background())
}

// FSWithContext implements storage.ContextualStorage.
func (s *cloudStorage) FSWithContext(ctx context.Context) (storage.MusicFS, error) {
	if s.u.Host == "" {
		return nil, fmt.Errorf("cloudsource: invalid %s:// URI %q: missing endpoint host", SchemaID, s.u.String())
	}
	ep, err := EndpointFor(s.u.Host)
	if err != nil {
		return nil, err
	}
	root := strings.Trim(s.u.Path, "/")
	if ctx == nil {
		ctx = context.Background()
	}
	return &cloudFS{ctx: ctx, ep: ep, root: root}, nil
}

// DirectURL implements storage.DirectLinkProvider: it resolves a signed, short-lived
// download link for the given library-relative path. Used by the streaming layer to
// answer with an HTTP 302 straight to the drive's CDN (媒体数据不经过 NAS).
func (s *cloudStorage) DirectURL(ctx context.Context, name string) (string, time.Time, error) {
	ep, err := EndpointFor(s.u.Host)
	if err != nil {
		return "", time.Time{}, err
	}
	if !ep.supportsRedirect() {
		return "", time.Time{}, errRedirectDisabled
	}
	return ep.directURL(ctx, remotePathOf(strings.Trim(s.u.Path, "/"), name))
}

// BuildURI composes a `openlist://<host:port>/<remote path>` storage URI from a raw
// OpenList address and a remote folder path. It is the single source of truth shared by
// the admin UI/API, so that what the user typed is exactly what lands in the DB.
func BuildURI(address, remotePath string) (string, error) {
	host := canonicalHost(address)
	if host == "" {
		return "", fmt.Errorf("cloudsource: empty OpenList address")
	}
	remotePath = strings.Trim(remotePath, "/")
	u := url.URL{Scheme: SchemaID, Host: host}
	if remotePath != "" {
		// Set the *unescaped* path: url.URL.String() applies the per-component escaping
		// itself, so escaping here would double-encode non-ASCII components.
		u.Path = "/" + remotePath
	}
	return u.String(), nil
}

func init() {
	storage.Register(SchemaID, newCloudStorage)
}
