package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

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

	// Restore embedded images
	if len(backup.Images) > 0 {
		log.Info(ctx, "Restore: importing embedded images...", "count", len(backup.Images))
		restored := restoreEmbeddedImages(ctx, backup.Images)
		log.Info(ctx, "Restore: images done", "restored", restored, "total", len(backup.Images))
		result.ImagesRestored = true
	} else {
		imagesPath := ImagesTarPath(opts.FilePath)
		if _, imgErr := os.Stat(imagesPath); imgErr == nil {
			if err := ImportImages(ctx, imagesPath); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("图片文件恢复失败: %v", err))
			} else {
				result.ImagesRestored = true
			}
		}
	}

	collectError := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		result.Errors = append(result.Errors, msg)
		log.Warn(ctx, "Restore: "+msg)
	}

	// ═══════════════════════════════════════════════════════
	// ID 映射表：备份中的旧 ID → 数据库中的实际 ID
	// ═══════════════════════════════════════════════════════
	userIDMap := make(map[string]string)
	abIDMap := make(map[string]string)

	// ─── 1. Import users ─────────────────────────────────
	if opts.ImportUsers {
		for _, ub := range backup.Users {
			actualID := ub.ID
			existing, _ := ds.User(ctx).FindByUsername(ub.UserName)
			if existing != nil {
				if !opts.OverwriteUsers {
					userIDMap[ub.ID] = existing.ID
					continue
				}
				actualID = existing.ID
			}
			user := &model.User{
				ID: actualID, UserName: ub.UserName, Name: ub.Name,
				Email: ub.Email, Password: ub.Password, IsAdmin: ub.IsAdmin, CreatedAt: ub.CreatedAt,
			}
			if err := ds.User(ctx).Put(user); err != nil {
				collectError("用户 '%s' 导入失败: %v", ub.UserName, err)
				continue
			}
			userIDMap[ub.ID] = actualID
			result.UsersImported++
		}
	}
	log.Info(ctx, "Restore: users done", "imported", result.UsersImported, "mapped", len(userIDMap))

	// Admin context using actual DB ID
	adminCtx := ctx
	for _, u := range backup.Users {
		if u.IsAdmin {
			actualAdminID := userIDMap[u.ID]
			if actualAdminID == "" {
				actualAdminID = u.ID
			}
			adminCtx = request.WithUser(ctx, model.User{
				ID: actualAdminID, UserName: u.UserName, IsAdmin: true,
			})
			break
		}
	}

	// ─── 2. Import music metadata ────────────────────────
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
			collectError("歌手还有 %d 条导入失败", artistFail-5)
		}

		albumOK := 0
		for i := range backup.Albums {
			if err := ds.Album(ctx).Put(&backup.Albums[i]); err == nil {
				albumOK++
			}
		}
		result.AlbumsImported = albumOK

		songOK := 0
		for i := range backup.MediaFiles {
			if err := ds.MediaFile(ctx).Put(&backup.MediaFiles[i]); err == nil {
				songOK++
			}
		}
		result.SongsImported = songOK
		log.Info(ctx, "Restore: music done", "artists", artistOK, "albums", albumOK, "songs", songOK)
	}

	// ─── 3. Import audiobooks + chapters ─────────────────
	if opts.ImportAudiobookMeta {
		// Build path→existing map
		existingByPath := make(map[string]*model.Audiobook)
		if existingBooks, err := ds.Audiobook(ctx).GetAll(); err == nil {
			for i := range existingBooks {
				existingByPath[existingBooks[i].Path] = &existingBooks[i]
			}
		}

		bookOK, bookFail, bookUpdated := 0, 0, 0
		for i := range backup.Audiobooks {
			bak := &backup.Audiobooks[i]
			if existing, found := existingByPath[bak.Path]; found {
				// Path exists → update scraped fields, map backup ID → existing ID
				changed := false
				if bak.Description != "" && existing.Description == "" {
					existing.Description = bak.Description; changed = true
				}
				if bak.CoverUrl != "" && existing.CoverUrl == "" {
					existing.CoverUrl = bak.CoverUrl; changed = true
				}
				if bak.Author != "" && existing.Author == "" {
					existing.Author = bak.Author; changed = true
				}
				if bak.Narrator != "" && existing.Narrator == "" {
					existing.Narrator = bak.Narrator; changed = true
				}
				if bak.CoverPath != "" && existing.CoverPath == "" {
					existing.CoverPath = bak.CoverPath; changed = true
				}
				if bak.Genre != "" && bak.Genre != "有声读物" && existing.Genre == "有声读物" {
					existing.Genre = bak.Genre; changed = true
				}
				if bak.Year > 0 && existing.Year == 0 {
					existing.Year = bak.Year; changed = true
				}
				if bak.Series != "" && existing.Series == "" {
					existing.Series = bak.Series; changed = true
				}
				if changed {
					if err := ds.Audiobook(ctx).Put(existing); err != nil {
						bookFail++
					} else {
						bookUpdated++
					}
				}
				abIDMap[bak.ID] = existing.ID
			} else {
				// Path not found → insert new (Put preserves the backup ID)
			 bak.ID = bak.ID // keep backup ID
				if err := ds.Audiobook(ctx).Put(bak); err != nil {
					bookFail++
				} else {
					bookOK++
					abIDMap[bak.ID] = bak.ID
				}
			}
		}
		result.AudiobooksImported = bookOK + bookUpdated
		log.Info(ctx, "Restore: audiobooks done", "new", bookOK, "updated", bookUpdated, "fail", bookFail, "idMapSize", len(abIDMap))

		// Import chapters with remapped audiobook_id
		chOK, chFail := 0, 0
		for i := range backup.AudiobookChapters {
			ch := &backup.AudiobookChapters[i]
			if newABID, ok := abIDMap[ch.AudiobookID]; ok {
				ch.AudiobookID = newABID
			}
			if err := ds.Audiobook(ctx).PutChapter(ch); err != nil {
				chFail++
				if chFail <= 5 {
					collectError("章节 '%s' 导入失败: %v", ch.Title, err)
				}
			} else {
				chOK++
			}
		}
		result.ChaptersImported = chOK
		if chFail > 5 {
			collectError("章节还有 %d 条导入失败", chFail-5)
		}
		log.Info(ctx, "Restore: chapters done", "ok", chOK, "fail", chFail)

		// Clean up duplicates
		if allBooks, err := ds.Audiobook(ctx).GetAll(); err == nil {
			pathBooks := make(map[string][]model.Audiobook)
			for _, b := range allBooks {
				pathBooks[b.Path] = append(pathBooks[b.Path], b)
			}
			dupCount := 0
			for _, books := range pathBooks {
				if len(books) <= 1 {
					continue
				}
				keepIdx := 0
				for j := 1; j < len(books); j++ {
					if books[j].Description != "" || books[j].CoverUrl != "" {
						keepIdx = j; break
					}
				}
				for j, b := range books {
					if j == keepIdx { continue }
					if chs, chErr := ds.Audiobook(ctx).GetChapters(b.ID); chErr == nil {
						for _, ch := range chs {
							ch.AudiobookID = books[keepIdx].ID
							ds.Audiobook(ctx).PutChapter(&ch)
						}
					}
					ds.Audiobook(ctx).Delete(b.ID)
					dupCount++
				}
			}
			if dupCount > 0 {
				log.Info(ctx, "Restore: cleaned duplicates", "deleted", dupCount)
			}
		}
	}

	// ─── 4. Import playlists ─────────────────────────────
	if opts.ImportPlaylists {
		// Resolve admin user ID for owner_id fallback
		adminUserID := ""
		for _, u := range backup.Users {
			if u.IsAdmin {
				adminUserID = userIDMap[u.ID]
				if adminUserID == "" { adminUserID = u.ID }
				break
			}
		}

		plOK, plFail := 0, 0
		for _, pb := range backup.Playlists {
			pl := pb.Playlist
			// Remap owner_id
			if mappedID, ok := userIDMap[pl.OwnerID]; ok {
				pl.OwnerID = mappedID
			} else if pl.OwnerID != "" && adminUserID != "" {
				// Owner not in mapping, use admin as fallback
				log.Info(ctx, "Restore: playlist owner not found, using admin", "playlist", pl.Name)
				pl.OwnerID = adminUserID
			}
			if err := ds.Playlist(adminCtx).Put(&pl); err != nil {
				plFail++
				collectError("歌单 '%s' 导入失败: %v", pl.Name, err)
				continue
			}
			if len(pb.TrackIDs) > 0 {
				if _, err := ds.Playlist(adminCtx).Tracks(pl.ID, false).Add(pb.TrackIDs); err != nil {
					collectError("歌单 '%s' 添加曲目失败: %v", pl.Name, err)
				}
			}
			plOK++
		}
		result.PlaylistsImported = plOK
		log.Info(ctx, "Restore: playlists done", "ok", plOK, "fail", plFail)
	}

	// ─── 5. Import starred items ─────────────────────────
	if opts.ImportStarred {
		if len(backup.StarredSongIDs) > 0 {
			if err := ds.MediaFile(ctx).SetStar(true, backup.StarredSongIDs...); err != nil {
				collectError("歌曲收藏导入失败: %v", err)
			} else {
				result.StarredImported += len(backup.StarredSongIDs)
			}
		}
		if len(backup.StarredAlbumIDs) > 0 {
			ds.Album(ctx).SetStar(true, backup.StarredAlbumIDs...)
			result.StarredImported += len(backup.StarredAlbumIDs)
		}
		if len(backup.StarredArtistIDs) > 0 {
			ds.Artist(ctx).SetStar(true, backup.StarredArtistIDs...)
			result.StarredImported += len(backup.StarredArtistIDs)
		}
		// Starred audiobooks with remapped IDs
		actualUserID := ""
		if len(backup.Users) > 0 {
			actualUserID = userIDMap[backup.Users[0].ID]
			if actualUserID == "" { actualUserID = backup.Users[0].ID }
		}
		for _, abID := range backup.StarredAudiobookIDs {
			actualABID := abIDMap[abID]
			if actualABID == "" { actualABID = abID }
			if actualUserID != "" {
				if err := ds.Audiobook(ctx).Star(actualUserID, actualABID); err != nil {
					collectError("有声书收藏导入失败: %v", err)
				} else {
					result.StarredImported++
				}
			}
		}
	}

	// ─── 6. Import progress & bookmarks ──────────────────
	if opts.ImportProgress {
		progOK := 0
		for i := range backup.AudiobookProgress {
			p := &backup.AudiobookProgress[i]
			if newABID, ok := abIDMap[p.AudiobookID]; ok {
				p.AudiobookID = newABID
			}
			if newUID, ok := userIDMap[p.UserID]; ok {
				p.UserID = newUID
			}
			if err := ds.Audiobook(ctx).SaveProgress(p); err == nil {
				progOK++
			}
		}
		result.ProgressImported = progOK

		bmOK := 0
		for i := range backup.AudiobookBookmarks {
			bm := &backup.AudiobookBookmarks[i]
			if newABID, ok := abIDMap[bm.AudiobookID]; ok {
				bm.AudiobookID = newABID
			}
			if newUID, ok := userIDMap[bm.UserID]; ok {
				bm.UserID = newUID
			}
			if err := ds.Audiobook(ctx).SaveBookmark(bm); err == nil {
				bmOK++
			}
		}
		result.BookmarksImported = bmOK
		log.Info(ctx, "Restore: progress done", "progress", progOK, "bookmarks", bmOK)
	}

	// ─── 7. Import radio stations ────────────────────────
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
