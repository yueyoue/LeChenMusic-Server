package online

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 上游正常返回时绝不能误判成拦截：只要正文是 JSON 对象/数组就放行。
func TestClassifyBlockedResponseAcceptsJSON(t *testing.T) {
	cases := map[string]string{
		"ok envelope":   `{"code":0,"msg":"ok","data":{"total":0,"list":[]}}`,
		"code!=0":       `{"code":-2,"msg":"参数校验失败"}`,
		"array":         `[{"id":"1"}]`,
		"leading space": "  \n\t{\"code\":0}",
		"utf8 BOM":      "\xef\xbb\xbf{\"code\":0}",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			kind, blocked := classifyBlockedResponse([]byte(body))
			assert.False(t, blocked, "JSON body must not be treated as blocked")
			assert.Empty(t, kind)
		})
	}
}

// 口令页：站点 11.php 的正文特征（口令/解锁/kl.php）应归为 password-gate。
func TestClassifyBlockedResponseDetectsPasswordGate(t *testing.T) {
	body := `<h4>免费获取口令即可解锁下载功能</h4><li>使用快手扫一扫下面的二维码</li>
	<input id="kl" placeholder="请输入口令"><script src="kl.php"></script>`
	kind, blocked := classifyBlockedResponse([]byte(body))
	assert.True(t, blocked)
	assert.Equal(t, BlockedKindPasswordGate, kind)
}

// 风控页：验证码/访问频繁等特征归为 anti-bot。
func TestClassifyBlockedResponseDetectsAntiBot(t *testing.T) {
	body := `<html><body>访问过于频繁，请完成人机验证后再试</body></html>`
	kind, blocked := classifyBlockedResponse([]byte(body))
	assert.True(t, blocked)
	assert.Equal(t, BlockedKindAntiBot, kind)
}

// 既不是 JSON 也不是已知拦截页（网关错误页、被截断响应、空响应）归为 non-json。
func TestClassifyBlockedResponseFallsBackToNonJSON(t *testing.T) {
	cases := map[string][]byte{
		"gateway html": []byte(`<!DOCTYPE html><html><body>502 Bad Gateway</body></html>`),
		"plain text":   []byte(`error`),
		"empty":        {},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			kind, blocked := classifyBlockedResponse(body)
			assert.True(t, blocked)
			assert.Equal(t, BlockedKindNonJSON, kind)
		})
	}
}

// 接口用 code!=0 + msg 表达口令要求时，也要识别成口令拦截。
func TestBlockedKindFromTextJSONMessage(t *testing.T) {
	kind, ok := blockedKindFromText("请输入口令后重试")
	assert.True(t, ok)
	assert.Equal(t, BlockedKindPasswordGate, kind)

	_, ok = blockedKindFromText("参数校验失败")
	assert.False(t, ok)
}

func TestIsUpstreamBlocked(t *testing.T) {
	kind, ok := IsUpstreamBlocked(&UpstreamBlockedError{Source: "hicn", Action: "search", Kind: BlockedKindPasswordGate})
	assert.True(t, ok)
	assert.Equal(t, BlockedKindPasswordGate, kind)

	wrapped := fmt.Errorf("search: %w", &UpstreamBlockedError{Kind: BlockedKindAntiBot})
	kind, ok = IsUpstreamBlocked(wrapped)
	assert.True(t, ok, "errors.As must find blocked errors wrapped by callers")
	assert.Equal(t, BlockedKindAntiBot, kind)

	kind, ok = IsUpstreamBlocked(fmt.Errorf("online/hicn: %s failed: code=-2", "getUrl"))
	assert.False(t, ok)
	assert.Empty(t, kind)
}

// 管理端提示必须点名“口令”，但通用文案不能泄露该细节（APP 普通用户看到的是通用文案）。
func TestUpstreamBlockedHint(t *testing.T) {
	for _, kind := range []string{BlockedKindPasswordGate, BlockedKindAntiBot, BlockedKindCooldown, BlockedKindNonJSON, "unknown"} {
		hint := UpstreamBlockedHint(kind)
		assert.NotEmpty(t, hint)
		assert.False(t, strings.HasPrefix(hint, "online/"), "hint must be human readable: %s", hint)
	}
	assert.Contains(t, UpstreamBlockedHint(BlockedKindPasswordGate), "口令")
}

// 冷却期间应直接失败，不再发请求。
func TestHicnCooldownBlocksRequests(t *testing.T) {
	src := NewHicnSource("https://example.invalid")
	src.setCooldown(hicnBlockedCooldown)

	err := src.cooldownError("search")
	if err == nil {
		t.Fatal("expected cooldown error while cooling down")
	}
	kind, ok := IsUpstreamBlocked(err)
	assert.True(t, ok)
	assert.Equal(t, BlockedKindCooldown, kind)

	src.setCooldown(-time.Hour)
	assert.NoError(t, src.cooldownError("search"), "expired cooldown must allow requests")
}

// 任务行文案面向 APP：上游拦截必须降级为中性句子，不带口令字样。
func TestTaskErrorTextHidesUpstreamDetail(t *testing.T) {
	blocked := &UpstreamBlockedError{Source: "hicn", Action: "getUrl", Kind: BlockedKindPasswordGate, Detail: "请输入口令"}
	msg := taskErrorText(context.Background(), blocked)
	assert.Equal(t, "在线下载源暂时不可用，请稍后重试", msg)
	assert.NotContains(t, msg, "口令")

	plain := errors.New("online/hicn: getUrl returned status 404")
	assert.Equal(t, plain.Error(), taskErrorText(context.Background(), plain), "non-blocked errors keep their message")
}

// 上游把口令页搬回接口层（HTTP 200 + HTML）时：搜索必须失败并带上明确类型，
// 同时进入冷却——后续请求不再打上游，避免把临时拦截撞成长期封禁。
func TestHicnSearchDetectsHTMLGateAndEntersCooldown(t *testing.T) {
	var hits int32
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(`<h4>免费获取口令即可解锁下载功能</h4>`))
	})

	_, err := s.SearchSong(context.Background(), SearchRequest{Keyword: "海阔天空"})
	if err == nil {
		t.Fatal("expected upstream block error")
	}
	kind, blocked := IsUpstreamBlocked(err)
	assert.True(t, blocked)
	assert.Equal(t, BlockedKindPasswordGate, kind)
	assert.Greater(t, s.cooldownRemaining(), time.Duration(0))

	// 冷却期内直接失败，不再发出请求。
	_, err = s.SearchSong(context.Background(), SearchRequest{Keyword: "海阔天空"})
	kind, blocked = IsUpstreamBlocked(err)
	assert.True(t, blocked)
	assert.Equal(t, BlockedKindCooldown, kind)
	assert.EqualValues(t, 1, atomic.LoadInt32(&hits), "cooldown must not touch the upstream")
}

// 429 只是一次限流：写入短冷却后重试要等冷却过去，最终仍能成功拿到结果。
func TestHicnRetryWaitsOutRateLimitCooldown(t *testing.T) {
	orig := hicnRateLimitCooldown
	hicnRateLimitCooldown = 50 * time.Millisecond
	t.Cleanup(func() { hicnRateLimitCooldown = orig })

	var hits int32
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchResponse))
	})

	res, err := s.SearchSong(context.Background(), SearchRequest{Keyword: "海阔天空"})
	assert.NoError(t, err)
	if assert.NotNil(t, res) {
		assert.Equal(t, "海阔天空", res.Songs[0].Name)
	}
	assert.EqualValues(t, 2, atomic.LoadInt32(&hits), "one 429 then a successful retry")
}

// 空正文/截断响应同样是上游异常，不能伪装成“本地解析 bug”。
func TestHicnSearchDetectsTruncatedJSON(t *testing.T) {
	s, _ := newTestHicn(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"list":[{"id":"1"`))
	})
	_, err := s.SearchSong(context.Background(), SearchRequest{Keyword: "海阔天空"})
	kind, blocked := IsUpstreamBlocked(err)
	assert.True(t, blocked)
	assert.Equal(t, BlockedKindNonJSON, kind)
}
