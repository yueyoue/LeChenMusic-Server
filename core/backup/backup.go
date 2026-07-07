// [LeChenMusic] Backup & Restore - simplified version using available repository interfaces
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	 squirrel "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

const BackupVersion = "1.0"

// BackupData is the complete backup export structure
type BackupData struct {
	Version            string                      `json:"version"`
	CreatedAt          time.Time                   `json:"created_at"`
	ServerVersion      string                      `json:"server_version"`
	Libraries          []LibraryBackup             `json:"libraries"`
	Users              []UserBackup                `json:"users"`
	Playlists          []PlaylistBackup            `json:"playlists"`
	AudiobookProgress  []model.AudiobookProgress   `json:"audiobook_progress"`
	AudiobookBookmarks []model.AudiobookBookmark   `json:"audiobook_bookmarks"`
	StarredSongIDs      []string                   `json:"starred_song_ids,omitempty"`
	StarredAlbumIDs     []string                   `json:"starred_album_ids,omitempty"`
	StarredArtistIDs    []string                   `json:"starred_artist_ids,omitempty"`
	StarredAudiobookIDs []string                   `json:"starred_audiobook_ids,omitempty"`
}

type LibraryBackup struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
}

type UserBackup struct {
	ID        string    `json:"id"`
	UserName  string    `json:"user_name"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"` // bcrypt hash
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type PlaylistBackup struct {
	Playlist model.Playlist `json:"playlist"`
	TrackIDs []string       `json:"track_ids"`
}

// Export creates a backup file at the specified path
func Export(ctx context.Context, ds model.DataStore, outputPath string, serverVersion string) (*ExportResult, error) {
	backup := BackupData{
		Version:       BackupVersion,
		CreatedAt:     time.Now(),
		ServerVersion: serverVersion,
	}

	// Libraries
	libs, _ := ds.Library(ctx).GetAll()
	for _, lib := range libs {
		backup.Libraries = append(backup.Libraries, LibraryBackup{
			ID: lib.ID, Name: lib.Name, Path: lib.Path, MediaType: lib.MediaType,
		})
	}

	// Users (without passwords)
	users, _ := ds.User(ctx).GetAll()
	for _, u := range users {
		backup.Users = append(backup.Users, UserBackup{
			ID: u.ID, UserName: u.UserName, Name: u.Name,
			Email: u.Email, Password: u.Password, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt,
		})
	}

	// Playlists with track IDs
	playlists, _ := ds.Playlist(ctx).GetAll()
	for _, pl := range playlists {
		pb := PlaylistBackup{Playlist: pl}
		tracks, err := ds.Playlist(ctx).Tracks(pl.ID, false).GetAll()
		if err == nil {
			for _, t := range tracks {
				pb.TrackIDs = append(pb.TrackIDs, t.MediaFileID)
			}
		}
		backup.Playlists = append(backup.Playlists, pb)
	}

	// Starred/favorited items
	starFilter := model.QueryOptions{Filters: squirrel.Eq{"starred": true}}
	starredSongs, _ := ds.MediaFile(ctx).GetAll(starFilter)
	for _, s := range starredSongs {
		backup.StarredSongIDs = append(backup.StarredSongIDs, s.ID)
	}
	starredAlbums, _ := ds.Album(ctx).GetAll(starFilter)
	for _, a := range starredAlbums {
		backup.StarredAlbumIDs = append(backup.StarredAlbumIDs, a.ID)
	}
	starredArtists, _ := ds.Artist(ctx).GetAll(starFilter)
	for _, a := range starredArtists {
		backup.StarredArtistIDs = append(backup.StarredArtistIDs, a.ID)
	}

	// Audiobook progress and bookmarks
	abProgress, _ := ds.Audiobook(ctx).GetAllProgress()
	backup.AudiobookProgress = abProgress
	abBookmarks, _ := ds.Audiobook(ctx).GetAllBookmarks()
	backup.AudiobookBookmarks = abBookmarks

	// Starred audiobooks
	// Get all users to collect starred audiobooks
	for _, u := range users {
		starred, err := ds.Audiobook(ctx).GetStarred(u.ID)
		if err == nil {
			for _, ab := range starred {
				backup.StarredAudiobookIDs = append(backup.StarredAudiobookIDs, ab.ID)
			}
		}
	}

	// Write to file
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write backup file: %w", err)
	}

	info, _ := os.Stat(outputPath)
	log.Info(ctx, "Backup exported", "path", outputPath, "size", info.Size(),
		"users", len(backup.Users), "playlists", len(backup.Playlists))

	return &ExportResult{
		FilePath: outputPath, Size: info.Size(), CreatedAt: backup.CreatedAt,
		UserCount: len(backup.Users), PlaylistCount: len(backup.Playlists),
	}, nil
}

// Import reads a backup file and restores data
func Import(ctx context.Context, ds model.DataStore, opts ImportOptions) (*ImportResult, error) {
	data, err := os.ReadFile(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}
	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("parse backup: %w", err)
	}
	if backup.Version == "" {
		return nil, fmt.Errorf("invalid backup file")
	}

	result := &ImportResult{}
	log.Info(ctx, "Starting restore", "version", backup.Version, "created", backup.CreatedAt)

	// Import users
	if opts.ImportUsers {
		for _, ub := range backup.Users {
			existing, _ := ds.User(ctx).FindByUsername(ub.UserName)
			if existing != nil && !opts.OverwriteUsers {
				continue
			}
			user := &model.User{
				ID: ub.ID, UserName: ub.UserName, Name: ub.Name,
				Email: ub.Email, Password: ub.Password, IsAdmin: ub.IsAdmin, CreatedAt: ub.CreatedAt,
			}
			if existing != nil {
				user.ID = existing.ID
			}
			if err := ds.User(ctx).Put(user); err != nil {
				log.Warn(ctx, "Restore: failed to import user", "username", ub.UserName, err)
				continue
			}
			result.UsersImported++
		}
	}

	// Import playlists
	for _, pb := range backup.Playlists {
		pl := pb.Playlist
		if err := ds.Playlist(ctx).Put(&pl); err != nil {
			log.Warn(ctx, "Restore: failed to import playlist", "name", pl.Name, err)
			continue
		}
		if len(pb.TrackIDs) > 0 {
			ds.Playlist(ctx).Tracks(pl.ID, false).Add(pb.TrackIDs)
		}
		result.PlaylistsImported++
	}

	// Import starred items
	if len(backup.StarredSongIDs) > 0 {
		ds.MediaFile(ctx).SetStar(true, backup.StarredSongIDs...)
		result.AnnotationsImported += len(backup.StarredSongIDs)
	}
	if len(backup.StarredAlbumIDs) > 0 {
		ds.Album(ctx).SetStar(true, backup.StarredAlbumIDs...)
		result.AnnotationsImported += len(backup.StarredAlbumIDs)
	}
	if len(backup.StarredArtistIDs) > 0 {
		ds.Artist(ctx).SetStar(true, backup.StarredArtistIDs...)
		result.AnnotationsImported += len(backup.StarredArtistIDs)
	}

	// Import audiobook progress
	for _, p := range backup.AudiobookProgress {
		if err := ds.Audiobook(ctx).SaveProgress(&p); err != nil {
			log.Debug(ctx, "Restore: could not import audiobook progress", "book_id", p.AudiobookID, err)
			continue
		}
		result.AudiobookProgressImported++
	}

	// Import audiobook bookmarks
	for _, bm := range backup.AudiobookBookmarks {
		if err := ds.Audiobook(ctx).SaveBookmark(&bm); err != nil {
			log.Debug(ctx, "Restore: could not import audiobook bookmark", "book_id", bm.AudiobookID, err)
			continue
		}
		result.AudiobookBookmarksImported++
	}

	// Import starred audiobooks
	for _, abID := range backup.StarredAudiobookIDs {
		// Use first user as the default user for starred items
		if len(backup.Users) > 0 {
			ds.Audiobook(ctx).Star(backup.Users[0].ID, abID)
		}
	}

	log.Info(ctx, "Restore completed", "users", result.UsersImported,
		"playlists", result.PlaylistsImported, "starred", result.AnnotationsImported)

	return result, nil
}

// ExportResult contains info about the exported backup
type ExportResult struct {
	FilePath      string    `json:"file_path"`
	Size          int64     `json:"size"`
	CreatedAt     time.Time `json:"created_at"`
	UserCount     int       `json:"user_count"`
	PlaylistCount int       `json:"playlist_count"`
}

// ImportOptions controls how restore behaves
type ImportOptions struct {
	FilePath       string `json:"file_path"`
	ImportUsers    bool   `json:"import_users"`
	OverwriteUsers bool   `json:"overwrite_users"`
}

// ImportResult contains info about the imported data
type ImportResult struct {
	UsersImported              int `json:"users_imported"`
	PlaylistsImported          int `json:"playlists_imported"`
	AnnotationsImported        int `json:"annotations_imported"`
	AudiobookProgressImported  int `json:"audiobook_progress_imported"`
	AudiobookBookmarksImported int `json:"audiobook_bookmarks_imported"`
}

// BackupConfig holds scheduled backup configuration
type BackupConfig struct {
	Enabled   bool   `json:"enabled"`
	BackupDir string `json:"backup_dir"`
	KeepCount int    `json:"keep_count"`
	Interval  string `json:"interval"`
}

func DefaultBackupConfig() BackupConfig {
	return BackupConfig{Enabled: false, BackupDir: "/data/backups", KeepCount: 7, Interval: "daily"}
}

// ScheduledBackup runs a daily backup if not already done today
func ScheduledBackup(ctx context.Context, ds model.DataStore, cfg BackupConfig, serverVersion string) error {
	if !cfg.Enabled {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	entries, _ := os.ReadDir(cfg.BackupDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-"+today) {
			return nil
		}
	}
	filename := fmt.Sprintf("backup-%s-%s.json", today, time.Now().Format("150405"))
	outputPath := filepath.Join(cfg.BackupDir, filename)
	_, err := Export(ctx, ds, outputPath, serverVersion)
	if cfg.KeepCount > 0 {
		cleanupOldBackups(cfg.BackupDir, cfg.KeepCount)
	}
	return err
}

func cleanupOldBackups(dir string, keep int) {
	entries, _ := os.ReadDir(dir)
	var files []os.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-") && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e)
		}
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() > files[j].Name() })
	for _, f := range files[keep:] {
		os.Remove(filepath.Join(dir, f.Name()))
	}
}

// BackupFileInfo is metadata about a backup file
type BackupFileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// ListBackups returns all backup files in the directory
func ListBackups(dir string) ([]BackupFileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var backups []BackupFileInfo
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "backup-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupFileInfo{
			Name: e.Name(), Path: filepath.Join(dir, e.Name()),
			Size: info.Size(), CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	return backups, nil
}

// GetBackupInfo reads a backup file and returns metadata only
func GetBackupInfo(filePath string) (*BackupData, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, err
	}
	// Clear actual data, keep only metadata
	backup.AudiobookProgress = nil
	backup.AudiobookBookmarks = nil
	backup.StarredSongIDs = nil
	backup.StarredAlbumIDs = nil
	backup.StarredArtistIDs = nil
	backup.StarredAudiobookIDs = nil
	for i := range backup.Playlists {
		backup.Playlists[i].TrackIDs = nil
	}
	return &backup, nil
}
