// Package cloudsource implements the "cloud media source" (网盘媒体源) feature: media
// libraries whose files live on a network drive (阿里云盘/夸克/115/百度) behind an
// OpenList gateway (https://github.com/OpenListTeam/OpenList).
//
// Design (see docs/网盘媒体源方案设计.md): 网盘内容只索引、不搬运.
//
//	listing  -> POST /api/fs/list   (pure JSON, very light)  -> build the library
//	tags     -> HTTP Range on the direct link, head/tail bytes only, never a full download
//	playback -> POST /api/fs/get -> raw_url -> HTTP 302 -> player pulls from the drive's CDN
//
// The package plugs into the existing storage registry (core/storage) under the
// "openlist" scheme, so a cloud library is just a library whose Path is
//
//	openlist://<host:port>/<remote path>
//
// and the scanner/artwork/streams keep using storage.MusicFS without knowing about
// cloud drives at all.
//
// Risk-control (风控) rules honoured here, per the design doc §4:
//   - every upstream call goes through adapters/openlist (token bucket + jitter +
//     backoff + circuit breaker);
//   - file contents are only ever read in bounded byte ranges (see remoteFile), with a
//     hard per-file budget so a misbehaving tag parser can never download a whole file;
//   - directory listings are cached and only re-fetched when the TTL expires.
package cloudsource

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// SchemaID is the storage scheme registered for cloud media sources.
const SchemaID = "openlist"

// Defaults, all overridable per endpoint through conf.Server.OpenList.<name>.
const (
	defaultListInterval     = 2 * time.Second
	defaultReadInterval     = 500 * time.Millisecond
	defaultJitterFraction   = 0.2
	defaultMaxRetries       = 3
	defaultRetryBaseDelay   = 500 * time.Millisecond
	defaultFailureThreshold = 5
	defaultCircuitOpenFor   = 30 * time.Minute

	defaultHeadBytes       = int64(2 << 20)  // 2MB head, per the design doc
	defaultTailBytes       = int64(2 << 20)  // 2MB tail, to find trailing MP4 moov boxes
	defaultWindowBytes     = int64(256 << 10) // 256KB minimum sliding window
	defaultMaxTagReadBytes = int64(16 << 20) // hard budget for one tag extraction
	defaultDirCacheTTL     = 5 * time.Minute
)

// Endpoint is one configured OpenList gateway, shared by every cloud library that
// points at it. It owns the throttled API client, the Range fetcher used for lazy
// tag/cover reads and the directory listing cache, so that all libraries hitting the
// same gateway share a single token bucket (风控: one account, one instance).
type Endpoint struct {
	Name    string
	BaseURL string

	client  *openlist.Client
	fetcher *openlist.RangeFetcher

	headBytes       int64
	tailBytes       int64
	windowBytes     int64
	maxTagReadBytes int64
	enableRedirect  bool

	dirs *dirCache
}

// RemoteRoot is the part of a remote path below the gateway's drive root.
func (e *Endpoint) supportsRedirect() bool { return e.enableRedirect }

var (
	endpointsMu sync.RWMutex
	endpoints   = map[string]*Endpoint{}
)

// resetEndpoints drops every cached endpoint. Called from conf.AddHook so config
// reloads (and tests) always rebuild clients with the new credentials.
func resetEndpoints() {
	endpointsMu.Lock()
	defer endpointsMu.Unlock()
	endpoints = map[string]*Endpoint{}
}

func init() {
	conf.AddHook(resetEndpoints)
}

// EndpointFor resolves the OpenList gateway referenced by the host part of a
// `openlist://<host:port>/<path>` storage URI. The result is cached and shared.
//
// Matching rules (in order):
//  1. the map key in conf.Server.OpenList equals the URI host (or its host:port);
//  2. the configured URL's host:port equals the URI host;
//  3. exactly one gateway is configured -> use it (with a warning).
func EndpointFor(hostport string) (*Endpoint, error) {
	key := canonicalHost(hostport)

	endpointsMu.RLock()
	ep := endpoints[key]
	endpointsMu.RUnlock()
	if ep != nil {
		return ep, nil
	}

	ep, err := buildEndpoint(hostport, key)
	if err != nil {
		return nil, err
	}

	endpointsMu.Lock()
	defer endpointsMu.Unlock()
	if prev := endpoints[key]; prev != nil {
		return prev, nil
	}
	endpoints[key] = ep
	return ep, nil
}

func buildEndpoint(hostport, key string) (*Endpoint, error) {
	cfg := conf.Server.OpenList
	if len(cfg) == 0 {
		return nil, fmt.Errorf("cloudsource: no OpenList gateway configured (add [OpenList.<name>] to the server config)")
	}

	var match *conf.OpenListOptions
	matchName := ""
	for name, opts := range cfg {
		o := opts
		switch {
		case canonicalHost(name) == key:
			match, matchName = &o, name
		case canonicalHost(urlHost(o.URL)) == key:
			match, matchName = &o, name
		}
		if match != nil {
			break
		}
	}

	if match == nil {
		if len(cfg) == 1 {
			for name, opts := range cfg {
				o := opts
				match, matchName = &o, name
			}
			log.Warn("cloudsource: library points to an unknown OpenList endpoint, falling back to the only configured one",
				"requested", hostport, "using", matchName)
		} else {
			names := make([]string, 0, len(cfg))
			for name := range cfg {
				names = append(names, name)
			}
			return nil, fmt.Errorf("cloudsource: no credentials configured for OpenList endpoint %q (configured: %s)", hostport, strings.Join(names, ", "))
		}
	}

	if match.URL == "" {
		return nil, fmt.Errorf("cloudsource: OpenList endpoint %q has no URL configured", matchName)
	}

	ep := &Endpoint{
		Name:            matchName,
		BaseURL:         strings.TrimRight(match.URL, "/"),
		headBytes:       orDefault(match.HeadBytes, defaultHeadBytes),
		tailBytes:       orDefault(match.TailBytes, defaultTailBytes),
		windowBytes:     orDefault(match.WindowBytes, defaultWindowBytes),
		maxTagReadBytes: orDefault(match.MaxTagReadBytes, defaultMaxTagReadBytes),
		enableRedirect:  !match.DisableRedirect,
		dirs:            newDirCache(orDefaultDuration(match.DirCacheTTL, defaultDirCacheTTL)),
	}

	jitter := match.JitterFraction
	if jitter == 0 {
		jitter = defaultJitterFraction
	}

	ep.client = openlist.NewClient(openlist.Config{
		BaseURL:         ep.BaseURL,
		Username:        match.Username,
		Password:        match.Password,
		Token:           match.Token,
		ListInterval:    orDefaultDuration(match.ListInterval, defaultListInterval),
		GetInterval:     orDefaultDuration(match.ReadInterval, defaultReadInterval),
		JitterFraction:  jitter,
		MaxRetries:      int(orDefault(int64(match.MaxRetries), defaultMaxRetries)),
		RetryBaseDelay:  orDefaultDuration(match.RetryBaseDelay, defaultRetryBaseDelay),
		FailureThreshold: int(orDefault(int64(match.FailureThreshold), defaultFailureThreshold)),
		CircuitOpenFor:  orDefaultDuration(match.CircuitOpenFor, defaultCircuitOpenFor),
	})
	ep.fetcher = openlist.NewRangeFetcher(openlist.RangeFetcherOptions{
		MinInterval:    orDefaultDuration(match.ReadInterval, defaultReadInterval),
		JitterFraction: jitter,
		MaxRetries:     int(orDefault(int64(match.MaxRetries), defaultMaxRetries)),
		RetryBaseDelay: orDefaultDuration(match.RetryBaseDelay, defaultRetryBaseDelay),
	})

	log.Debug("cloudsource: OpenList endpoint ready", "name", matchName, "url", ep.BaseURL,
		"head", ep.headBytes, "tail", ep.tailBytes, "maxTagRead", ep.maxTagReadBytes, "redirect", ep.enableRedirect)
	return ep, nil
}

// canonicalHost normalises a host, host:port or full URL into a lookup key.
func canonicalHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		s = urlHost(s)
	}
	return strings.ToLower(s)
}

// urlHost extracts "host:port" from a URL, tolerating a missing scheme.
func urlHost(raw string) string {
	raw = strings.TrimSpace(strings.TrimRight(raw, "/"))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Host
}

func orDefault(v, def int64) int64 {
	if v <= 0 {
		return def
	}
	return v
}

func orDefaultDuration(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

// dirCache is a small TTL cache of directory listings. Listings are the most repeated
// call of a scan, and re-reading them is exactly what the 风控 rules forbid, so they are
// strongly cached (the design doc: "目录列表强缓存").
type dirCache struct {
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]dirCacheItem
}

type dirCacheItem struct {
	entries []openlist.Entry
	expires time.Time
}

const dirCacheMaxItems = 8192

func newDirCache(ttl time.Duration) *dirCache {
	return &dirCache{ttl: ttl, items: map[string]dirCacheItem{}}
}

func (c *dirCache) Get(key string) ([]openlist.Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if !ok || time.Now().After(item.expires) {
		return nil, false
	}
	return item.entries, true
}

func (c *dirCache) Set(key string, entries []openlist.Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= dirCacheMaxItems {
		now := time.Now()
		for k, v := range c.items {
			if now.After(v.expires) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= dirCacheMaxItems {
			c.items = map[string]dirCacheItem{}
		}
	}
	c.items[key] = dirCacheItem{entries: entries, expires: time.Now().Add(c.ttl)}
}
