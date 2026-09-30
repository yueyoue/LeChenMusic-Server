package cloudsource

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 直链 TTL 缓存回归（评审后优化）：拖动进度条时每个 Range 请求都解析新直链 =
// 每次一个 /api/fs/get + 网关节流 ≥2s，seek 体感卡顿的主要来源之一。
// 口径：只缓存带过期时间的链接，服务到 expiresAt-linkRefreshMargin；
// 未报告过期时间不缓存（与 remotefile_refresh_test 锁定的语义一致）。

func directURLTwice(t *testing.T, fsys *cloudFS) (string, string) {
	t.Helper()
	ctx := context.Background()
	u1, _, err := fsys.DirectURL(ctx, testRelPath)
	if err != nil {
		t.Fatalf("first DirectURL: %v", err)
	}
	u2, _, err := fsys.DirectURL(ctx, testRelPath)
	if err != nil {
		t.Fatalf("second DirectURL: %v", err)
	}
	return u1, u2
}

// 有效期内的直链必须复用：拖动进度条的第 2..N 次请求不再打网关。
func TestDirectURLCachedWithinValidity(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	f.expiry = time.Now().Add(1 * time.Hour)
	fsys := setup(t, f, nil)

	u1, u2 := directURLTwice(t, fsys)
	if u1 != u2 {
		t.Fatalf("cached link must be reused, got %q vs %q", u1, u2)
	}
	if f.gets != 1 {
		t.Fatalf("expected 1 gateway resolution for a cached link, got %d", f.gets)
	}
}

// 网关未报告过期时间的链接不缓存（行为零变化）：两次解析 = 两次 /api/fs/get。
func TestDirectURLNotCachedWithoutReportedExpiry(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	fsys := setup(t, f, nil)

	directURLTwice(t, fsys)
	if f.gets != 2 {
		t.Fatalf("links without reported expiry must not be cached, got %d resolutions for 2 calls", f.gets)
	}
}

// 已进入 refresh margin 的链接视同过期，不从缓存服务（口径与 remoteFile 一致）。
func TestDirectURLNotCachedInsideRefreshMargin(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	f.expiry = time.Now().Add(1 * time.Minute) // 在 linkRefreshMargin(2min) 之内
	fsys := setup(t, f, nil)

	directURLTwice(t, fsys)
	if f.gets != 2 {
		t.Fatalf("links inside the refresh margin must not be served from cache, got %d resolutions", f.gets)
	}
}

// 读失败后的强制换链必须绕过缓存：绝不能把刚死的签名原样再喂一次。
func TestDirectURLFreshBypassesCache(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	f.expiry = time.Now().Add(1 * time.Hour)
	fsys := setup(t, f, nil)

	ctx := context.Background()
	u1, _, err := fsys.DirectURL(ctx, testRelPath)
	if err != nil {
		t.Fatalf("first DirectURL: %v", err)
	}
	u2, _, err := fsys.ep.directURLFresh(ctx, remotePathOf(testRoot[1:], testRelPath))
	if err != nil {
		t.Fatalf("directURLFresh: %v", err)
	}
	if u1 == u2 {
		t.Fatal("forced refresh must resolve a new link, got the cached one back")
	}
	if f.gets != 2 {
		t.Fatalf("expected 2 gateway resolutions (cached + forced), got %d", f.gets)
	}
}

// 并发解析同一文件合并为一次网关往返（seek 突发场景）。
func TestDirectURLCoalescesConcurrentResolves(t *testing.T) {
	f := newFakeOpenList(t)
	f.addFile(testFilePath, pattern(200000))
	f.enforceSign = true
	f.expiry = time.Now().Add(1 * time.Hour)
	fsys := setup(t, f, nil)

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := fsys.DirectURL(ctx, testRelPath); err != nil {
				t.Errorf("concurrent DirectURL: %v", err)
			}
		}()
	}
	wg.Wait()
	if f.gets != 1 {
		t.Fatalf("concurrent resolves must coalesce into one gateway call, got %d", f.gets)
	}
}
