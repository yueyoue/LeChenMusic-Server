package nativeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 P2-4 (云源管理面板): gateway status list, connectivity probe
// and the 启停 switch. Red lines: credentials never appear in responses, and the panel
// never reads file content.

func gatewayFixture(t *testing.T, libs model.Libraries) *cloudSourceHandler {
	t.Helper()
	tests.Init(t, false)
	restore := conf.SnapshotConfig()
	t.Cleanup(restore)
	conf.Server.OpenList = map[string]conf.OpenListOptions{
		"nas":   {URL: "http://192.168.1.10:5244", Username: "admin", Password: "secret"},
		"quark": {URL: "http://pan.example.com:5244", Token: "super-secret-token"},
	}

	libRepo := &tests.MockLibraryRepo{}
	libRepo.SetData(libs)
	ds := &tests.MockDataStore{
		MockedLibrary:  libRepo,
		MockedProperty: &tests.MockedPropertyRepo{},
	}
	return &cloudSourceHandler{ds: ds}
}

func adminRequest(t *testing.T, method, target string, body string) *http.Request {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	ctx := request.WithUser(req.Context(), model.User{ID: "admin-1", UserName: "admin", IsAdmin: true})
	return req.WithContext(ctx)
}

func gatewayPathParam(req *http.Request, name string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", name)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func decodeGatewayList(t *testing.T, w *httptest.ResponseRecorder) []cloudGatewayInfo {
	t.Helper()
	var resp struct {
		Data cloudGatewayListResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode gateway list: %v (body: %s)", err, w.Body.String())
	}
	return resp.Data.Gateways
}

func TestCloudGatewaysListsBoundLibrariesAndAggregates(t *testing.T) {
	scanned := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	h := gatewayFixture(t, model.Libraries{
		{ID: 1, Name: "本地音乐", Path: "/vol1/music", TotalFiles: 50, TotalSongs: 40},
		{ID: 2, Name: "网盘音乐", Path: "openlist://192.168.1.10:5244/fnos/音乐", MediaType: "music",
			TotalFiles: 100, TotalSongs: 90, LastScanAt: scanned},
		{ID: 3, Name: "网盘有声书", Path: "openlist://192.168.1.10:5244/fnos/有声书", MediaType: "audiobook",
			TotalFiles: 20, LastScanAt: scanned.Add(-time.Hour)},
	})

	w := httptest.NewRecorder()
	h.gateways(w, adminRequest(t, "GET", "/api/cloudsource/gateways", ""))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	gws := decodeGatewayList(t, w)
	if len(gws) != 2 {
		t.Fatalf("expected 2 gateways, got %d", len(gws))
	}

	var nas, quark *cloudGatewayInfo
	for i := range gws {
		switch gws[i].Name {
		case "nas":
			nas = &gws[i]
		case "quark":
			quark = &gws[i]
		}
	}
	if nas == nil || quark == nil {
		t.Fatalf("both gateways must be listed, got %+v", gws)
	}
	if !nas.Enabled {
		t.Fatal("gateways must default to enabled")
	}
	if len(nas.Libraries) != 2 {
		t.Fatalf("nas must have 2 bound libraries, got %+v", nas.Libraries)
	}
	if nas.TotalFiles != 120 || nas.TotalSongs != 90 {
		t.Fatalf("nas aggregates wrong: files=%d songs=%d", nas.TotalFiles, nas.TotalSongs)
	}
	if nas.LastScanAt == nil || !nas.LastScanAt.Equal(scanned) {
		t.Fatalf("nas lastScanAt must be the most recent of its libraries, got %v", nas.LastScanAt)
	}
	if len(quark.Libraries) != 0 || quark.LastScanAt != nil {
		t.Fatalf("quark has no libraries, got %+v", quark)
	}

	// 红线: credentials must never leak into the response.
	body := w.Body.String()
	if strings.Contains(body, "secret") {
		t.Fatalf("response must not contain credentials: %s", body)
	}
}

func TestCloudGatewaysRequiresAdmin(t *testing.T) {
	h := gatewayFixture(t, nil)
	req := httptest.NewRequest("GET", "/api/cloudsource/gateways", nil)
	req = req.WithContext(request.WithUser(req.Context(), model.User{ID: "u-1", IsAdmin: false}))

	w := httptest.NewRecorder()
	h.gateways(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin must be rejected, got %d", w.Code)
	}
}

func TestCloudGatewayEnableToggleRoundTrip(t *testing.T) {
	h := gatewayFixture(t, model.Libraries{
		{ID: 2, Name: "网盘音乐", Path: "openlist://192.168.1.10:5244/fnos/音乐"},
	})

	// Disable "nas"
	w := httptest.NewRecorder()
	h.setGatewayEnabled(w, gatewayPathParam(adminRequest(t, "PUT", "/api/cloudsource/gateways/nas/enabled", `{"enabled":false}`), "nas"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	// The list must reflect it (and keep the other gateway untouched).
	w = httptest.NewRecorder()
	h.gateways(w, adminRequest(t, "GET", "/api/cloudsource/gateways", ""))
	for _, gw := range decodeGatewayList(t, w) {
		switch gw.Name {
		case "nas":
			if gw.Enabled {
				t.Fatal("nas must be disabled after the toggle")
			}
		case "quark":
			if !gw.Enabled {
				t.Fatal("quark must be unaffected by nas toggle")
			}
		}
	}

	// Re-enable
	w = httptest.NewRecorder()
	h.setGatewayEnabled(w, gatewayPathParam(adminRequest(t, "PUT", "/api/cloudsource/gateways/nas/enabled", `{"enabled":true}`), "nas"))
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.gateways(w, adminRequest(t, "GET", "/api/cloudsource/gateways", ""))
	for _, gw := range decodeGatewayList(t, w) {
		if gw.Name == "nas" && !gw.Enabled {
			t.Fatal("nas must be enabled again")
		}
	}
}

func TestCloudGatewayUnknownNameRejected(t *testing.T) {
	h := gatewayFixture(t, nil)

	w := httptest.NewRecorder()
	h.checkGateway(w, gatewayPathParam(adminRequest(t, "POST", "/api/cloudsource/gateways/ghost/check", ""), "ghost"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown gateway check must 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.setGatewayEnabled(w, gatewayPathParam(adminRequest(t, "PUT", "/api/cloudsource/gateways/ghost/enabled", `{"enabled":false}`), "ghost"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown gateway toggle must 404, got %d", w.Code)
	}
}

func TestCloudGatewayCheckReportsFailure(t *testing.T) {
	h := gatewayFixture(t, nil)

	// Point "nas" at a closed local port (instant connection refused) and probe it: the
	// panel must get a structured failure (ok=false + error), not a 500.
	conf.Server.OpenList["nas"] = conf.OpenListOptions{URL: "http://127.0.0.1:1", Token: "t"}
	w := httptest.NewRecorder()
	req := gatewayPathParam(adminRequest(t, "POST", "/api/cloudsource/gateways/nas/check", ""), "nas")
	h.checkGateway(w, req)
	if w.Code != 200 {
		t.Fatalf("probe failures are reported in the payload, expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			Source string `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if resp.Data.OK || resp.Data.Error == "" {
		t.Fatalf("unreachable gateway must yield ok=false with an error, got %+v", resp.Data)
	}
	if resp.Data.Source != "manual" {
		t.Fatalf("panel probes must be tagged source=manual, got %q", resp.Data.Source)
	}
}
