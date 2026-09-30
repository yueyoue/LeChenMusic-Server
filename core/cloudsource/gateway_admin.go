package cloudsource

// 评审 P2-4: 云源管理面板 backend support (docs/网盘媒体源上线前评审与整改方案.md §4.6).
//
// This file backs the admin "cloud source" panel: per-gateway connectivity status,
// enable/disable (启停), and the library/file-count aggregates the panel shows.
//
// Enable/disable semantics (启停): a disabled gateway is skipped by ALL scans (scheduled
// or manual, music and audiobook alike) — its libraries are neither listed nor imported
// and, crucially, their existing rows are never touched by the missing/purge phases.
// Already-imported media stays playable (non-destructive by design); the switch only
// stops scan traffic to the gateway (e.g. while a drive is rate-limited or broken).
//
// Red lines honoured here (design doc §15.1): no file content is ever read by the
// panel. The connectivity probe is one login + one root directory listing at most.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/adapters/openlist"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// GatewayCheck is the outcome of one connectivity probe against an OpenList gateway.
type GatewayCheck struct {
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	OK          bool      `json:"ok"`
	Error       string    `json:"error,omitempty"`
	RootEntries int       `json:"rootEntries"`
	CheckedAt   time.Time `json:"checkedAt"`
	Source      string    `json:"source"` // "startup" | "manual"
}

var checkRegistry = struct {
	sync.RWMutex
	results map[string]GatewayCheck
}{results: map[string]GatewayCheck{}}

func recordCheck(c GatewayCheck) {
	checkRegistry.Lock()
	defer checkRegistry.Unlock()
	checkRegistry.results[c.Name] = c
}

// LastCheck returns the most recent probe result for a gateway, if any.
func LastCheck(name string) (GatewayCheck, bool) {
	checkRegistry.RLock()
	defer checkRegistry.RUnlock()
	c, ok := checkRegistry.results[name]
	return c, ok
}

// probeTimeout bounds how long a flaky gateway can hold a probe open.
const probeTimeout = 15 * time.Second

// ProbeGateway runs one login + root listing against a gateway and records the outcome
// so the admin panel can show it (in addition to the startup self-test log line).
// Traffic-wise this is deliberately minimal: no retries beyond one, no file reads.
func ProbeGateway(ctx context.Context, name, source string) (GatewayCheck, error) {
	opts, ok := conf.Server.OpenList[name]
	if !ok {
		return GatewayCheck{}, fmt.Errorf("cloudsource: unknown OpenList gateway %q", name)
	}

	client := openlist.NewClient(openlist.Config{
		BaseURL:  opts.URL,
		Username: opts.Username,
		Password: opts.Password,
		Token:    opts.Token,
		// A probe is one request, not a scan: fail fast and without extra traffic (风控).
		MaxRetries:       1,
		FailureThreshold: 1,
	})

	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	result := GatewayCheck{
		Name:      name,
		URL:       opts.URL,
		CheckedAt: time.Now(),
		Source:    source,
	}

	entries, err := client.List(cctx, "/", 1)
	if err != nil {
		result.Error = err.Error()
		recordCheck(result)
		return result, err
	}
	result.OK = true
	result.RootEntries = len(entries)
	recordCheck(result)
	return result, nil
}

// LibraryGatewayName resolves which configured gateway a library path belongs to.
// Only `openlist://...` paths match; local libraries return ok=false.
func LibraryGatewayName(libraryPath string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(libraryPath))
	if err != nil || u.Scheme != SchemaID || u.Host == "" {
		return "", false
	}
	return GatewayName(u.Host)
}

// GatewayName resolves the conf.Server.OpenList entry that a `openlist://<hostport>/...`
// URI refers to, using the same matching rules as EndpointFor (see matchGateway).
func GatewayName(hostport string) (string, bool) {
	_, name, err := matchGateway(hostport)
	if err != nil {
		return "", false
	}
	return name, true
}

// loadGatewayStates reads the persisted enable/disable flags. Entries default to
// enabled: only an explicit `false` disables a gateway.
func loadGatewayStates(ctx context.Context, ds model.DataStore) map[string]bool {
	raw, err := ds.Property(ctx).DefaultGet(consts.CloudSourceGatewayStatesKey, "")
	if err != nil || raw == "" {
		return map[string]bool{}
	}
	var states map[string]bool
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		log.Warn(ctx, "[cloud][openlist] invalid gateway states in property store, treating all as enabled", err)
		return map[string]bool{}
	}
	return states
}

// IsGatewayEnabled reports whether scans may touch the given gateway.
func IsGatewayEnabled(ctx context.Context, ds model.DataStore, name string) bool {
	enabled, explicitlySet := loadGatewayStates(ctx, ds)[name]
	if !explicitlySet {
		return true
	}
	return enabled
}

// SetGatewayEnabled persists the enable/disable flag for one gateway.
func SetGatewayEnabled(ctx context.Context, ds model.DataStore, name string, enabled bool) error {
	states := loadGatewayStates(ctx, ds)
	if _, known := conf.Server.OpenList[name]; !known {
		return fmt.Errorf("cloudsource: unknown OpenList gateway %q", name)
	}
	states[name] = enabled
	raw, err := json.Marshal(states)
	if err != nil {
		return err
	}
	return ds.Property(ctx).Put(consts.CloudSourceGatewayStatesKey, string(raw))
}

// SkipDisabledGateways filters out libraries that live on a gateway currently disabled
// from the admin panel. Local libraries are always kept — the switch never affects them
// (红线: 本地库行为零变化).
func SkipDisabledGateways(ctx context.Context, ds model.DataStore, libs model.Libraries) model.Libraries {
	states := loadGatewayStates(ctx, ds)
	if len(states) == 0 {
		return libs
	}
	kept := make(model.Libraries, 0, len(libs))
	for _, lib := range libs {
		if name, ok := LibraryGatewayName(lib.Path); ok {
			if explicitlySet, present := states[name]; present && !explicitlySet {
				log.Warn(ctx, "[cloud][openlist] gateway disabled, skipping library in scan",
					"gateway", name, "library", lib.Name, "libraryID", lib.ID)
				continue
			}
		}
		kept = append(kept, lib)
	}
	return kept
}

// SortedGatewayNames returns the configured gateway names in stable order.
func SortedGatewayNames() []string {
	cfg := conf.Server.OpenList
	names := make([]string, 0, len(cfg))
	for name := range cfg {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
