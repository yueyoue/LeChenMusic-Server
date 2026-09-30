package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/cloudsource"
)

// Regression tests for 评审 §4.6 P2-5: startup self-test reports gateway health without
// blocking or failing startup. The probe itself lives in core/cloudsource (shared with
// the P2-4 云源管理面板); these tests cover the cmd wiring + logging contract.

func fakeOpenList(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckOpenListGatewayOK(t *testing.T) {
	restore := conf.SnapshotConfig()
	defer restore()
	srv := fakeOpenList(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"","data":{"content":[{"name":"音乐","is_dir":true,"size":0}],"total":1}}`))
	})
	conf.Server.OpenList = map[string]conf.OpenListOptions{"nas": {URL: srv.URL, Token: "test-token"}}

	result, err := cloudsource.ProbeGateway(context.Background(), "nas", "startup")
	if err != nil {
		t.Fatalf("self-test must pass against a healthy gateway, got %v", err)
	}
	if !result.OK || result.RootEntries != 1 {
		t.Fatalf("unexpected probe result %+v", result)
	}
	// The panel reads the recorded startup check back.
	if last, ok := cloudsource.LastCheck("nas"); !ok || !last.OK || last.Source != "startup" {
		t.Fatalf("probe result must be recorded for the admin panel, got %+v (ok=%v)", last, ok)
	}
}

func TestCheckOpenListGatewayReportsFailure(t *testing.T) {
	restore := conf.SnapshotConfig()
	defer restore()
	srv := fakeOpenList(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"code":500,"message":"boom"}`, http.StatusInternalServerError)
	})
	conf.Server.OpenList = map[string]conf.OpenListOptions{"nas": {URL: srv.URL, Token: "test-token"}}

	result, err := cloudsource.ProbeGateway(context.Background(), "nas", "startup")
	if err == nil {
		t.Fatal("self-test must report an unreachable/broken gateway")
	}
	if result.OK {
		t.Fatal("failed probe must not be marked OK")
	}
	if last, ok := cloudsource.LastCheck("nas"); !ok || last.OK || last.Error == "" {
		t.Fatalf("failure must be recorded for the admin panel, got %+v (ok=%v)", last, ok)
	}
}

func TestStartOpenListCheckIsNoopWithoutGateways(t *testing.T) {
	restore := conf.SnapshotConfig()
	defer restore()
	conf.Server.OpenList = nil

	if err := startOpenListCheck(context.Background())(); err != nil {
		t.Fatalf("no gateways must be a clean no-op, got %v", err)
	}
}

func TestStartOpenListCheckNeverFails(t *testing.T) {
	restore := conf.SnapshotConfig()
	defer restore()
	srv := fakeOpenList(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	})
	conf.Server.OpenList = map[string]conf.OpenListOptions{
		"nas": {URL: srv.URL, Token: "t"},
	}

	// A broken gateway must be logged, not returned as a fatal startup error.
	if err := startOpenListCheck(context.Background())(); err != nil {
		t.Fatalf("self-test must never fail startup, got %v", err)
	}
}
