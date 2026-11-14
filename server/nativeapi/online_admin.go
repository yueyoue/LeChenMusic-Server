package nativeapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	onlinesvc "github.com/navidrome/navidrome/server/online"
	"github.com/navidrome/navidrome/scanner"
)

// onlineAdminHandler serves the admin-only "download management" API (M5):
// list all users' downloaded songs with scrape verification info, delete them,
// re-scrape inaccurate metadata, and browse the operation log.
type onlineAdminHandler struct {
	ds     model.DataStore
	ingest *onlinesvc.Ingestor
	pls    playlistsvc.Playlists
}

func (h *onlineHandler) adminRoutes(r chi.Router) {
	ah := &onlineAdminHandler{ds: h.ds, ingest: h.ingest, pls: h.playlists}
	r.Group(func(r chi.Router) {
		r.Use(adminOnlyMiddleware)
		r.Get("/downloads", ah.listDownloads)
		r.Get("/downloads/{id}/verify", ah.verifyDownload)
		r.Delete("/downloads/{id}", ah.deleteDownload)
		r.Post("/downloads/{id}/rescrape", ah.rescrapeDownload)
		r.Get("/logs", ah.listLogs)
		r.Delete("/logs", ah.clearLogs)
	})
}

// ---- list ----

type adminDownloadItem struct {
	model.DownloadTask
	UserName   string                  `json:"userName"`
	FileExists bool                    `json:"fileExists"`
	Scrape     *onlinesvc.EmbeddedInfo `json:"scrape,omitempty"` // embedded metadata actually in the file
}

type adminDownloadList struct {
	Data  []adminDownloadItem `json:"data"`
	Total int                 `json:"total"`
}

// listDownloads: GET /api/online/downloads?user=&q=&scrape=&offset=&limit=
// Lists every user's finished downloads (newest first) with scrape status
// badges (cover/lyrics/tags) read back from the audio files.
func (ah *onlineAdminHandler) listDownloads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	tasks, err := ah.ds.DownloadTask(ctx).GetAll()
	if err != nil {
		http.Error(w, "could not load downloads", http.StatusInternalServerError)
		return
	}

	userFilter := strings.ToLower(strings.TrimSpace(q.Get("user")))
	keyword := strings.ToLower(strings.TrimSpace(q.Get("q")))
	scrapeFilter := q.Get("scrape") // "" | ok | missing-cover | missing-lyrics | missing-tags
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	userNames := ah.loadUserNames(ctx)

	items := make([]adminDownloadItem, 0, len(tasks))
	for _, t := range tasks {
		if t.Status != model.DownloadStatusDone {
			continue
		}
		// The filter matches either the user ID or the display name so the admin
		// UI can offer a human-readable "download user" search box.
		if userFilter != "" {
			who := strings.ToLower(t.UserID + " " + userNames[t.UserID])
			if !strings.Contains(who, userFilter) {
				continue
			}
		}
		if keyword != "" {
			hay := strings.ToLower(t.SongName + " " + t.Artist + " " + t.AlbumName)
			if !strings.Contains(hay, keyword) {
				continue
			}
		}

		item := adminDownloadItem{DownloadTask: t, UserName: userNames[t.UserID]}
		if item.UserName == "" {
			item.UserName = t.UserID
		}
		item.FileExists = fileExists(t.FilePath)
		if item.FileExists {
			if info, err := onlinesvc.ReadEmbeddedMetadata(t.FilePath); err == nil {
				info.Cover = nil // status only; full cover via /verify
				item.Scrape = info
			}
		}

		if scrapeFilter != "" && !matchesScrapeFilter(item, scrapeFilter) {
			continue
		}
		items = append(items, item)
	}

	// Newest download first; fall back to creation time for legacy rows that
	// never got a FinishedAt (e.g. imported/migrated tasks).
	sort.SliceStable(items, func(i, j int) bool {
		return downloadSortTime(items[i]).After(downloadSortTime(items[j]))
	})

	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, adminDownloadList{Data: items[offset:end], Total: total})
}

func downloadSortTime(item adminDownloadItem) time.Time {
	if !item.FinishedAt.IsZero() {
		return item.FinishedAt
	}
	return item.CreatedAt
}

func matchesScrapeFilter(item adminDownloadItem, filter string) bool {
	switch filter {
	case "ok":
		return item.FileExists && item.Scrape != nil && item.Scrape.HasTags() &&
			item.Scrape.HasCover && item.Scrape.Lyrics != ""
	case "missing-cover":
		return item.FileExists && (item.Scrape == nil || !item.Scrape.HasCover)
	case "missing-lyrics":
		return item.FileExists && (item.Scrape == nil || item.Scrape.Lyrics == "")
	case "missing-tags":
		return item.FileExists && (item.Scrape == nil || !item.Scrape.HasTags())
	}
	return true
}

// ---- verify ----

type verifyResponse struct {
	Task     model.DownloadTask      `json:"task"`
	UserName string                  `json:"userName"`
	Exists   bool                    `json:"exists"`
	Metadata *onlinesvc.EmbeddedInfo `json:"metadata,omitempty"`
	CoverURL string                  `json:"coverUrl,omitempty"` // data URI of the embedded cover
	Error    string                  `json:"error,omitempty"`
}

// verifyDownload: GET /api/online/downloads/{id}/verify
// Reads the embedded metadata back from the audio file so the admin can
// check whether the download + scraping is accurate.
func (ah *onlineAdminHandler) verifyDownload(w http.ResponseWriter, r *http.Request) {
	task, ok := ah.loadTask(w, r)
	if !ok {
		return
	}

	resp := verifyResponse{Task: *task, UserName: ah.userName(r, task.UserID)}
	if !fileExists(task.FilePath) {
		resp.Error = "文件不存在"
		writeJSON(w, resp)
		return
	}
	resp.Exists = true

	info, err := onlinesvc.ReadEmbeddedMetadata(task.FilePath)
	if err != nil {
		resp.Error = "无法读取内嵌标签: " + err.Error()
		writeJSON(w, resp)
		return
	}
	if len(info.Cover) > 0 {
		mime := info.CoverMIME
		if mime == "" {
			mime = "image/jpeg"
		}
		resp.CoverURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(info.Cover)
	}
	info.Cover = nil // already exposed as data URI
	resp.Metadata = info
	writeJSON(w, resp)
}

// ---- delete ----

type deleteResponse struct {
	DeletedFile bool     `json:"deletedFile"`
	DeletedDB   int      `json:"deletedDb"`
	Warnings    []string `json:"warnings,omitempty"`
}

// deleteDownload: DELETE /api/online/downloads/{id}
// Removes the downloaded song: audio file + library record(s) + task record.
// The operation log keeps the audit trail.
func (ah *onlineAdminHandler) deleteDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task, ok := ah.loadTask(w, r)
	if !ok {
		return
	}

	resp := deleteResponse{}

	if fileExists(task.FilePath) {
		if err := os.Remove(task.FilePath); err != nil {
			http.Error(w, "could not delete file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		resp.DeletedFile = true
	}

	for _, mf := range ah.findMediaFiles(ctx, task) {
		if err := ah.ds.MediaFile(ctx).Delete(mf.ID); err != nil {
			log.Warn(ctx, "online admin: could not delete media file record", "id", mf.ID, err)
			resp.Warnings = append(resp.Warnings, "删除曲库记录失败: "+mf.ID)
			continue
		}
		resp.DeletedDB++
	}

	if err := ah.ds.DownloadTask(ctx).Delete(task.ID); err != nil {
		log.Warn(ctx, "online admin: could not delete task record", "task", task.ID, err)
		resp.Warnings = append(resp.Warnings, "删除任务记录失败")
	}

	if resp.DeletedFile || resp.DeletedDB > 0 {
		// Library entries vanished from disk/DB: refresh so the song stops
		// showing up (best-effort, asynchronous).
		triggerLibraryScan(ctx, ah.ds, ah.pls)
	}

	ah.recordOp(ctx, model.OpLogDelete, task, resp.DeletedFile || resp.DeletedDB > 0, strings.Join(resp.Warnings, "; "))
	writeJSON(w, resp)
}

// ---- rescrape ----

// rescrapeDownload: POST /api/online/downloads/{id}/rescrape
// Body (all optional): {title, artist, album, coverUrl}
// Re-fetches cover/lyrics and rewrites the embedded metadata in place.
func (ah *onlineAdminHandler) rescrapeDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task, ok := ah.loadTask(w, r)
	if !ok {
		return
	}
	if !fileExists(task.FilePath) {
		http.Error(w, "file not found on disk", http.StatusConflict)
		return
	}

	var req onlinesvc.RescrapeRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := ah.ingest.Rescrape(ctx, task, req)
	if err != nil {
		ah.recordOp(ctx, model.OpLogRescrape, task, false, err.Error())
		http.Error(w, "rescrape failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Keep the task record in sync with corrected metadata.
	if req.Title != "" {
		task.SongName = req.Title
	}
	if req.Artist != "" {
		task.Artist = req.Artist
	}
	if req.Album != "" {
		task.AlbumName = req.Album
	}
	if err := ah.ds.DownloadTask(ctx).Update(task); err != nil {
		log.Warn(ctx, "online admin: could not update task after rescrape", "task", task.ID, err)
	}

	// Refresh library metadata from the rewritten file (best-effort).
	triggerLibraryScan(ctx, ah.ds, ah.pls)

	ah.recordOp(ctx, model.OpLogRescrape, task, true, "")
	writeJSON(w, result)
}

// ---- operation log ----

// adminLogList is a page of operation-log entries.
type adminLogList struct {
	Data  model.OnlineOpLogs `json:"data"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
	Pages int                `json:"pages"`
}

// listLogs: GET /api/online/logs?type=&user=&from=&to=&page=&limit=
// Supports filtering by action, by user (ID/name/detail), and by time range
// (RFC3339, "2006-01-02 15:04:05", "2006-01-02" or unix seconds/millis).
func (ah *onlineAdminHandler) listLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	logs, err := ah.ds.OnlineOpLog(ctx).GetAll()
	if err != nil {
		http.Error(w, "could not load logs", http.StatusInternalServerError)
		return
	}

	action := strings.ToLower(strings.TrimSpace(q.Get("type")))
	userFilter := strings.ToLower(strings.TrimSpace(q.Get("user")))
	from, hasFrom := parseTimeParam(q.Get("from"), false)
	to, hasTo := parseTimeParam(q.Get("to"), true)

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	filtered := make(model.OnlineOpLogs, 0, len(logs))
	for _, entry := range logs {
		if action != "" && !strings.EqualFold(entry.Action, action) {
			continue
		}
		if userFilter != "" {
			hay := strings.ToLower(entry.UserName + " " + entry.UserID + " " + entry.Detail)
			if !strings.Contains(hay, userFilter) {
				continue
			}
		}
		if hasFrom && entry.CreatedAt.Before(from) {
			continue
		}
		if hasTo && entry.CreatedAt.After(to) {
			continue
		}
		filtered = append(filtered, entry)
	}

	// Repository returns newest-first already; re-sort so ordering stays stable
	// after filtering (and for test/fake repositories without ORDER BY).
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	writeJSON(w, adminLogList{Data: filtered[start:end], Total: total, Page: page, Limit: limit, Pages: pages})
}

// clearLogs: DELETE /api/online/logs
func (ah *onlineAdminHandler) clearLogs(w http.ResponseWriter, r *http.Request) {
	if err := ah.ds.OnlineOpLog(r.Context()).DeleteAll(); err != nil {
		http.Error(w, "could not clear logs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "cleared"})
}

// ---- helpers ----

func (ah *onlineAdminHandler) loadTask(w http.ResponseWriter, r *http.Request) (*model.DownloadTask, bool) {
	taskID := chi.URLParam(r, "id")
	task, err := ah.ds.DownloadTask(r.Context()).Get(taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return nil, false
	}
	return task, true
}

func (ah *onlineAdminHandler) userName(r *http.Request, userID string) string {
	u, err := ah.ds.User(r.Context()).Get(userID)
	if err != nil || u == nil {
		return userID
	}
	return u.UserName
}

func (ah *onlineAdminHandler) loadUserNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	users, err := ah.ds.User(ctx).GetAll()
	if err != nil {
		return names
	}
	for _, u := range users {
		names[u.ID] = u.UserName
	}
	return names
}

// findMediaFiles locates library records for the task's file: exact absolute
// path match, with a basename pre-filter to keep the scan cheap.
func (ah *onlineAdminHandler) findMediaFiles(ctx context.Context, task *model.DownloadTask) model.MediaFiles {
	if task.FilePath == "" {
		return nil
	}
	repo := ah.ds.MediaFile(ctx)
	if repo == nil {
		return nil
	}
	base := filepath.Base(task.FilePath)
	mfs, err := repo.GetAll(model.QueryOptions{
		Filters: squirrel.Like{"path": "%" + base},
		Max:     100,
	})
	if err != nil {
		log.Warn(ctx, "online admin: media file lookup failed", "path", task.FilePath, err)
		return nil
	}
	var out model.MediaFiles
	for _, mf := range mfs {
		if sameFilePath(mf.AbsolutePath(), task.FilePath) {
			out = append(out, mf)
		}
	}
	return out
}

// recordOp appends an entry to the persistent operation log (best-effort).
// The entry is attributed to the acting user (the admin, for admin actions);
// when the acting user differs from the download's owner the owner is noted in
// Detail so the audit trail keeps both facts.
func (ah *onlineAdminHandler) recordOp(ctx context.Context, action string, task *model.DownloadTask, success bool, detail string) {
	repo := ah.ds.OnlineOpLog(ctx)
	if repo == nil {
		return
	}

	ownerName := ah.userNameFromID(ctx, task.UserID)
	actorID, actorName := task.UserID, ownerName
	if u, ok := request.UserFrom(ctx); ok && u.ID != "" {
		actorID = u.ID
		actorName = u.UserName
		if actorName == "" {
			actorName = ownerName
		}
	}
	if actorID != task.UserID {
		detail = joinDetail(detail, "下载用户: "+ownerName)
	}

	entry := &model.OnlineOpLog{
		UserID:   actorID,
		UserName: actorName,
		Action:   action,
		SongName: task.SongName,
		Artist:   task.Artist,
		Detail:   detail,
		Success:  success,
	}
	if err := repo.Add(entry); err != nil {
		log.Warn(ctx, "online admin: could not write op log", "action", action, err)
	}
}

func (ah *onlineAdminHandler) userNameFromID(ctx context.Context, userID string) string {
	u, err := ah.ds.User(ctx).Get(userID)
	if err != nil || u == nil {
		return userID
	}
	return u.UserName
}

func joinDetail(current, extra string) string {
	switch {
	case extra == "":
		return current
	case current == "":
		return extra
	default:
		return current + " | " + extra
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func sameFilePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}

// parseTimeParam accepts the formats the admin UI can produce. When endOfDay is
// true and only a date was given, the range end is extended to the end of that
// day so "to=2026-11-14" includes the whole day.
func parseTimeParam(value string, endOfDay bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n > 1e12 { // milliseconds
			return time.Unix(n/1000, (n%1000)*int64(time.Millisecond)), true
		}
		return time.Unix(n, 0), true
	}
	const dateOnly = "2006-01-02"
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", dateOnly}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err != nil {
			continue
		}
		if endOfDay && layout == dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t, true
	}
	return time.Time{}, false
}

// decodeOptionalJSON decodes a JSON body but tolerates an empty one.
func decodeOptionalJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return json.Unmarshal(body, dst)
}

// triggerLibraryScan refreshes library metadata after a file changed on disk.
// It is a no-op when the playlists service is unavailable (e.g. tests) or when
// a scan is already running.
func triggerLibraryScan(ctx context.Context, ds model.DataStore, pls playlistsvc.Playlists) {
	if ds == nil || pls == nil || scanner.IsScanning() {
		return
	}
	go func() {
		if _, err := scanner.CallScan(context.Background(), ds, pls, false, nil); err != nil {
			log.Warn(ctx, "online admin: could not trigger library scan", err)
		}
	}()
}