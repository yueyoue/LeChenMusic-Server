package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	onlinesvc "github.com/navidrome/navidrome/server/online"
)

// 上游拦截的提示必须按调用方分层：网页管理端拿到可操作的中文说明（含“需要换源”），
// 其它客户端（APP 普通用户）只拿到中性文案，且两者都不含原始上游正文。
func TestWriteSourceErrorLayers(t *testing.T) {
	blocked := fmt.Errorf("resolve media URL: %w", &onlinesvc.UpstreamBlockedError{
		Source: "hicn", Action: "getUrl", Kind: onlinesvc.BlockedKindPasswordGate,
		Detail: "<html>请输入密码 kl.php</html>",
	})

	t.Run("web-admin gets operator hint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/online/search", nil)
		req.Header.Set(onlineClientHeader, onlineClientWebAdmin)
		rec := httptest.NewRecorder()
		writeSourceError(rec, req, blocked, "search failed: ")

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want json", ct)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON body: %v (%s)", err, rec.Body.String())
		}
		if body["code"] != onlineUpstreamErrorKey {
			t.Errorf("code = %q, want %q", body["code"], onlineUpstreamErrorKey)
		}
		if body["kind"] != onlinesvc.BlockedKindPasswordGate {
			t.Errorf("kind = %q, want %q", body["kind"], onlinesvc.BlockedKindPasswordGate)
		}
		if !strings.Contains(body["error"], "更换下载源") {
			t.Errorf("admin hint should suggest switching source, got %q", body["error"])
		}
		if strings.Contains(body["error"], "kl.php") {
			t.Errorf("admin hint must not leak raw upstream body, got %q", body["error"])
		}
	})

	t.Run("app gets neutral text", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/online/search", nil)
		rec := httptest.NewRecorder()
		writeSourceError(rec, req, blocked, "search failed: ")

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON body: %v (%s)", err, rec.Body.String())
		}
		if body["error"] != onlinesvc.UpstreamBlockedNeutralText {
			t.Errorf("error = %q, want neutral %q", body["error"], onlinesvc.UpstreamBlockedNeutralText)
		}
		for _, leak := range []string{"口令", "风控", "验证", "换源", "kl.php"} {
			if strings.Contains(body["error"], leak) {
				t.Errorf("app response leaked %q: %q", leak, body["error"])
			}
		}
	})

	t.Run("non-blocked error keeps original text", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/online/search", nil)
		req.Header.Set(onlineClientHeader, onlineClientWebAdmin)
		rec := httptest.NewRecorder()
		writeSourceError(rec, req, fmt.Errorf("dial tcp: timeout"), "search failed: ")

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
		if body := rec.Body.String(); !strings.Contains(body, "dial tcp: timeout") {
			t.Errorf("body = %q, want original error text", body)
		}
	})
}