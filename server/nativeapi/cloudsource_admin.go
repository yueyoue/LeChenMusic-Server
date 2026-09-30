package nativeapi

// 评审 P2-4: 云源管理面板 (docs/网盘媒体源上线前评审与整改方案.md §4.6).
//
// Admin endpoints backing the "cloud sources" management panel:
//
//	GET    /api/cloudsource/gateways              — per-gateway status, file counts, last scan
//	POST   /api/cloudsource/gateways/{name}/check — live connectivity probe (login + root listing)
//	PUT    /api/cloudsource/gateways/{name}/enabled — enable/disable (启停), see semantics below
//
// Enable/disable semantics (启停): a disabled gateway is skipped by ALL scans (scheduled
// or manual); its libraries' existing rows are never touched while disabled. Playback of
// already-imported media is unaffected. The switch is persisted in the property store
// (consts.CloudSourceGatewayStatesKey) so it survives restarts.
//
// Red lines (design doc §15.1): credentials never leave the server (only names/URLs are
// reported), and the panel never reads file content — the probe is one login + one root
// directory listing.

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/cloudsource"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model/request"
)

// cloudGatewayLibrary is one library bound to a gateway, with the aggregate stats the
// panel shows ("文件数/最后扫描时间").
type cloudGatewayLibrary struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	MediaType         string    `json:"mediaType"`
	Path              string    `json:"path"`
	TotalFiles        int       `json:"totalFiles"`
	TotalSongs        int       `json:"totalSongs"`
	LastScanAt        time.Time `json:"lastScanAt"`
	LastScanStartedAt time.Time `json:"lastScanStartedAt"`
}

// cloudGatewayInfo is one configured OpenList gateway as shown by the panel.
type cloudGatewayInfo struct {
	Name       string                   `json:"name"`
	URL        string                   `json:"url"`
	Host       string                   `json:"host"`
	Enabled    bool                     `json:"enabled"`
	Check      *cloudsource.GatewayCheck `json:"check,omitempty"`
	Libraries  []cloudGatewayLibrary    `json:"libraries"`
	TotalFiles int                      `json:"totalFiles"`
	TotalSongs int                      `json:"totalSongs"`
	LastScanAt *time.Time               `json:"lastScanAt,omitempty"`
}

type cloudGatewayListResponse struct {
	Gateways []cloudGatewayInfo `json:"gateways"`
}

// gateways lists every configured gateway with its bound libraries and aggregates.
func (h *cloudSourceHandler) gateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !requireAdmin(w, r) {
		return
	}

	libs, err := h.ds.Library(ctx).GetAll()
	if err != nil {
		log.Error(ctx, "[cloud][openlist] cannot list libraries for gateway panel", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := cloudGatewayListResponse{Gateways: []cloudGatewayInfo{}}
	for _, name := range cloudsource.SortedGatewayNames() {
		opts := conf.Server.OpenList[name]
		gw := cloudGatewayInfo{
			Name:      name,
			URL:       opts.URL,
			Host:      hostOf(opts.URL),
			Enabled:   cloudsource.IsGatewayEnabled(ctx, h.ds, name),
			Libraries: []cloudGatewayLibrary{},
		}
		if check, ok := cloudsource.LastCheck(name); ok {
			gw.Check = &check
		}
		for _, lib := range libs {
			if libName, ok := cloudsource.LibraryGatewayName(lib.Path); !ok || libName != name {
				continue
			}
			gw.Libraries = append(gw.Libraries, cloudGatewayLibrary{
				ID:                lib.ID,
				Name:              lib.Name,
				MediaType:         lib.MediaType,
				Path:              lib.Path,
				TotalFiles:        lib.TotalFiles,
				TotalSongs:        lib.TotalSongs,
				LastScanAt:        lib.LastScanAt,
				LastScanStartedAt: lib.LastScanStartedAt,
			})
			gw.TotalFiles += lib.TotalFiles
			gw.TotalSongs += lib.TotalSongs
			if gw.LastScanAt == nil || lib.LastScanAt.After(*gw.LastScanAt) {
				t := lib.LastScanAt
				gw.LastScanAt = &t
			}
		}
		resp.Gateways = append(resp.Gateways, gw)
	}
	// Envelope with a `data` key: the UI (and the tests) read res.json.data.gateways.
	writeCloudSourceJSON(w, struct {
		Data cloudGatewayListResponse `json:"data"`
	}{Data: resp})
}

type gatewayCheckResponse struct {
	Data cloudsource.GatewayCheck `json:"data"`
}

// checkGateway runs a live connectivity probe against one gateway and records the
// result (shown on the panel until the next probe or restart).
func (h *cloudSourceHandler) checkGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !requireAdmin(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	if _, ok := conf.Server.OpenList[name]; !ok {
		http.Error(w, "unknown gateway", http.StatusNotFound)
		return
	}

	result, err := cloudsource.ProbeGateway(ctx, name, "manual")
	if err != nil {
		log.Warn(ctx, "[cloud][openlist] manual gateway check failed", "gateway", name, err)
	} else {
		log.Info(ctx, "[cloud][openlist] manual gateway check OK", "gateway", name, "rootEntries", result.RootEntries)
	}
	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(gatewayCheckResponse{Data: result}); encErr != nil {
		log.Error(ctx, "Error encoding gateway check response", encErr)
	}
}

type setGatewayEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type setGatewayEnabledResponse struct {
	Data struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"data"`
}

// setGatewayEnabled persists the 启停 switch for one gateway.
func (h *cloudSourceHandler) setGatewayEnabled(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !requireAdmin(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	if _, ok := conf.Server.OpenList[name]; !ok {
		http.Error(w, "unknown gateway", http.StatusNotFound)
		return
	}

	var req setGatewayEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := cloudsource.SetGatewayEnabled(ctx, h.ds, name, req.Enabled); err != nil {
		log.Error(ctx, "[cloud][openlist] cannot persist gateway state", "gateway", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	log.Info(ctx, "[cloud][openlist] gateway enable state changed",
		"gateway", name, "enabled", req.Enabled, "user", currentUserName(r))

	resp := setGatewayEnabledResponse{}
	resp.Data.Name = name
	resp.Data.Enabled = req.Enabled
	writeCloudSourceJSON(w, resp)
}

// requireAdmin gates the management endpoints to admin users.
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, ok := request.UserFrom(r.Context())
	if !ok || !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func currentUserName(r *http.Request) string {
	if user, ok := request.UserFrom(r.Context()); ok {
		return user.UserName
	}
	return ""
}

func writeCloudSourceJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Error(context.Background(), "Error encoding cloud source response", err)
	}
}
