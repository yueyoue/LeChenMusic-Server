package nativeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	onlinesvc "github.com/navidrome/navidrome/server/online"
)

// 请求方标识：网页管理端在 httpClient 里统一带上该头，APP 不带。
// 上游被拦（口令页/风控页/非 JSON）时，只有管理端拿到可操作的中文提示，
// APP 普通用户只看到一句中性失败信息。
const (
	onlineClientHeader     = "X-Online-Client"
	onlineClientWebAdmin   = "web-admin"
	onlineUpstreamGeneric  = onlinesvc.UpstreamBlockedNeutralText
	onlineUpstreamErrorKey = "UPSTREAM_BLOCKED"
)

// writeSourceError renders an upstream failure, keeping the operator-facing
// hint away from ordinary APP users.
func writeSourceError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	kind, blocked := onlinesvc.IsUpstreamBlocked(err)
	if !blocked {
		http.Error(w, fallback+err.Error(), http.StatusBadGateway)
		return
	}
	message := onlineUpstreamGeneric
	if r.Header.Get(onlineClientHeader) == onlineClientWebAdmin {
		message = onlinesvc.UpstreamBlockedHint(kind)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  onlineUpstreamErrorKey,
		"kind":  kind,
	})
}

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
		r.Post("/preview", h.preview)
		r.Post("/download", h.download)
		r.Get("/tasks", h.listTasks)
		r.Post("/tasks/{id}/retry", h.retryTask)
		r.Delete("/tasks/{id}", h.deleteTask)
		r.Get("/settings", h.getSettings)
		r.With(adminOnlyMiddleware).Put("/settings", h.updateSettings)
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
	ingest := onlinesvc.NewIngestor(libPath, onlinesvc.GetDefaultSource(), matcher).
		WithAlbumInfo(onlinesvc.NewKuwoAlbumInfo(""))
	ingest.OnIngested(func(ctx context.Context, _ string) {
		triggerLibraryScanWhenIdle(ctx, ds, pls)
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
				// 操作日志只对管理员可见（adminOnlyMiddleware），因此这里放运维视角的
				// 说明（含“需要换源”的判断），任务行本身仍是用户可见的中性文案。
				if kind, blocked := onlinesvc.IsUpstreamBlocked(err); blocked {
					detail = onlinesvc.UpstreamBlockedHint(kind)
				}
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
		// Title-only pre-filter: the artist credit differs between sources and
		// the library ("苏永康&彭佳丽" vs "苏永康 • 彭佳丽"), so an artist LIKE
		// here would silently miss existing songs. Artist matching happens in Go.
		clean := onlinesvc.CleanSongTitle(title)
		filters := squirrel.Or{squirrel.Like{"title": "%" + title + "%"}}
		if clean != title {
			filters = append(filters, squirrel.Like{"title": "%" + clean + "%"})
		}
		mfs, err := ds.MediaFile(ctx).GetAll(model.QueryOptions{Filters: filters, Max: 200})
		if err != nil {
			return false, err
		}
		wantTitle := onlinesvc.NormalizeSongKey(clean, "")
		for _, mf := range mfs {
			if onlinesvc.NormalizeSongKey(onlinesvc.CleanSongTitle(mf.Title), "") != wantTitle {
				continue
			}
			// Same title: any shared artist counts as the same song
			// (a search for "苏永康" must match the library's "苏永康 • 彭佳丽").
			if onlinesvc.ArtistsMatch(artist, mf.Artist) {
				return true, nil
			}
		}
		return false, nil
	}
}

// ---- request/response shapes ----

type onlineSong struct {
	onlinesvc.Song
	InLibrary   bool `json:"inLibrary"`
	Downloading bool `json:"downloading"` // a download task is queued/running for this song
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
	AlbumID  string `json:"albumId"` // upstream album id: album artist + release year
	CoverURL string `json:"coverUrl"`
	Duration int    `json:"duration"` // expected song length (seconds) from the search result
	Quality  string `json:"quality"`  // e.g. "flac/2000"; empty = default
	Fuzzy    bool   `json:"fuzzy"`    // allow downloading other versions even if a match exists
	Time     int64  `json:"time"`
	Sign     string `json:"sign"`
}

// ---- download dedup ----

// Download states used by the dedup gate.
const (
	downloadStateActive = "active" // queued/resolving/downloading/tagging
	downloadStateDone   = "done"   // finished and the audio file is on disk
)

// downloadGateMu serializes the "check for duplicates, then enqueue" sequence in
// the download handler. Without it two concurrent clicks could both pass the
// duplicate check before either task exists.
var downloadGateMu sync.Mutex

// isActiveDownload reports whether the task is still in flight.
func isActiveDownload(status string) bool {
	switch status {
	case model.DownloadStatusQueued, model.DownloadStatusResolving,
		model.DownloadStatusDownloading, model.DownloadStatusTagging:
		return true
	}
	return false
}

// downloadIndex answers "is this song already downloaded or being downloaded?"
// from the persistent task table. Using the task table (in addition to the
// library) closes the gap where a freshly downloaded file exists on disk but the
// library scan has not indexed it yet: before, that window allowed the same song
// to be downloaded again by simply clicking the button twice.
type downloadIndex struct {
	activeBySong map[string]bool // "source|songId"
	doneBySong   map[string]bool
	activeByKey  map[string]bool // normalized "title|artist"
	doneByKey    map[string]bool
}

func buildDownloadIndex(tasks model.DownloadTasks) downloadIndex {
	idx := downloadIndex{
		activeBySong: map[string]bool{},
		doneBySong:   map[string]bool{},
		activeByKey:  map[string]bool{},
		doneByKey:    map[string]bool{},
	}
	for i := range tasks {
		t := tasks[i]
		active := isActiveDownload(t.Status)
		// A finished task only counts while its file still exists: once the song
		// is deleted ("不在库") it must be downloadable again.
		done := t.Status == model.DownloadStatusDone && fileExists(t.FilePath)
		if !active && !done {
			continue
		}
		if t.SongID != "" {
			songKey := t.Source + "|" + t.SongID
			if active {
				idx.activeBySong[songKey] = true
			} else {
				idx.doneBySong[songKey] = true
			}
		}
		if key := onlinesvc.NormalizeSongKey(t.SongName, t.Artist); key != "|" {
			if active {
				idx.activeByKey[key] = true
			} else {
				idx.doneByKey[key] = true
			}
		}
	}
	return idx
}

// state returns the download state for one upstream song, matched both by its
// upstream identity (source + song id) and by its normalized title/artist.
func (idx downloadIndex) state(source, songID, name, artist string) (songState, keyState string) {
	if songID != "" {
		songKey := source + "|" + songID
		switch {
		case idx.activeBySong[songKey]:
			songState = downloadStateActive
		case idx.doneBySong[songKey]:
			songState = downloadStateDone
		}
	}
	if key := onlinesvc.NormalizeSongKey(name, artist); key != "|" {
		switch {
		case idx.activeByKey[key]:
			keyState = downloadStateActive
		case idx.doneByKey[key]:
			keyState = downloadStateDone
		}
	}
	return songState, keyState
}

// loadDownloadIndex reads all tasks (no error surfacing: a lookup failure must
// never block a download, the library check still applies).
func (h *onlineHandler) loadDownloadIndex(ctx context.Context) downloadIndex {
	repo := h.ds.DownloadTask(ctx)
	if repo == nil {
		return buildDownloadIndex(nil)
	}
	tasks, err := repo.GetAll()
	if err != nil {
		log.Warn(ctx, "online: could not load download tasks for dedup", err)
		return buildDownloadIndex(nil)
	}
	return buildDownloadIndex(tasks)
}

// duplicateMessage renders the 409 reason for an already downloaded song.
func duplicateMessage(state string) string {
	if state == downloadStateActive {
		return "该歌曲正在下载中，请稍候（完成后即可播放）"
	}
	return "该歌曲已下载过，请勿重复下载（如需重新下载请先在下载管理中删除）"
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
		writeSourceError(w, r, err, "search failed: ")
		return
	}

	idx := h.loadDownloadIndex(ctx)
	resp := onlineSearchResponse{Songs: make([]onlineSong, 0, len(result.Songs)), Total: result.Total}
	for _, song := range result.Songs {
		inLibrary, err := h.ingest.Exists(ctx, song.Name, song.Artist)
		if err != nil {
			log.Warn(ctx, "online: dedup check failed", "song", song.Name, err)
		}
		songState, keyState := idx.state(song.Source, song.ID, song.Name, song.Artist)
		// A finished download whose file is still on disk counts as "in library"
		// even before the scan indexed it.
		downloaded := songState == downloadStateDone || keyState == downloadStateDone
		downloading := songState == downloadStateActive || keyState == downloadStateActive
		resp.Songs = append(resp.Songs, onlineSong{
			Song:        song,
			InLibrary:   inLibrary || downloaded,
			Downloading: downloading,
		})
	}
	writeJSON(w, resp)
}

// onlinePreviewRequest asks for a short-lived playable URL for one upstream song.
type onlinePreviewRequest struct {
	Source   string `json:"source"`
	Platform string `json:"platform"`
	SongID   string `json:"songId"`
	Time     int64  `json:"time"`
	Sign     string `json:"sign"`
	Duration int    `json:"duration"` // expected song length (seconds) from the search result
	Quality  string `json:"quality"`  // e.g. "mp3/128"; empty = preview default (low bitrate)
}

// onlinePreviewResponse is a playable direct media URL.
type onlinePreviewResponse struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Bitrate  int    `json:"bitrate"`
	Duration int    `json:"duration,omitempty"`
	Expires  int64  `json:"expires,omitempty"` // unix seconds hint for clients
}

// previewQuality prefers a moderate bitrate for 试听. IMPORTANT: upstream
// (hicn/kuwo) treats bitrate=128 as "trial clip" and returns a generic 12s
// placeholder instead of the real track — so previews (and downloads) must
// request 320k, which transparently downgrades to a real 128k full track
// when 320k is unavailable.
func previewQuality(qStr string) onlinesvc.Quality {
	if qStr != "" {
		if q := onlinesvc.ParseQuality(qStr); !q.IsZero() {
			if q.Bitrate == 128 {
				q.Bitrate = 320 // 128 = upstream trial-clip placeholder, never the real song
			}
			return q
		}
	}
	return onlinesvc.Quality{Format: "mp3", Bitrate: 320}
}

// preview: POST /api/online/preview — resolve a playable direct URL for 试听.
// The URL is temporary and anonymous-CDN-backed: clients can stream it directly
// with <audio>/MediaPlayer, no proxy needed. Requires login (any user may audition).
func (h *onlineHandler) preview(w http.ResponseWriter, r *http.Request) {
	if !conf.Server.OnlineDownload.Enabled {
		http.Error(w, "Online download is disabled", http.StatusForbidden)
		return
	}
	var req onlinePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SongID == "" {
		http.Error(w, "songId is required", http.StatusBadRequest)
		return
	}
	if req.Source == "" {
		req.Source = "hicn"
	}
	src := h.sourceByName(req.Source)
	if src == nil {
		http.Error(w, "unknown download source", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	quality := previewQuality(req.Quality)
	ref := onlinesvc.SongRef{
		Source:   req.Source,
		Platform: req.Platform,
		ID:       req.SongID,
		Time:     req.Time,
		Sign:     req.Sign,
	}
	// ResolveValidated screens upstream ad/placeholder responses (the "看广告
	// 免费畅听" voice clip served as any song) and re-resolves before giving up,
	// so a 试听 never plays the ad by accident.
	media, err := onlinesvc.ResolveValidated(ctx, src, ref, quality, req.Duration, 3)
	if err != nil {
		log.Warn(ctx, "online: preview resolve failed", "songId", req.SongID, err)
		writeSourceError(w, r, err, "preview failed: ")
		return
	}
	resp := onlinePreviewResponse{
		URL:      media.URL,
		Format:   media.Format,
		Bitrate:  media.Bitrate,
		Duration: media.Duration,
		Expires:  time.Now().Add(30 * time.Minute).Unix(),
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

	// Serialize check-and-enqueue so two rapid clicks (or the app retrying) cannot
	// both slip past the dedup gate before the first task is stored.
	downloadGateMu.Lock()
	defer downloadGateMu.Unlock()

	// Dedup gate #1: the same upstream song (source + song id) may never be
	// queued twice, even with fuzzy matching on.
	idx := h.loadDownloadIndex(ctx)
	songState, keyState := idx.state(req.Source, req.SongID, req.Name, req.Artist)
	if songState != "" {
		http.Error(w, duplicateMessage(songState), http.StatusConflict)
		return
	}
	// Dedup gate #2: a song with the same title+artist that is already being
	// downloaded is a duplicate click, not a new version.
	if keyState == downloadStateActive {
		http.Error(w, duplicateMessage(keyState), http.StatusConflict)
		return
	}
	// Dedup gate #3: library / finished-download check. Fuzzy matching keeps its
	// documented meaning (download another version on purpose).
	if !req.Fuzzy {
		exists, err := h.ingest.Exists(ctx, req.Name, req.Artist)
		if err != nil {
			http.Error(w, "dedup check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if exists || keyState == downloadStateDone {
			http.Error(w, duplicateMessage(downloadStateDone), http.StatusConflict)
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
		AlbumID:          onlinesvc.NormalizeAlbumID(req.AlbumID),
		CoverURL:         req.CoverURL,
		Duration:         req.Duration,
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
	// Content-Type must be set before the status code: WriteHeader commits the
	// headers, so setting it afterwards leaves the JSON body labelled
	// text/plain and the Android app rejects the (successful) response.
	writeJSONStatus(w, http.StatusCreated, task)
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
// Deletes the download record together with the song: the audio file on the NAS
// and the library entry it created (same semantics as the admin download
// management API, so the mobile app behaves like the web console).
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
	resp, err := deleteDownloadedArtifacts(ctx, h.ds, h.playlists, task, downloadLibraryRoots(h.ingest.LibraryRoot()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeOnlineOpLog(ctx, h.ds, task.UserID, model.OpLogDelete, task.SongName, task.Artist,
		strings.Join(resp.Warnings, "; "), resp.DeletedFile || resp.DeletedDB > 0)
	writeJSON(w, struct {
		Status string `json:"status"`
		deleteResponse
	}{Status: "deleted", deleteResponse: resp})
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

// ---- download permission settings ----

// onlineSettingsResponse is what both the web UI and the mobile app read to
// know whether the current user is allowed to download right now.
type onlineSettingsResponse struct {
	onlinesvc.Settings
	onlinesvc.Access
}

// getSettings: GET /api/online/settings
// Available to every logged-in user: the response carries the effective
// permission mode plus the caller's own access decision.
func (h *onlineHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, h.settingsResponse(ctx, user))
}

// updateSettings: PUT /api/online/settings (admin only)
// Body: {"permission":"open|admin-or-approved|quota","dailyQuota":<int>}
func (h *onlineHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Permission string `json:"permission"`
		DailyQuota *int   `json:"dailyQuota"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !onlinesvc.ValidPermission(req.Permission) {
		http.Error(w, "invalid permission mode", http.StatusBadRequest)
		return
	}
	next := onlinesvc.LoadSettings(ctx, h.ds)
	next.Permission = req.Permission
	if req.DailyQuota != nil {
		if *req.DailyQuota < 0 || *req.DailyQuota > 1000 {
			http.Error(w, "dailyQuota must be between 0 and 1000", http.StatusBadRequest)
			return
		}
		next.DailyQuota = *req.DailyQuota
	}
	saved, err := onlinesvc.SaveSettings(ctx, h.ds, next)
	if err != nil {
		log.Error(ctx, "online: could not save download settings", err)
		http.Error(w, "could not save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info(ctx, "Online download permission updated", "user", user.UserName, "permission", saved.Permission, "dailyQuota", saved.DailyQuota)
	writeJSON(w, h.settingsResponse(ctx, user))
}

func (h *onlineHandler) settingsResponse(ctx context.Context, user model.User) onlineSettingsResponse {
	settings := onlinesvc.LoadSettings(ctx, h.ds)
	used := 0
	if settings.Permission == onlinesvc.PermissionQuota {
		if n, err := h.countTodaysDownloads(ctx, user.ID); err == nil {
			used = n
		}
	}
	return onlineSettingsResponse{Settings: settings, Access: settings.CheckAccess(user.IsAdmin, used)}
}

// checkDownloadPermission enforces the runtime permission mode and daily quota.
// It writes an error response and returns non-nil when access is denied.
func (h *onlineHandler) checkDownloadPermission(ctx context.Context, user model.User, w http.ResponseWriter) error {
	settings := onlinesvc.LoadSettings(ctx, h.ds)
	used := 0
	if settings.Permission == onlinesvc.PermissionQuota {
		count, err := h.countTodaysDownloads(ctx, user.ID)
		if err != nil {
			http.Error(w, "quota check failed", http.StatusInternalServerError)
			return err
		}
		used = count
	}
	if settings.CheckAccess(user.IsAdmin, used).CanDownload {
		return nil
	}
	if settings.Permission == onlinesvc.PermissionQuota {
		http.Error(w, "daily download quota reached", http.StatusTooManyRequests)
		return errDailyQuota
	}
	http.Error(w, "only administrators can download (permission mode: admin-or-approved)", http.StatusForbidden)
	return errNotAdmin
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
