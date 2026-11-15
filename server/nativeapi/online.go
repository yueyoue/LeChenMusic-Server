package nativeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/scanner"
	onlinesvc "github.com/navidrome/navidrome/server/online"
)

// onlineHandler serves the "search & download missing songs" API under /api/online.
type onlineHandler struct {
	ds        model.DataStore
	playlists playlistsvc.Playlists
	manager   *onlinesvc.Manager
	ingest    *onlinesvc.Ingestor
}

func (api *Router) addOnlineRoute(r chi.Router) {
	h := newOnlineHandler(api.ds, api.playlists)
	r.Route("/online", func(r chi.Router) {
		r.Get("/search", h.search)
		r.Post("/download", h.download)
		r.Get("/tasks", h.listTasks)
		r.Post("/tasks/{id}/retry", h.retryTask)
		r.Delete("/tasks/{id}", h.deleteTask)
		h.adminRoutes(r) // M5: /downloads*, /logs* (admin-only)
	})
}

func newOnlineHandler(ds model.DataStore, pls playlistsvc.Playlists) *onlineHandler {
	ctx := context.Background()

	libPath := conf.Server.OnlineDownload.LibPath
	if libPath == "" {
		libPath = conf.Server.MusicFolder
	}
	matcher := newLibraryMatcher(ds)
	ingest := onlinesvc.NewIngestor(libPath, onlinesvc.GetDefaultSource(), matcher)
	ingest.OnIngested(func(ctx context.Context, _ string) {
		if scanner.IsScanning() {
			return
		}
		go func() {
			if _, err := scanner.CallScan(context.Background(), ds, pls, false, nil); err != nil {
				log.Warn(ctx, "online: could not trigger library scan", err)
			}
		}()
	})

	manager := onlinesvc.NewManager(ds.DownloadTask(ctx), filepath.Join(conf.Server.DataFolder.String(), "downloads", "tmp"), ingest.Process)
	manager.TaskJitter = conf.Server.OnlineDownload.TaskJitter
	if ds.DownloadTask(ctx) != nil {
		manager.OnSettled(func(ctx context.Context, task *model.DownloadTask, err error) {
			action := model.OpLogDownloadDone
			success := true
			detail := ""
			if err != nil {
				action = model.OpLogDownloadFailed
				success = false
				detail = err.Error()
			}
			writeOnlineOpLog(ctx, ds, task.UserID, action, task.SongName, task.Artist, detail, success)
		})
		manager.Start(ctx)
	} else {
		log.Warn(ctx, "online: download task repository unavailable, download worker not started")
	}

	return &onlineHandler{ds: ds, playlists: pls, manager: manager, ingest: ingest}
}

// newLibraryMatcher matches songs against the library using a SQL pre-filter
// plus normalized comparison (fuzzy: ignores case/width/whitespace/bracket notes).
func newLibraryMatcher(ds model.DataStore) onlinesvc.MatcherFunc {
	return func(ctx context.Context, title, artist string) (bool, error) {
		title = strings.TrimSpace(title)
		if title == "" {
			return false, nil
		}
		filters := squirrel.And{squirrel.Like{"title": "%" + title + "%"}}
		if a := strings.TrimSpace(artist); a != "" {
			filters = append(filters, squirrel.Like{"artist": "%" + a + "%"})
		}
		mfs, err := ds.MediaFile(ctx).GetAll(model.QueryOptions{Filters: filters, Max: 100})
		if err != nil {
			return false, err
		}
		want := onlinesvc.NormalizeSongKey(onlinesvc.CleanSongTitle(title), onlinesvc.NormalizeArtist(artist))
		for _, mf := range mfs {
			if onlinesvc.NormalizeSongKey(onlinesvc.CleanSongTitle(mf.Title), onlinesvc.NormalizeArtist(mf.Artist)) == want {
				return true, nil
			}
		}
		return false, nil
	}
}

// ---- request/response shapes ----

type onlineSong struct {
	onlinesvc.Song
	InLibrary bool `json:"inLibrary"`
}

type onlineSearchResponse struct {
	Songs []onlineSong `json:"songs"`
	Total int          `json:"total"`
}

type onlineDownloadRequest struct {
	Source   string `json:"source"`
	Platform string `json:"platform"`
	SongID   string `json:"songId"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverURL string `json:"coverUrl"`
	Quality  string `json:"quality"` // e.g. "flac/2000"; empty = default
	Fuzzy    bool   `json:"fuzzy"`   // allow downloading other versions even if a match exists
	Time     int64  `json:"time"`
	Sign     string `json:"sign"`
}

// ---- handlers ----

// search: GET /api/online/search?keyword=&page=&size=
func (h *onlineHandler) search(w http.ResponseWriter, r *http.Request) {
	if !conf.Server.OnlineDownload.Enabled {
		http.Error(w, "Online download is disabled", http.StatusForbidden)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if keyword == "" {
		http.Error(w, "keyword is required", http.StatusBadRequest)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 50 {
		size = 20
	}

	src := h.sourceByName(r.URL.Query().Get("source"))
	if src == nil {
		http.Error(w, "unknown download source", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	result, err := src.SearchSong(ctx, onlinesvc.SearchRequest{Keyword: keyword, Page: page, PageSize: size})
	if err != nil {
		log.Warn(ctx, "online: search failed", "keyword", keyword, err)
		http.Error(w, "search failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	resp := onlineSearchResponse{Songs: make([]onlineSong, 0, len(result.Songs)), Total: result.Total}
	for _, song := range result.Songs {
		inLibrary, err := h.ingest.Exists(ctx, song.Name, song.Artist)
		if err != nil {
			log.Warn(ctx, "online: dedup check failed", "song", song.Name, err)
		}
		resp.Songs = append(resp.Songs, onlineSong{Song: song, InLibrary: inLibrary})
	}
	writeJSON(w, resp)
}

// download: POST /api/online/download
func (h *onlineHandler) download(w http.ResponseWriter, r *http.Request) {
	if !conf.Server.OnlineDownload.Enabled {
		http.Error(w, "Online download is disabled", http.StatusForbidden)
		return
	}
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req onlineDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.SongID == "" || req.Name == "" {
		http.Error(w, "songId and name are required", http.StatusBadRequest)
		return
	}
	// Store clean metadata: short title (no media-annotation suffixes) and
	// artists split/rejoined with the library separator.
	req.Name = onlinesvc.CleanSongTitle(req.Name)
	req.Artist = onlinesvc.NormalizeArtist(req.Artist)
	if req.Source == "" {
		req.Source = "hicn"
	}

	if err := h.checkDownloadPermission(ctx, user, w); err != nil {
		return // response already written
	}

	if !req.Fuzzy {
		exists, err := h.ingest.Exists(ctx, req.Name, req.Artist)
		if err != nil {
			http.Error(w, "dedup check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if exists {
			http.Error(w, "song already exists in library (enable fuzzy matching to download anyway)", http.StatusConflict)
			return
		}
	}

	quality := req.Quality
	if quality == "" {
		quality = conf.Server.OnlineDownload.Quality
	}
	if h.ds.DownloadTask(ctx) == nil {
		http.Error(w, "download task storage unavailable", http.StatusServiceUnavailable)
		return
	}

	task := &model.DownloadTask{
		UserID:           user.ID,
		Source:           req.Source,
		Platform:         req.Platform,
		SongID:           req.SongID,
		SongName:         req.Name,
		Artist:           req.Artist,
		AlbumName:        req.Album,
		CoverURL:         req.CoverURL,
		RequestedQuality: quality,
		UpstreamTime:     req.Time,
		Sign:             req.Sign,
	}
	if err := h.manager.Enqueue(task); err != nil {
		log.Error(ctx, "online: could not enqueue download", "song", req.Name, err)
		http.Error(w, "could not create download task", http.StatusInternalServerError)
		return
	}
	log.Info(ctx, "Online download queued", "user", user.UserName, "song", req.Name, "artist", req.Artist)
	writeOnlineOpLog(ctx, h.ds, user.ID, model.OpLogDownloadQueued, req.Name, req.Artist, "", true)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, task)
}

// writeOnlineOpLog records one online-download operation (best-effort).
func writeOnlineOpLog(ctx context.Context, ds model.DataStore, userID, action, song, artist, detail string, success bool) {
	repo := ds.OnlineOpLog(ctx)
	if repo == nil {
		return
	}
	userName := userID
	if u, err := ds.User(ctx).Get(userID); err == nil && u != nil {
		userName = u.UserName
	}
	entry := &model.OnlineOpLog{
		UserID:   userID,
		UserName: userName,
		Action:   action,
		SongName: song,
		Artist:   artist,
		Detail:   detail,
		Success:  success,
	}
	if err := repo.Add(entry); err != nil {
		log.Warn(ctx, "online: could not write op log", "action", action, err)
	}
}

// listTasks: GET /api/online/tasks?mine=1
func (h *onlineHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tasks, err := h.ds.DownloadTask(ctx).GetAll()
	if err != nil {
		http.Error(w, "could not load tasks", http.StatusInternalServerError)
		return
	}
	mine := r.URL.Query().Get("mine") == "1" || !user.IsAdmin
	filtered := make(model.DownloadTasks, 0, len(tasks))
	for _, t := range tasks {
		if mine && t.UserID != user.ID {
			continue
		}
		filtered = append(filtered, t)
	}
	writeJSON(w, filtered)
}

// retryTask: POST /api/online/tasks/{id}/retry
func (h *onlineHandler) retryTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task, ok := h.ownedTask(w, r)
	if !ok {
		return
	}
	if err := h.manager.Retry(ctx, task.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"status": "re-queued"})
}

// deleteTask: DELETE /api/online/tasks/{id}
func (h *onlineHandler) deleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task, ok := h.ownedTask(w, r)
	if !ok {
		return
	}
	switch task.Status {
	case model.DownloadStatusQueued, model.DownloadStatusFailed, model.DownloadStatusDone:
	default:
		http.Error(w, "task is still running", http.StatusConflict)
		return
	}
	if err := h.ds.DownloadTask(ctx).Delete(task.ID); err != nil {
		http.Error(w, "could not delete task", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// ---- helpers ----

func (h *onlineHandler) sourceByName(name string) onlinesvc.Source {
	if name == "" {
		name = "hicn"
	}
	src, ok := onlinesvc.Get(name)
	if !ok {
		return nil
	}
	return src
}

// checkDownloadPermission enforces the configured permission mode and daily
// quota. It writes an error response and returns non-nil when access is denied.
func (h *onlineHandler) checkDownloadPermission(ctx context.Context, user model.User, w http.ResponseWriter) error {
	switch conf.Server.OnlineDownload.Permission {
	case "open":
		return nil
	case "quota":
		quota := conf.Server.OnlineDownload.DailyQuota
		if quota > 0 {
			count, err := h.countTodaysDownloads(ctx, user.ID)
			if err != nil {
				http.Error(w, "quota check failed", http.StatusInternalServerError)
				return err
			}
			if count >= quota {
				http.Error(w, "daily download quota reached", http.StatusTooManyRequests)
				return errDailyQuota
			}
		}
		return nil
	default: // "admin-or-approved"
		if !user.IsAdmin {
			http.Error(w, "only administrators can download (permission mode: admin-or-approved)", http.StatusForbidden)
			return errNotAdmin
		}
		return nil
	}
}

var (
	errNotAdmin   = errStr("admin required")
	errDailyQuota = errStr("daily quota reached")
)

type errStr string

func (e errStr) Error() string { return string(e) }

func (h *onlineHandler) countTodaysDownloads(ctx context.Context, userID string) (int, error) {
	tasks, err := h.ds.DownloadTask(ctx).GetAll()
	if err != nil {
		return 0, err
	}
	now := time.Now()
	count := 0
	for _, t := range tasks {
		if t.UserID != userID || t.Status == model.DownloadStatusFailed {
			continue
		}
		if t.CreatedAt.Year() == now.Year() && t.CreatedAt.YearDay() == now.YearDay() {
			count++
		}
	}
	return count, nil
}

// ownedTask loads the task from the URL and enforces owner-or-admin access.
func (h *onlineHandler) ownedTask(w http.ResponseWriter, r *http.Request) (*model.DownloadTask, bool) {
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	taskID := chi.URLParam(r, "id")
	task, err := h.ds.DownloadTask(ctx).Get(taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return nil, false
	}
	if task.UserID != user.ID && !user.IsAdmin {
		http.Error(w, "access denied", http.StatusForbidden)
		return nil, false
	}
	return task, true
}
