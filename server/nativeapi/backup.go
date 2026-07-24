// [LeChenMusic] Backup & Restore API routes
package nativeapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
		r.Post("/import", h.importBackup)
		r.Get("/list", h.list)
		r.Get("/config", h.getConfig)
		r.Post("/config", h.saveConfig)
	})
}

type backupHandler struct {
	ds model.DataStore
}

func (h *backupHandler) export(w http.ResponseWriter, r *http.Request) {
	backupDir := getBackupDir()
	filename := "backup-" + time.Now().Format("2006-01-02-150405") + ".json"
	outputPath := filepath.Join(backupDir, filename)

	// 从请求体读取备份选项
	opts := backup.DefaultBackupOptions()
	if r.Body != nil {
		var reqOpts backup.BackupOptions
		if err := json.NewDecoder(r.Body).Decode(&reqOpts); err == nil {
			opts = reqOpts
		}
	}

	result, err := backup.Export(r.Context(), h.ds, outputPath, "dev", &opts)
	if err != nil {
		log.Error(r.Context(), "Backup export failed", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": result})
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
	result, err := backup.Import(r.Context(), h.ds, opts)
	if err != nil {
		log.Error(r.Context(), "Backup import failed", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": result})
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

// writeJSON is defined in audiobook.go
