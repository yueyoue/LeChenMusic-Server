// [LeChenMusic] Backup & Restore - with embedded Base64 images
package backup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	squirrel "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

const BackupVersion = "3.0"

// BackupData is the complete backup export structure
type BackupData struct {
	Version            string                      `json:"version"`
	CreatedAt          time.Time                   `json:"created_at"`
	ServerVersion      string                      `json:"server_version"`
	Libraries          []LibraryBackup             `json:"libraries"`
	Users              []UserBackup                `json:"users"`
	Playlists          []PlaylistBackup            `json:"playlists,omitempty"`
	AudiobookProgress  []model.AudiobookProgress   `json:"audiobook_progress,omitempty"`
	AudiobookBookmarks []model.AudiobookBookmark   `json:"audiobook_bookmarks,omitempty"`
	StarredSongIDs      []string                   `json:"starred_song_ids,omitempty"`
	StarredAlbumIDs     []string                   `json:"starred_album_ids,omitempty"`
	StarredArtistIDs    []string                   `json:"starred_artist_ids,omitempty"`
	StarredAudiobookIDs []string                   `json:"starred_audiobook_ids,omitempty"`
	// 音乐元数据（含艺术家头像 URL）
	Artists           []model.Artist              `json:"artists,omitempty"`
	Albums            []model.Album               `json:"albums,omitempty"`
	MediaFiles        []model.MediaFile           `json:"media_files,omitempty"`
	// 有声书元数据
	Audiobooks        []model.Audiobook           `json:"audiobooks,omitempty"`
	AudiobookChapters []model.AudiobookChapter    `json:"audiobook_chapters,omitempty"`
	// 电台
	Radios            []model.Radio               `json:"radios,omitempty"`
	// 内嵌图片（Base64编码）
	Images            []ImageEntry                `json:"images,omitempty"`
}

// ImageEntry represents a single image file embedded in the backup
type ImageEntry struct {
	Key      string `json:"key"`       // Unique key for restore (e.g. "artwork/artist/ar-123_xxx.jpg")
	Path     string `json:"path"`      // Original absolute filesystem path
	MimeType string `json:"mime_type"` // MIME type (image/jpeg, image/png, etc.)
	Size     int64  `json:"size"`      // Original file size in bytes
	Data     string `json:"data"`      // Base64 encoded image data
}

// BackupOptions controls what data to include in the backup
type BackupOptions struct {
	IncludeMusicMeta     bool `json:"include_music_meta"`
	IncludeAudiobookMeta bool `json:"include_audiobook_meta"`
	IncludeStarred       bool `json:"include_starred"`
	IncludePlaylists     bool `json:"include_playlists"`
	IncludeProgress      bool `json:"include_progress"`
}

// DefaultBackupOptions returns options with everything enabled
func DefaultBackupOptions() BackupOptions {
	return BackupOptions{
		IncludeMusicMeta:     true,
		IncludeAudiobookMeta: true,
		IncludeStarred:       true,
		IncludePlaylists:     true,
		IncludeProgress:      true,
	}
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
func Export(ctx context.Context, ds model.DataStore, outputPath string, serverVersion string, opts *BackupOptions) (*ExportResult, error) {
	if opts == nil {
		def := DefaultBackupOptions()
		opts = &def
	}

	backup := BackupData{
		Version:       BackupVersion,
		CreatedAt:     time.Now(),
		ServerVersion: serverVersion,
	}

	// Libraries (always included)
	libs, _ := ds.Library(ctx).GetAll()
	for _, lib := range libs {
		backup.Libraries = append(backup.Libraries, LibraryBackup{
			ID: lib.ID, Name: lib.Name, Path: lib.Path, MediaType: lib.MediaType,
		})
	}

	// Users (always included)
	users, _ := ds.User(ctx).GetAll()
	for _, u := range users {
		backup.Users = append(backup.Users, UserBackup{
			ID: u.ID, UserName: u.UserName, Name: u.Name,
			Email: u.Email, Password: u.Password, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt,
		})
	}

	// Create admin context for operations that require permission checks (Playlist, Radio)
	adminCtx := ctx
	for _, u := range users {
		if u.IsAdmin {
			adminCtx = request.WithUser(ctx, model.User{
				ID: u.ID, UserName: u.UserName, IsAdmin: true,
			})
			break
		}
	}

	// Playlists with track IDs
	if opts.IncludePlaylists {
		playlists, _ := ds.Playlist(adminCtx).GetAll()
		for _, pl := range playlists {
			pb := PlaylistBackup{Playlist: pl}
			tracks, err := ds.Playlist(adminCtx).Tracks(pl.ID, false).GetAll()
			if err == nil {
				for _, t := range tracks {
					pb.TrackIDs = append(pb.TrackIDs, t.MediaFileID)
				}
			}
			backup.Playlists = append(backup.Playlists, pb)
		}
	}

	// Starred/favorited items
	if opts.IncludeStarred {
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
		// Starred audiobooks
		for _, u := range users {
			starred, err := ds.Audiobook(ctx).GetStarred(u.ID)
			if err == nil {
				for _, ab := range starred {
					backup.StarredAudiobookIDs = append(backup.StarredAudiobookIDs, ab.ID)
				}
			}
		}
	}

	// Audiobook progress and bookmarks
	if opts.IncludeProgress {
		abProgress, _ := ds.Audiobook(ctx).GetAllProgress()
		backup.AudiobookProgress = abProgress
		abBookmarks, _ := ds.Audiobook(ctx).GetAllBookmarks()
		backup.AudiobookBookmarks = abBookmarks
	}

	// Music metadata (artists with avatar URLs, albums, songs)
	if opts.IncludeMusicMeta {
		log.Info(ctx, "Backup: exporting music metadata...")
		allArtists, _ := ds.Artist(ctx).GetAll(model.QueryOptions{})
		backup.Artists = allArtists
		log.Info(ctx, "Backup: exported artists", "count", len(allArtists))

		allAlbums, _ := ds.Album(ctx).GetAll(model.QueryOptions{})
		backup.Albums = allAlbums
		log.Info(ctx, "Backup: exported albums", "count", len(allAlbums))

		allSongs, _ := ds.MediaFile(ctx).GetAll(model.QueryOptions{})
		backup.MediaFiles = allSongs
		log.Info(ctx, "Backup: exported songs", "count", len(allSongs))
	}

	// Audiobook metadata (books + chapters)
	if opts.IncludeAudiobookMeta {
		log.Info(ctx, "Backup: exporting audiobook metadata...")
		allBooks, _ := ds.Audiobook(ctx).GetAll()
		backup.Audiobooks = allBooks
		log.Info(ctx, "Backup: exported audiobooks", "count", len(allBooks))

		for _, book := range allBooks {
			chapters, err := ds.Audiobook(ctx).GetChapters(book.ID)
			if err == nil {
				backup.AudiobookChapters = append(backup.AudiobookChapters, chapters...)
			}
		}
		log.Info(ctx, "Backup: exported chapters", "count", len(backup.AudiobookChapters))
	}

	// Radio stations (always included)
	allRadios, _ := ds.Radio(adminCtx).GetAll()
	backup.Radios = allRadios
	log.Info(ctx, "Backup: exported radios", "count", len(allRadios))

	// Collect and embed images as Base64
	log.Info(ctx, "Backup: collecting images for embedding...")
	backup.Images = collectAllImages(ctx, ds)
	var totalImageSize int64
	for _, img := range backup.Images {
		totalImageSize += img.Size
	}
	log.Info(ctx, "Backup: images embedded", "count", len(backup.Images), "original_size", totalImageSize)

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
		"users", len(backup.Users), "playlists", len(backup.Playlists),
		"artists", len(backup.Artists), "albums", len(backup.Albums),
		"songs", len(backup.MediaFiles), "audiobooks", len(backup.Audiobooks),
		"images", len(backup.Images))

	result := &ExportResult{
		FilePath: outputPath, Size: info.Size(), CreatedAt: backup.CreatedAt,
		UserCount: len(backup.Users), PlaylistCount: len(backup.Playlists),
		ArtistCount: len(backup.Artists), AlbumCount: len(backup.Albums),
		SongCount: len(backup.MediaFiles), AudiobookCount: len(backup.Audiobooks),
		ImageCount: len(backup.Images), ImagesSize: totalImageSize,
		HasImages: len(backup.Images) > 0,
	}

	return result, nil
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

	// Restore embedded images (Base64 in JSON) - new format
	if len(backup.Images) > 0 {
		log.Info(ctx, "Restore: importing embedded images...", "count", len(backup.Images))
		restored := restoreEmbeddedImages(ctx, backup.Images)
		result.ImagesRestored = restored > 0
	} else {
		// Fallback: try legacy tar.gz image file
		imagesPath := ImagesTarPath(opts.FilePath)
		if _, imgErr := os.Stat(imagesPath); imgErr == nil {
			log.Info(ctx, "Restore: importing legacy tar.gz images...", "file", imagesPath)
			if err := ImportImages(ctx, imagesPath); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("图片文件恢复失败: %v", err))
			} else {
				result.ImagesRestored = true
			}
		}
	}

	// Helper to collect errors
	collectError := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		result.Errors = append(result.Errors, msg)
		log.Warn(ctx, "Restore: "+msg)
	}

	// Create an admin context for operations that require permission checks (Radio, Playlist)
	adminCtx := ctx
	if len(backup.Users) > 0 {
		for _, u := range backup.Users {
			if u.IsAdmin {
				adminCtx = request.WithUser(ctx, model.User{
					ID: u.ID, UserName: u.UserName, IsAdmin: true,
				})
				break
			}
		}
	}

	// 1. Import users
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
				collectError("用户 '%s' 导入失败: %v", ub.UserName, err)
				continue
			}
			result.UsersImported++
		}
	}

	// 2. Import music metadata (artists → albums → songs)
	if opts.ImportMusicMeta {
		artistOK, artistFail := 0, 0
		for i := range backup.Artists {
			if err := ds.Artist(ctx).Put(&backup.Artists[i]); err != nil {
				artistFail++
				if artistFail <= 5 {
					collectError("歌手 '%s' 导入失败: %v", backup.Artists[i].Name, err)
				}
			} else {
				artistOK++
			}
		}
		result.ArtistsImported = artistOK
		if artistFail > 5 {
			collectError("歌手还有 %d 条导入失败（已省略详细信息）", artistFail-5)
		}
		log.Info(ctx, "Restore: artists done", "ok", artistOK, "fail", artistFail)

		albumOK, albumFail := 0, 0
		for i := range backup.Albums {
			if err := ds.Album(ctx).Put(&backup.Albums[i]); err != nil {
				albumFail++
				if albumFail <= 5 {
					collectError("专辑 '%s' 导入失败: %v", backup.Albums[i].Name, err)
				}
			} else {
				albumOK++
			}
		}
		result.AlbumsImported = albumOK
		if albumFail > 5 {
			collectError("专辑还有 %d 条导入失败（已省略详细信息）", albumFail-5)
		}
		log.Info(ctx, "Restore: albums done", "ok", albumOK, "fail", albumFail)

		songOK, songFail := 0, 0
		for i := range backup.MediaFiles {
			if err := ds.MediaFile(ctx).Put(&backup.MediaFiles[i]); err != nil {
				songFail++
				if songFail <= 5 {
					collectError("歌曲 '%s' 导入失败: %v", backup.MediaFiles[i].Title, err)
				}
			} else {
				songOK++
			}
		}
		result.SongsImported = songOK
		if songFail > 5 {
			collectError("歌曲还有 %d 条导入失败（已省略详细信息）", songFail-5)
		}
		log.Info(ctx, "Restore: songs done", "ok", songOK, "fail", songFail)
	}

	// 3. Import audiobook metadata (books → chapters)
	if opts.ImportAudiobookMeta {
		bookOK, bookFail := 0, 0
		for i := range backup.Audiobooks {
			if err := ds.Audiobook(ctx).Put(&backup.Audiobooks[i]); err != nil {
				bookFail++
				if bookFail <= 5 {
					collectError("有声书 '%s' 导入失败: %v", backup.Audiobooks[i].Title, err)
				}
			} else {
				bookOK++
			}
		}
		result.AudiobooksImported = bookOK
		if bookFail > 5 {
			collectError("有声书还有 %d 条导入失败（已省略详细信息）", bookFail-5)
		}
		log.Info(ctx, "Restore: audiobooks done", "ok", bookOK, "fail", bookFail, "total_in_backup", len(backup.Audiobooks))

		chOK, chFail := 0, 0
		for i := range backup.AudiobookChapters {
			if err := ds.Audiobook(ctx).PutChapter(&backup.AudiobookChapters[i]); err != nil {
				chFail++
				if chFail <= 5 {
					collectError("章节 '%s' (书ID: %s) 导入失败: %v", backup.AudiobookChapters[i].Title, backup.AudiobookChapters[i].AudiobookID, err)
				}
			} else {
				chOK++
			}
		}
		result.ChaptersImported = chOK
		if chFail > 5 {
			collectError("章节还有 %d 条导入失败（已省略详细信息）", chFail-5)
		}
		log.Info(ctx, "Restore: chapters done", "ok", chOK, "fail", chFail, "total_in_backup", len(backup.AudiobookChapters))
	}

	// 4. Import playlists (depends on songs existing)
	if opts.ImportPlaylists {
		// Build set of valid user IDs
		validUserIDs := make(map[string]bool)
		for _, u := range backup.Users {
			validUserIDs[u.ID] = true
		}
		if existingUsers, err := ds.User(ctx).GetAll(); err == nil {
			for _, u := range existingUsers {
				validUserIDs[u.ID] = true
			}
		}
		// Get first admin user ID as fallback
		adminUserID := ""
		for _, u := range backup.Users {
			if u.IsAdmin {
				adminUserID = u.ID
				break
			}
		}

		plOK, plFail := 0, 0
		for _, pb := range backup.Playlists {
			pl := pb.Playlist
			// Fix owner_id if it references a non-existent user
			if pl.OwnerID != "" && !validUserIDs[pl.OwnerID] && adminUserID != "" {
				log.Info(ctx, "Restore: playlist owner not found, using admin", "playlist", pl.Name, "old_owner", pl.OwnerID)
				pl.OwnerID = adminUserID
			}
			if err := ds.Playlist(adminCtx).Put(&pl); err != nil {
				plFail++
				collectError("歌单 '%s' 导入失败: %v", pl.Name, err)
				continue
			}
			if len(pb.TrackIDs) > 0 {
				added, err := ds.Playlist(adminCtx).Tracks(pl.ID, false).Add(pb.TrackIDs)
				if err != nil {
					collectError("歌单 '%s' 添加曲目失败 (%d首): %v", pl.Name, len(pb.TrackIDs), err)
				} else {
					log.Info(ctx, "Restore: playlist tracks added", "name", pl.Name, "added", added)
				}
			}
			plOK++
		}
		result.PlaylistsImported = plOK
		log.Info(ctx, "Restore: playlists done", "ok", plOK, "fail", plFail, "total_in_backup", len(backup.Playlists))
	}

	// 5. Import starred items (depends on songs/albums/artists existing)
	if opts.ImportStarred {
		if len(backup.StarredSongIDs) > 0 {
			if err := ds.MediaFile(ctx).SetStar(true, backup.StarredSongIDs...); err != nil {
				collectError("歌曲收藏导入失败: %v", err)
			} else {
				result.StarredImported += len(backup.StarredSongIDs)
			}
		}
		if len(backup.StarredAlbumIDs) > 0 {
			if err := ds.Album(ctx).SetStar(true, backup.StarredAlbumIDs...); err != nil {
				collectError("专辑收藏导入失败: %v", err)
			} else {
				result.StarredImported += len(backup.StarredAlbumIDs)
			}
		}
		if len(backup.StarredArtistIDs) > 0 {
			if err := ds.Artist(ctx).SetStar(true, backup.StarredArtistIDs...); err != nil {
				collectError("歌手收藏导入失败: %v", err)
			} else {
				result.StarredImported += len(backup.StarredArtistIDs)
			}
		}
		// Import starred audiobooks
		for _, abID := range backup.StarredAudiobookIDs {
			if len(backup.Users) > 0 {
				if err := ds.Audiobook(ctx).Star(backup.Users[0].ID, abID); err != nil {
					collectError("有声书收藏导入失败 (ID: %s): %v", abID, err)
				} else {
					result.StarredImported++
				}
			}
		}
	}

	// 6. Import audiobook progress and bookmarks
	if opts.ImportProgress {
		progOK, progFail := 0, 0
		for _, p := range backup.AudiobookProgress {
			if err := ds.Audiobook(ctx).SaveProgress(&p); err != nil {
				progFail++
				if progFail <= 5 {
					collectError("播放进度导入失败 (书: %s): %v", p.AudiobookID, err)
				}
				continue
			}
			progOK++
		}
		result.ProgressImported = progOK
		if progFail > 5 {
			collectError("播放进度还有 %d 条导入失败（已省略详细信息）", progFail-5)
		}
		log.Info(ctx, "Restore: progress done", "ok", progOK, "fail", progFail, "total_in_backup", len(backup.AudiobookProgress))

		bmOK, bmFail := 0, 0
		for _, bm := range backup.AudiobookBookmarks {
			if err := ds.Audiobook(ctx).SaveBookmark(&bm); err != nil {
				bmFail++
				if bmFail <= 5 {
					collectError("书签导入失败 (书: %s): %v", bm.AudiobookID, err)
				}
				continue
			}
			bmOK++
		}
		result.BookmarksImported = bmOK
		if bmFail > 5 {
			collectError("书签还有 %d 条导入失败（已省略详细信息）", bmFail-5)
		}
		log.Info(ctx, "Restore: bookmarks done", "ok", bmOK, "fail", bmFail)
	}

	// 7. Import radio stations (requires admin permission)
	for i := range backup.Radios {
		if err := ds.Radio(adminCtx).Put(&backup.Radios[i]); err != nil {
			collectError("电台 '%s' 导入失败: %v", backup.Radios[i].Name, err)
		} else {
			result.RadiosImported++
		}
	}

	log.Info(ctx, "Restore completed",
		"users", result.UsersImported,
		"artists", result.ArtistsImported,
		"albums", result.AlbumsImported,
		"songs", result.SongsImported,
		"audiobooks", result.AudiobooksImported,
		"chapters", result.ChaptersImported,
		"playlists", result.PlaylistsImported,
		"starred", result.StarredImported,
		"progress", result.ProgressImported,
		"radios", result.RadiosImported)

	return result, nil
}

// ExportResult contains info about the exported backup
type ExportResult struct {
	FilePath       string    `json:"file_path"`
	Size           int64     `json:"size"`
	CreatedAt      time.Time `json:"created_at"`
	UserCount      int       `json:"user_count"`
	PlaylistCount  int       `json:"playlist_count"`
	ArtistCount    int       `json:"artist_count"`
	AlbumCount     int       `json:"album_count"`
	SongCount      int       `json:"song_count"`
	AudiobookCount int       `json:"audiobook_count"`
	ImageCount     int       `json:"image_count"`
	ImagesSize     int64     `json:"images_size,omitempty"`
	HasImages      bool      `json:"has_images"`
}

// ImportOptions controls restore behavior
type ImportOptions struct {
	FilePath           string `json:"file_path"`
	ImportUsers        bool   `json:"import_users"`
	OverwriteUsers     bool   `json:"overwrite_users"`
	ImportPlaylists    bool   `json:"import_playlists"`
	ImportStarred      bool   `json:"import_starred"`
	ImportProgress     bool   `json:"import_progress"`
	ImportMusicMeta    bool   `json:"import_music_meta"`
	ImportAudiobookMeta bool  `json:"import_audiobook_meta"`
}

// ImportResult contains info about the imported data
type ImportResult struct {
	UsersImported       int      `json:"users_imported"`
	ArtistsImported     int      `json:"artists_imported"`
	AlbumsImported      int      `json:"albums_imported"`
	SongsImported       int      `json:"songs_imported"`
	AudiobooksImported  int      `json:"audiobooks_imported"`
	ChaptersImported    int      `json:"chapters_imported"`
	PlaylistsImported   int      `json:"playlists_imported"`
	StarredImported     int      `json:"starred_imported"`
	ProgressImported    int      `json:"progress_imported"`
	BookmarksImported   int      `json:"bookmarks_imported"`
	RadiosImported      int      `json:"radios_imported"`
	ImagesRestored      bool     `json:"images_restored"`
	Errors              []string `json:"errors,omitempty"`
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
	opts := DefaultBackupOptions()
	_, err := Export(ctx, ds, outputPath, serverVersion, &opts)
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
		os.Remove(filepath.Join(dir, ImagesTarPath(f.Name())))
	}
}

// BackupFileInfo is metadata about a backup file
type BackupFileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	HasImages bool      `json:"has_images"`
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
		totalSize := info.Size()
		hasImg := false
		// Check for legacy tar.gz image file
		imgInfo, imgErr := os.Stat(filepath.Join(dir, ImagesTarPath(e.Name())))
		if imgErr == nil {
			totalSize += imgInfo.Size()
			hasImg = true
		}
		// Check for embedded images in JSON (new format)
		if !hasImg {
			bData, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
			if readErr == nil {
				var partial struct {
					Images []json.RawMessage `json:"images"`
				}
				if json.Unmarshal(bData, &partial) == nil && len(partial.Images) > 0 {
					hasImg = true
				}
			}
		}
		backups = append(backups, BackupFileInfo{
			Name: e.Name(), Path: filepath.Join(dir, e.Name()),
			Size: totalSize, CreatedAt: info.ModTime(), HasImages: hasImg,
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
	backup.MediaFiles = nil
	backup.AudiobookChapters = nil
	for i := range backup.Playlists {
		backup.Playlists[i].TrackIDs = nil
	}
	// Clear image data (keep count only)
	backup.Images = nil
	return &backup, nil
}

// ExportImages exports all image files to a tar.gz file (legacy, kept for backward compatibility)
func ExportImages(ctx context.Context, ds model.DataStore, outputPath string) (int64, error) {
	dataDir := conf.Server.DataFolder.String()
	var dirs []string
	for _, d := range []string{"artwork", "artist-images", "narrator-avatars"} {
		full := filepath.Join(dataDir, d)
		if _, err := os.Stat(full); err == nil {
			dirs = append(dirs, full)
		}
	}
	log.Info(ctx, "Backup: image dirs found", "dirs", dirs, "dataDir", dataDir)
	libs, _ := ds.Library(ctx).GetAll()
	coverNames := map[string]bool{"cover.jpg": true, "cover.jpeg": true, "cover.png": true, "folder.jpg": true, "folder.jpeg": true, "folder.png": true}
	var coverFiles []string
	for _, lib := range libs {
		filepath.Walk(lib.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && coverNames[info.Name()] {
				coverFiles = append(coverFiles, path)
			}
			return nil
		})
	}
	if len(dirs) == 0 && len(coverFiles) == 0 {
		log.Info(ctx, "Backup: no image dirs or cover files found, skipping image backup")
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return 0, err
	}
	// Use absolute paths with --absolute-names to preserve full paths in archive
	args := []string{"-czf", outputPath, "--absolute-names"}
	args = append(args, dirs...)
	args = append(args, coverFiles...)
	log.Info(ctx, "Backup: creating image tar", "output", outputPath)
	cmd := exec.CommandContext(ctx, "tar", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("tar: %w: %s", err, stderr.String())
	}
	if info, _ := os.Stat(outputPath); info != nil {
		log.Info(ctx, "Backup: images exported", "size", info.Size(), "covers", len(coverFiles))
		return info.Size(), nil
	}
	return 0, nil
}

// ImportImages restores image files from a tar.gz file (legacy)
func ImportImages(ctx context.Context, imagesPath string) error {
	f, err := os.Open(imagesPath)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.CommandContext(ctx, "tar", "-xzf", "-", "--absolute-names")
	cmd.Stdin = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extract images: %w: %s", err, stderr.String())
	}
	log.Info(ctx, "Restore: images imported", "from", imagesPath)
	return nil
}

// ==================== Image Base64 Embedding ====================

// isImageFile checks if a file is an image by extension
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// fileToBase64 reads a file and returns its base64-encoded content, MIME type, and size
func fileToBase64(filePath string) (encoded string, mimeType string, size int64, err error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", 0, err
	}
	size = int64(len(data))
	// Detect MIME type
	mimeBuf := data
	if len(mimeBuf) > 512 {
		mimeBuf = mimeBuf[:512]
	}
	mimeType = http.DetectContentType(mimeBuf)
	// Fallback for common types
	if mimeType == "application/octet-stream" {
		switch strings.ToLower(filepath.Ext(filePath)) {
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".png":
			mimeType = "image/png"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		}
	}
	encoded = base64.StdEncoding.EncodeToString(data)
	return encoded, mimeType, size, nil
}

// base64ToFile decodes base64 data and writes it to a file
func base64ToFile(encoded, filePath string) error {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("base64 decode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(filePath, data, 0644)
}

// collectAllImages gathers all images from data directory and library paths
func collectAllImages(ctx context.Context, ds model.DataStore) []ImageEntry {
	var images []ImageEntry
	dataDir := conf.Server.DataFolder.String()
	seen := make(map[string]bool) // deduplicate by absolute path

	// 1. Collect all files from artwork directory (artist, playlist, radio uploads)
	artworkDir := filepath.Join(dataDir, consts.ArtworkFolder)
	images = append(images, collectImagesFromDir(ctx, artworkDir, "artwork", seen)...)

	// 2. Collect artist-images directory (scraped artist images)
	artistImgDir := filepath.Join(dataDir, "artist-images")
	images = append(images, collectImagesFromDir(ctx, artistImgDir, "artist-images", seen)...)

	// 3. Collect narrator-avatars directory
	narratorDir := filepath.Join(dataDir, "narrator-avatars")
	images = append(images, collectImagesFromDir(ctx, narratorDir, "narrator-avatars", seen)...)

	// 4. Collect audiobook cover files from library paths
	images = append(images, collectAudiobookCovers(ctx, ds, seen)...)

	// 5. Collect album cover files from library paths
	images = append(images, collectAlbumCovers(ctx, ds, seen)...)

	log.Info(ctx, "Backup: image collection complete", "total_images", len(images))
	return images
}

// collectImagesFromDir walks a directory and collects all image files
func collectImagesFromDir(ctx context.Context, dir, prefix string, seen map[string]bool) []ImageEntry {
	var images []ImageEntry
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Info(ctx, "Backup: image dir not found, skipping", "dir", dir)
		return nil
	}
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !isImageFile(path) {
			return nil
		}
		absPath, _ := filepath.Abs(path)
		if seen[absPath] {
			return nil
		}
		seen[absPath] = true

		encoded, mimeType, size, err := fileToBase64(path)
		if err != nil {
			log.Warn(ctx, "Backup: failed to read image", "path", path, "error", err)
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		images = append(images, ImageEntry{
			Key:      prefix + "/" + relPath,
			Path:     absPath,
			MimeType: mimeType,
			Size:     size,
			Data:     encoded,
		})
		count++
		return nil
	})
	if err != nil {
		log.Warn(ctx, "Backup: error walking image dir", "dir", dir, "error", err)
	}
	log.Info(ctx, "Backup: collected images from dir", "dir", dir, "count", count)
	return images
}

// collectAudiobookCovers collects cover images for all audiobooks
func collectAudiobookCovers(ctx context.Context, ds model.DataStore, seen map[string]bool) []ImageEntry {
	var images []ImageEntry
	allBooks, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		log.Warn(ctx, "Backup: failed to get audiobooks for cover collection", "error", err)
		return nil
	}
	// Build library path map
	libs, _ := ds.Library(ctx).GetAll()
	libPaths := make(map[int]string)
	for _, lib := range libs {
		libPaths[lib.ID] = lib.Path
	}

	count := 0
	for _, book := range allBooks {
		if book.CoverPath == "" {
			continue
		}
		libPath, ok := libPaths[book.LibraryID]
		if !ok {
			continue
		}
		absCover := filepath.Join(libPath, book.CoverPath)
		if seen[absCover] {
			continue
		}
		if _, err := os.Stat(absCover); os.IsNotExist(err) {
			continue
		}
		seen[absCover] = true

		encoded, mimeType, size, err := fileToBase64(absCover)
		if err != nil {
			log.Warn(ctx, "Backup: failed to read audiobook cover", "book", book.Title, "path", absCover, "error", err)
			continue
		}
		images = append(images, ImageEntry{
			Key:      "audiobook-cover/" + book.ID + filepath.Ext(absCover),
			Path:     absCover,
			MimeType: mimeType,
			Size:     size,
			Data:     encoded,
		})
		count++
	}
	log.Info(ctx, "Backup: collected audiobook covers", "count", count)
	return images
}

// collectAlbumCovers collects cover files (cover.jpg, folder.jpg, etc.) from all library paths
func collectAlbumCovers(ctx context.Context, ds model.DataStore, seen map[string]bool) []ImageEntry {
	var images []ImageEntry
	libs, _ := ds.Library(ctx).GetAll()
	coverNames := map[string]bool{
		"cover.jpg": true, "cover.jpeg": true, "cover.png": true,
		"folder.jpg": true, "folder.jpeg": true, "folder.png": true,
	}

	count := 0
	for _, lib := range libs {
		if lib.MediaType == "audiobook" {
			continue // audiobook covers handled separately
		}
		err := filepath.Walk(lib.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !coverNames[info.Name()] {
				return nil
			}
			absPath, _ := filepath.Abs(path)
			if seen[absPath] {
				return nil
			}
			seen[absPath] = true

			encoded, mimeType, size, err := fileToBase64(path)
			if err != nil {
				return nil
			}
			relPath, _ := filepath.Rel(lib.Path, path)
			images = append(images, ImageEntry{
				Key:      "album-cover/" + relPath,
				Path:     absPath,
				MimeType: mimeType,
				Size:     size,
				Data:     encoded,
			})
			count++
			return nil
		})
		if err != nil {
			log.Warn(ctx, "Backup: error walking library for covers", "lib", lib.Name, "error", err)
		}
	}
	log.Info(ctx, "Backup: collected album covers", "count", count)
	return images
}

// restoreEmbeddedImages writes all embedded images back to their original paths
func restoreEmbeddedImages(ctx context.Context, images []ImageEntry) int {
	restored := 0
	for _, img := range images {
		targetPath := img.Path
		if targetPath == "" {
			continue
		}
		// Skip if file already exists (don't overwrite user's files)
		if _, err := os.Stat(targetPath); err == nil {
			log.Trace(ctx, "Restore: image already exists, skipping", "path", targetPath)
			continue
		}
		if err := base64ToFile(img.Data, targetPath); err != nil {
			log.Warn(ctx, "Restore: failed to write image", "path", targetPath, "error", err)
			continue
		}
		restored++
	}
	log.Info(ctx, "Restore: embedded images restored", "total", len(images), "restored", restored)
	return restored
}

func ImagesTarPath(jsonPath string) string {
	return strings.TrimSuffix(jsonPath, ".json") + ".images.tar.gz"
}

func hasImagesTar(jsonPath string) bool {
	_, err := os.Stat(ImagesTarPath(jsonPath))
	return err == nil
}

func BackupFileSize(jsonPath string) int64 {
	var total int64
	if info, err := os.Stat(jsonPath); err == nil {
		total += info.Size()
	}
	if info, err := os.Stat(ImagesTarPath(jsonPath)); err == nil {
		total += info.Size()
	}
	return total
}

func copyFile(dst, src string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, sf)
	return err
}
