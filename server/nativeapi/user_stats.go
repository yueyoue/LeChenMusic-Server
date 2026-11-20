package nativeapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

// [LeChenMusic-START:user-stats]

func (api *Router) addUserStatsRoute(r chi.Router) {
	h := &userStatsHandler{ds: api.ds}
	r.Route("/stats", func(r chi.Router) {
		// Any authenticated user can report
		r.Post("/play-log", h.reportPlayLog)
		r.Post("/device", h.reportDevice)

		// Any authenticated user can get their own stats
		r.Get("/me", h.getMyStats)

		// Admin-only queries
		r.Get("/users", h.listUserStats)
		r.Get("/users/{userID}/devices", h.listUserDevices)
		r.Get("/users/{userID}/play-logs", h.listUserPlayLogs)
		r.Get("/devices", h.listAllDevices)
		r.Get("/versions", h.listVersionStats)
	})
}

type userStatsHandler struct {
	ds model.DataStore
}

// ─── Request types ───────────────────────────────────────

type playLogRequest struct {
	ItemType   string `json:"itemType"`   // "music" or "audiobook"
	ItemID     string `json:"itemId"`
	ItemTitle  string `json:"itemTitle"`
	ItemArtist string `json:"itemArtist"`
	Duration   int    `json:"duration"` // seconds
}

type deviceReportRequest struct {
	DeviceID   string `json:"deviceId"`
	DeviceInfo string `json:"deviceInfo"`
	AppVersion string `json:"appVersion"`
}

// ─── Helpers ─────────────────────────────────────────────

func statsWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func isAdmin(r *http.Request) bool {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		return false
	}
	return usr.IsAdmin
}

// ─── Handlers ────────────────────────────────────────────

// POST /api/stats/play-log
func (h *userStatsHandler) reportPlayLog(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req playLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ItemType == "" || req.ItemID == "" {
		http.Error(w, "itemType and itemId are required", http.StatusBadRequest)
		return
	}
	if req.ItemType != "music" && req.ItemType != "audiobook" {
		http.Error(w, "itemType must be 'music' or 'audiobook'", http.StatusBadRequest)
		return
	}
	if req.Duration < 0 {
		req.Duration = 0
	}

	entry := &model.PlayLog{
		UserID:     usr.ID,
		ItemType:   req.ItemType,
		ItemID:     req.ItemID,
		ItemTitle:  req.ItemTitle,
		ItemArtist: req.ItemArtist,
		Duration:   req.Duration,
	}

	if err := h.ds.PlayLog(r.Context()).Add(entry); err != nil {
		log.Error(r.Context(), "Failed to save play log", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	statsWriteJSON(w, map[string]any{"status": "ok", "data": entry})
}

// POST /api/stats/device
func (h *userStatsHandler) reportDevice(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req deviceReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.DeviceID == "" || req.AppVersion == "" {
		http.Error(w, "deviceId and appVersion are required", http.StatusBadRequest)
		return
	}

	device := &model.UserDevice{
		UserID:     usr.ID,
		DeviceID:   req.DeviceID,
		DeviceInfo: req.DeviceInfo,
		AppVersion: req.AppVersion,
	}

	if err := h.ds.UserDevice(r.Context()).Upsert(device); err != nil {
		log.Error(r.Context(), "Failed to save device info", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	statsWriteJSON(w, map[string]any{"status": "ok"})
}

// GET /api/stats/me
func (h *userStatsHandler) getMyStats(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	stats, err := h.ds.PlayLog(r.Context()).GetUserStats(usr.ID)
	if err != nil {
		log.Error(r.Context(), "Failed to get user stats", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": stats})
}

// GET /api/stats/users  (admin only)
func (h *userStatsHandler) listUserStats(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	stats, err := h.ds.PlayLog(r.Context()).GetAllUsersStats()
	if err != nil {
		log.Error(r.Context(), "Failed to get user stats", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": stats})
}

// GET /api/stats/devices  (admin only) — all devices for all users, most recently seen first.
// The admin user list uses this to show each user's current app version in one request.
func (h *userStatsHandler) listAllDevices(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	devices, err := h.ds.UserDevice(r.Context()).GetAll()
	if err != nil {
		log.Error(r.Context(), "Failed to get devices", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": devices})
}

// GET /api/stats/users/{userID}/devices  (admin only)
func (h *userStatsHandler) listUserDevices(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "userID")
	devices, err := h.ds.UserDevice(r.Context()).GetByUser(userID)
	if err != nil {
		log.Error(r.Context(), "Failed to get user devices", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": devices})
}

// GET /api/stats/users/{userID}/play-logs?limit=N  (admin only)
func (h *userStatsHandler) listUserPlayLogs(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "userID")
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	logs, err := h.ds.PlayLog(r.Context()).GetByUser(userID, limit)
	if err != nil {
		log.Error(r.Context(), "Failed to get user play logs", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": logs})
}

// GET /api/stats/versions  (admin only)
func (h *userStatsHandler) listVersionStats(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}
	stats, err := h.ds.UserDevice(r.Context()).VersionStats()
	if err != nil {
		log.Error(r.Context(), "Failed to get version stats", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	statsWriteJSON(w, map[string]any{"data": stats})
}

// [LeChenMusic-END:user-stats]
