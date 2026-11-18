package online

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// 上游站点（flac.music.hi.cn）的下载入口在网页端是靠前端脚本拦截的：用户点第 6 次
// 下载图标后弹出“口令”弹窗，扫码换口令才能继续。正常情况下服务端调用 ajax 接口
// 完全不会碰到这套逻辑（见 docs/在线搜歌下载功能-开发文档.md）。
//
// 但这套限制随时可能被挪到服务端：一旦接口层开始返回 HTML 口令页、网关拦截页或
// 带“口令/验证”字样的错误，原来的 json.Unmarshal 只会抛出 “parse envelope: invalid
// character '<' …”，运维无法判断到底是本站 bug 还是上游拦截。这里把这类响应单独
// 建模，让上层能给出“上游要口令/被风控”的明确提示，并让下载源进入冷却。
var ErrUpstreamBlocked = errors.New("online: upstream blocked the request or returned a non-JSON page")

// 上游拦截类型，用于管理端提示文案与操作日志。
const (
	// BlockedKindPasswordGate：上游返回口令解锁页（或接口直接要求口令）。
	BlockedKindPasswordGate = "password-gate"
	// BlockedKindAntiBot：上游返回人机验证/风控页。
	BlockedKindAntiBot = "anti-bot"
	// BlockedKindNonJSON：上游返回了既不是 JSON 也不是已知拦截页的内容
	// （改版、网关错误页、被截断的响应等）。
	BlockedKindNonJSON = "non-json"
	// BlockedKindCooldown：本站在上游拦截后主动冷却，期间不发请求。
	BlockedKindCooldown = "cooldown"
)

// UpstreamBlockedError 描述一次具体的上游拦截，保留来源、动作与可读片段，
// 便于日志排查；Kind 供上层决定提示文案。
type UpstreamBlockedError struct {
	Source string // 下载源名，如 hicn
	Action string // 上游动作：search / getUrl / getLyric
	Kind   string // BlockedKind* 之一
	Detail string // 截断且清洗过的响应片段或上游 msg
}

func (e *UpstreamBlockedError) Error() string {
	detail := ""
	if e.Detail != "" {
		detail = ": " + e.Detail
	}
	return fmt.Sprintf("online/%s: upstream blocked (%s) during %s%s", e.Source, e.Kind, e.Action, detail)
}

// Unwrap lets callers use errors.Is(err, ErrUpstreamBlocked).
func (e *UpstreamBlockedError) Unwrap() error { return ErrUpstreamBlocked }

// IsUpstreamBlocked reports whether err is an upstream block and returns its kind.
func IsUpstreamBlocked(err error) (kind string, ok bool) {
	var blocked *UpstreamBlockedError
	if errors.As(err, &blocked) {
		return blocked.Kind, true
	}
	if errors.Is(err, ErrUpstreamBlocked) {
		return BlockedKindNonJSON, true
	}
	return "", false
}

// UpstreamBlockedNeutralText is the wording ordinary users (the APP, or any caller
// without the web-admin marker) get when the upstream blocks us. It says nothing
// about passwords or risk control, so the operator wording stays admin-only.
const UpstreamBlockedNeutralText = "在线下载源暂时不可用，请稍后重试"

// UpstreamBlockedHint returns the operator-facing explanation shown in the web
// admin UI. It intentionally names the gate so an operator can tell “上游要口令”
// apart from “我们自己的服务挂了”. Never send this text to the APP.
func UpstreamBlockedHint(kind string) string {
	switch kind {
	case BlockedKindPasswordGate:
		return "上游站点要求输入口令才能继续下载（浏览器里会弹出扫码页），服务端请求被拦下了。" +
			"本次操作未执行，稍后重试通常可恢复；若持续出现，说明该站已把口令校验移到了接口层，需要更换下载源。"
	case BlockedKindAntiBot:
		return "上游站点返回了人机验证/风控页，服务端请求被拦下。已暂停本次操作并进入冷却，" +
			"请降低批量并发或稍后重试。"
	case BlockedKindCooldown:
		return "上游刚拦下过请求，下载源正在冷却（口令/风控拦截按 10 分钟计，普通限流约 30 秒），" +
			"期间不会请求该站。稍后再试即可；已缓存的直链仍可直接使用。"
	default:
		return "上游返回了非接口内容（可能是验证页、网关拦截页或站点改版）。本次操作未执行，" +
			"稍后重试；持续出现则需要更换下载源。"
	}
}

// blockedKindFromText 依据响应正文判定拦截类型。返回 ok=false 表示正文里没有
// 任何口令/风控特征，调用方应退化为 BlockedKindNonJSON。
func blockedKindFromText(text string) (kind string, ok bool) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(text, "口令"),
		strings.Contains(text, "解锁"),
		strings.Contains(lower, "kl.php"),
		strings.Contains(lower, "11.php"),
		strings.Contains(lower, "请输入密码"):
		return BlockedKindPasswordGate, true
	case strings.Contains(text, "验证码"),
		strings.Contains(text, "人机"),
		strings.Contains(text, "风控"),
		strings.Contains(text, "访问过于频繁"),
		strings.Contains(text, "操作频繁"),
		strings.Contains(lower, "captcha"),
		strings.Contains(lower, "cf-challenge"):
		return BlockedKindAntiBot, true
	}
	return "", false
}

// classifyBlockedResponse 判定一次上游响应是否属于“不是我们的 JSON 接口”。
// body 为原始正文（响应头不参与判定：内容包括 JSON 就按接口响应处理，与
// Content-Type 声称的格式不一致时以正文为准，避免代理改写头导致误判）。
//
// 判定顺序：先确认是不是 JSON（'{' / '[' 开头即视为接口响应，交给调用方解析）；
// 否则按口令页、风控页、其它非 JSON 依次归类。正常 JSON 永不误判。
func classifyBlockedResponse(body []byte) (kind string, blocked bool) {
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")), " \t\r\n")
	if len(trimmed) == 0 {
		return BlockedKindNonJSON, true
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return "", false
	}
	if kind, ok := blockedKindFromText(string(trimmed)); ok {
		return kind, true
	}
	return BlockedKindNonJSON, true
}

// snippet 生成短小可读的正文片段，供日志与错误详情使用（清洗控制字符、按字符截断）。
func snippet(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	const limit = 120
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}
