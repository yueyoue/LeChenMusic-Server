// [LeChenMusic] Backup & Restore API routes
package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/backup"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func (api *Router) addBackupRoute(r chi.Router) {
	h := &backupHandler{ds: api.ds}
	r.Route("/backup", func(r chi.Router) {
		r.Post("/export", h.export)
		r.Get("/export/status", h.exportStatus)
		r.Post("/import", h.importBackup)
		r.Get("/import/status", h.importStatus)
		r.Get("/list", h.list)
		r.Get("/inspect", h.inspect)
		r.Get("/config", h.getConfig)
		r.Post("/config", h.saveConfig)
	})
}

type backupHandler struct {
	ds model.DataStore
}

// 异步任务管理（导出和导入共用）
type backupJob struct {
	ID        string               `json:"id"`
	Type      string               `json:"type"` // export / import
	Status    string               `json:"status"` // running / done / error
	Result    *backup.ExportResult `json:"result,omitempty"`
	ImportResult *backup.ImportResult `json:"import_result,omitempty"`
	Error     string               `json:"error,omitempty"`
	StartedAt time.Time            `json:"started_at"`
}

var (
	backupJobs   = map[string]*backupJob{}
	backupJobsMu sync.Mutex
)

func (h *backupHandler) export(w http.ResponseWriter, r *http.Request) {
	backupDir := getBackupDir()
	filename := "backup-" + time.Now().Format("2006-01-02-150405") + ".json"
	outputPath := filepath.Join(backupDir, filename)

	opts := backup.DefaultBackupOptions()
	if r.Body != nil {
		var reqOpts backup.BackupOptions
		if err := json.NewDecoder(r.Body).Decode(&reqOpts); err == nil {
			opts = reqOpts
		}
	}

	jobID := time.Now().Format("20060102150405.000")
	job := &backupJob{
		ID:        jobID,
		Type:      "export",
		Status:    "running",
		StartedAt: time.Now(),
	}
	backupJobsMu.Lock()
	backupJobs[jobID] = job
	backupJobsMu.Unlock()

	go func() {
		ctx := context.Background()
		log.Info(ctx, "Backup export started (async)", "job_id", jobID, "file", outputPath)
		result, err := backup.Export(ctx, h.ds, outputPath, "dev", &opts)
		backupJobsMu.Lock()
		if err != nil {
			job.Status = "error"
			job.Error = err.Error()
			log.Error(ctx, "Backup export failed", err, "job_id", jobID)
		} else {
			job.Status = "done"
			job.Result = result
			log.Info(ctx, "Backup export completed", "job_id", jobID,
				"users", result.UserCount, "artists", result.ArtistCount,
				"has_images", result.HasImages)
		}
		backupJobsMu.Unlock()
	}()

	writeJSON(w, map[string]any{"data": map[string]any{
		"job_id": jobID,
		"status": "running",
	}})
}

func (h *backupHandler) exportStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", 400)
		return
	}
	backupJobsMu.Lock()
	job, ok := backupJobs[jobID]
	backupJobsMu.Unlock()
	if !ok {
		http.Error(w, "job not found", 404)
		return
	}
	writeJSON(w, map[string]any{"data": job})
}

func (h *backupHandler) importBackup(w http.ResponseWriter, r *http.Request) {
	var opts backup.ImportOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	if opts.FilePath == "" {
		http.Error(w, "file_path required", 400)
		return
	}

	jobID := time.Now().Format("20060102150405.000")
	job := &backupJob{
		ID:        jobID,
		Type:      "import",
		Status:    "running",
		StartedAt: time.Now(),
	}
	backupJobsMu.Lock()
	backupJobs[jobID] = job
	backupJobsMu.Unlock()

	go func() {
		ctx := context.Background()
		log.Info(ctx, "Backup import started (async)", "job_id", jobID, "file", opts.FilePath)
		result, err := backup.Import(ctx, h.ds, opts)
		backupJobsMu.Lock()
		if err != nil {
			job.Status = "error"
			job.Error = err.Error()
			log.Error(ctx, "Backup import failed", err, "job_id", jobID)
		} else {
			job.Status = "done"
			job.ImportResult = result
			log.Info(ctx, "Backup import completed", "job_id", jobID,
				"users", result.UsersImported, "artists", result.ArtistsImported,
				"albums", result.AlbumsImported, "songs", result.SongsImported,
				"playlists", result.PlaylistsImported, "radios", result.RadiosImported,
				"images", result.ImagesRestored)
		}
		backupJobsMu.Unlock()
	}()

	writeJSON(w, map[string]any{"data": map[string]any{
		"job_id": jobID,
		"status": "running",
	}})
}

func (h *backupHandler) importStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", 400)
		return
	}
	backupJobsMu.Lock()
	job, ok := backupJobs[jobID]
	backupJobsMu.Unlock()
	if !ok {
		http.Error(w, "job not found", 404)
		return
	}
	writeJSON(w, map[string]any{"data": job})
}

func (h *backupHandler) list(w http.ResponseWriter, r *http.Request) {
	backupDir := getBackupDir()
	backups, err := backup.ListBackups(backupDir)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": backups})
}

func (h *backupHandler) inspect(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		http.Error(w, "file param required", 400)
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("read error: %v", err), 500)
		return
	}
	var bd backup.BackupData
	if err := json.Unmarshal(data, &bd); err != nil {
		http.Error(w, fmt.Sprintf("parse error: %v", err), 500)
		return
	}
	result := map[string]any{
		"version":       bd.Version,
		"created_at":    bd.CreatedAt,
		"users":         len(bd.Users),
		"libraries":     len(bd.Libraries),
		"artists":       len(bd.Artists),
		"albums":        len(bd.Albums),
		"songs":         len(bd.MediaFiles),
		"playlists":     len(bd.Playlists),
		"audiobooks":    len(bd.Audiobooks),
		"chapters":      len(bd.AudiobookChapters),
		"starred_songs": len(bd.StarredSongIDs),
		"starred_albums": len(bd.StarredAlbumIDs),
		"starred_artists": len(bd.StarredArtistIDs),
		"starred_abs":   len(bd.StarredAudiobookIDs),
		"progress":      len(bd.AudiobookProgress),
		"bookmarks":     len(bd.AudiobookBookmarks),
		"radios":        len(bd.Radios),
		"images":        len(bd.Images),
	}
	if len(bd.Users) > 0 {
		result["user0"] = bd.Users[0].UserName
	}
	if len(bd.Artists) > 0 {
		result["artist0"] = bd.Artists[0].Name
	}
	if len(bd.MediaFiles) > 0 {
		result["song0"] = bd.MediaFiles[0].Title
	}
	writeJSON(w, map[string]any{"data": result})
}

func (h *backupHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := backup.DefaultBackupConfig()
	val, err := h.ds.Property(r.Context()).Get("backup.config")
	if err == nil && val != "" {
		_ = json.Unmarshal([]byte(val), &cfg)
	}
	writeJSON(w, map[string]any{"data": cfg})
}

func (h *backupHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	var cfg backup.BackupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	data, _ := json.Marshal(cfg)
	if err := h.ds.Property(r.Context()).Put("backup.config", string(data)); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if cfg.BackupDir != "" {
		_ = os.MkdirAll(cfg.BackupDir, 0755)
	}
	writeJSON(w, map[string]any{"data": "ok"})
}

func getBackupDir() string {
	dir := conf.Server.DataFolder.String() + "/backups"
	_ = os.MkdirAll(dir, 0755)
	return dir
}
