package cloudsource

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// Regression tests for 评审 P2-4 (云源管理面板): gateway resolution, the 启停 switch and
// the "skip disabled gateways during scans" behaviour.

func adminDS(t *testing.T) *tests.MockDataStore {
	t.Helper()
	// NOTE: deliberately no tests.Init here — it os.Chdir()s to the repo root, which
	// would break the relative fixture paths of the storage conformance tests in this
	// package. Everything used here works without it.
	return &tests.MockDataStore{MockedProperty: &tests.MockedPropertyRepo{}}
}

func withGateways(t *testing.T, cfg map[string]conf.OpenListOptions) {
	t.Helper()
	restore := conf.SnapshotConfig()
	t.Cleanup(restore)
	conf.Server.OpenList = cfg
	resetEndpoints()
}

func TestGatewayNameMatching(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{
		"nas": {URL: "http://192.168.1.10:5244", Token: "t"},
		"quark": {URL: "http://pan.example.com:5244", Token: "t"},
	})

	// The library URI host equals the configured URL's host:port.
	if name, ok := LibraryGatewayName("openlist://192.168.1.10:5244/fnos/音乐"); !ok || name != "nas" {
		t.Fatalf("expected nas, got %q (ok=%v)", name, ok)
	}
	// Same via the map key being the host:port itself.
	withGateways(t, map[string]conf.OpenListOptions{
		"192.168.1.10:5244": {URL: "http://192.168.1.10:5244", Token: "t"},
	})
	if name, ok := GatewayName("192.168.1.10:5244"); !ok || name != "192.168.1.10:5244" {
		t.Fatalf("expected key match, got %q (ok=%v)", name, ok)
	}
	// Local libraries never match a gateway.
	if name, ok := LibraryGatewayName("/vol1/music"); ok {
		t.Fatalf("local path must not match a gateway, got %q", name)
	}
	if name, ok := LibraryGatewayName("fake:///music"); ok {
		t.Fatalf("non-openlist scheme must not match a gateway, got %q", name)
	}
}

func TestGatewayNameUnknownMultiGateway(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{
		"nas":   {URL: "http://192.168.1.10:5244", Token: "t"},
		"quark": {URL: "http://pan.example.com:5244", Token: "t"},
	})
	// Two gateways configured and the host matches neither: no silent guessing.
	if _, ok := LibraryGatewayName("openlist://10.0.0.1:5244/music"); ok {
		t.Fatal("unknown host must not resolve to a gateway when several are configured")
	}
}

func TestGatewayNameSingleGatewayFallback(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{
		"nas": {URL: "http://192.168.1.10:5244", Token: "t"},
	})
	// Only one gateway configured: anything openlist:// resolves to it (same rule as EndpointFor).
	if name, ok := LibraryGatewayName("openlist://10.0.0.1:5244/music"); !ok || name != "nas" {
		t.Fatalf("expected single-gateway fallback to nas, got %q (ok=%v)", name, ok)
	}
}

func TestGatewayEnabledDefaultsToEnabled(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{"nas": {URL: "http://192.168.1.10:5244", Token: "t"}})
	ds := adminDS(t)
	ctx := context.Background()

	if !IsGatewayEnabled(ctx, ds, "nas") {
		t.Fatal("a gateway with no stored state must default to enabled")
	}
}

func TestSetGatewayEnabledRoundTrip(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{"nas": {URL: "http://192.168.1.10:5244", Token: "t"}})
	ds := adminDS(t)
	ctx := context.Background()

	if err := SetGatewayEnabled(ctx, ds, "nas", false); err != nil {
		t.Fatalf("disabling must persist: %v", err)
	}
	if IsGatewayEnabled(ctx, ds, "nas") {
		t.Fatal("gateway must be disabled after SetGatewayEnabled(false)")
	}
	if err := SetGatewayEnabled(ctx, ds, "nas", true); err != nil {
		t.Fatalf("re-enabling must persist: %v", err)
	}
	if !IsGatewayEnabled(ctx, ds, "nas") {
		t.Fatal("gateway must be enabled again")
	}
}

func TestSetGatewayEnabledRejectsUnknownName(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{"nas": {URL: "http://192.168.1.10:5244", Token: "t"}})
	ds := adminDS(t)
	if err := SetGatewayEnabled(context.Background(), ds, "ghost", false); err == nil {
		t.Fatal("unknown gateway names must be rejected")
	}
}

func TestSkipDisabledGateways(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{
		"nas": {URL: "http://192.168.1.10:5244", Token: "t"},
		"quark": {URL: "http://pan.example.com:5244", Token: "t"},
	})
	ds := adminDS(t)
	ctx := context.Background()

	if err := SetGatewayEnabled(ctx, ds, "nas", false); err != nil {
		t.Fatal(err)
	}

	libs := model.Libraries{
		{ID: 1, Name: "本地音乐", Path: "/vol1/music"},
		{ID: 2, Name: "网盘音乐", Path: "openlist://192.168.1.10:5244/fnos/音乐"},
		{ID: 3, Name: "夸克书库", Path: "openlist://pan.example.com:5244/books"},
	}

	kept := SkipDisabledGateways(ctx, ds, libs)
	if len(kept) != 2 {
		t.Fatalf("expected 2 libraries kept, got %d: %+v", len(kept), kept)
	}
	if kept[0].ID != 1 || kept[1].ID != 3 {
		t.Fatalf("local + enabled-gateway libraries must be kept (in order), got IDs %d, %d", kept[0].ID, kept[1].ID)
	}
}

func TestSkipDisabledGatewaysNoopWhenAllEnabled(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{"nas": {URL: "http://192.168.1.10:5244", Token: "t"}})
	ds := adminDS(t)
	ctx := context.Background()

	if err := SetGatewayEnabled(ctx, ds, "nas", true); err != nil {
		t.Fatal(err)
	}
	libs := model.Libraries{
		{ID: 1, Name: "本地", Path: "/vol1/music"},
		{ID: 2, Name: "网盘", Path: "openlist://192.168.1.10:5244/music"},
	}
	kept := SkipDisabledGateways(ctx, ds, libs)
	if len(kept) != 2 {
		t.Fatalf("enabled gateways must not be skipped, got %d", len(kept))
	}
}

func TestProbeGatewayUnknownName(t *testing.T) {
	withGateways(t, map[string]conf.OpenListOptions{"nas": {URL: "http://192.168.1.10:5244", Token: "t"}})
	if _, err := ProbeGateway(context.Background(), "ghost", "manual"); err == nil {
		t.Fatal("probing an unknown gateway must fail")
	}
	if _, ok := LastCheck("ghost"); ok {
		t.Fatal("unknown gateway must not record a check result")
	}
}
