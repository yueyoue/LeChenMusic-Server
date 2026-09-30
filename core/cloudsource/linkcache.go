package cloudsource

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// linkCache memoises signed direct links in memory so a seek burst pays one /api/fs/get
// instead of one per Range request. Every gateway resolution goes through the request
// throttle (≥2s per get), which is what makes scrubbing a cloud track feel stuck.
//
// 红线合规：签名链接只驻内存、不落盘、不进日志（设计文档 §2.2/§15.1 的"不持久化"语义
// 不变）。只有"网关报告了过期时间"的链接才缓存，且只服务到 expiresAt-linkRefreshMargin
// （与 remoteFile 主动续期口径一致）；未报告过期时间的链接不缓存，保持每次解析的现状
// 语义（remotefile_refresh_test 锁定：新解析可能让旧签名失效）。
type linkCache struct {
	mu      sync.Mutex
	entries map[string]linkEntry
	group   singleflight.Group
}

type linkEntry struct {
	url       string
	expiresAt time.Time
}

func newLinkCache() *linkCache {
	return &linkCache{entries: map[string]linkEntry{}}
}

// get returns the cached link while it is still safely usable — linkRefreshMargin before
// the reported expiry — and drops it once it is not.
func (c *linkCache) get(key string) (string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", time.Time{}, false
	}
	if !time.Now().Before(e.expiresAt.Add(-linkRefreshMargin)) {
		delete(c.entries, key)
		return "", time.Time{}, false
	}
	return e.url, e.expiresAt, true
}

// put stores a resolved link. Links without a reported expiry are not cached: the gateway
// may invalidate them at any time and callers rely on fresh-resolution semantics.
// Stale entries are swept on write so a long-running server cannot grow unbounded.
func (c *linkCache) put(key, url string, expiresAt time.Time) {
	if expiresAt.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.entries {
		if !now.Before(v.expiresAt.Add(-linkRefreshMargin)) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = linkEntry{url: url, expiresAt: expiresAt}
}

// invalidate drops a link so the next resolve talks to the gateway again. Used after a
// fetch failed on the cached link: the same signature must never be reused.
func (c *linkCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
