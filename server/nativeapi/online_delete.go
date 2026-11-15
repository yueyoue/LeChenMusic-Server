package nativeapi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/conf"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// deleteResponse reports what a delete operation actually removed, so both the
// web UI and the mobile app can tell the user whether the audio file on the NAS
// was really deleted along with the record.
type deleteResponse struct {
	DeletedFile bool     `json:"deletedFile"`
	DeletedDB   int      `json:"deletedDb"`
	Warnings    []string `json:"warnings,omitempty"`
}

// downloadLibraryRoots returns the directories a downloaded song may live in:
// the dedicated online-download library path (when configured) plus the music
// folder. A file outside them is never removed, so a tampered/corrupt task
// record can't make the server delete unrelated files on the NAS.
func downloadLibraryRoots(extra ...string) []string {
	roots := make([]string, 0, 3)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return
		}
		for _, existing := range roots {
			if existing == abs {
				return
			}
		}
		roots = append(roots, abs)
	}
	add(conf.Server.OnlineDownload.LibPath)
	add(conf.Server.MusicFolder)
	for _, p := range extra {
		add(p)
	}
	return roots
}

// withinLibraryRoots reports whether path sits inside one of the given roots.
// With no configured roots the check is skipped (callers keep the legacy
// behaviour), which also keeps unit tests free of global config setup.
func withinLibraryRoots(path string, roots []string) bool {
	if len(roots) == 0 {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

// findMediaFilesForTask locates library records for the task's file: exact
// absolute path match, with a basename pre-filter to keep the scan cheap.
func findMediaFilesForTask(ctx context.Context, ds model.DataStore, task *model.DownloadTask) model.MediaFiles {
	if ds == nil || task.FilePath == "" {
		return nil
	}
	repo := ds.MediaFile(ctx)
	if repo == nil {
		return nil
	}
	base := filepath.Base(task.FilePath)
	mfs, err := repo.GetAll(model.QueryOptions{
		Filters: squirrel.Like{"path": "%" + base},
		Max:     100,
	})
	if err != nil {
		log.Warn(ctx, "online delete: media file lookup failed", "path", task.FilePath, err)
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

// otherTasksSharingFile counts the other download records pointing at the same
// file (legacy duplicates created before the dedup gate existed).
func otherTasksSharingFile(ctx context.Context, ds model.DataStore, task *model.DownloadTask) int {
	if ds == nil || task.FilePath == "" {
		return 0
	}
	repo := ds.DownloadTask(ctx)
	if repo == nil {
		return 0
	}
	tasks, err := repo.GetAll()
	if err != nil {
		return 0
	}
	count := 0
	for _, other := range tasks {
		if other.ID == task.ID || other.FilePath == "" {
			continue
		}
		if sameFilePath(other.FilePath, task.FilePath) {
			count++
		}
	}
	return count
}

// deleteDownloadedArtifacts removes everything that belongs to one downloaded
// song: the audio file on the NAS, the library record(s) created for it and the
// download task record itself. Shared by the admin download-management API and
// the per-user task API (used by the mobile app), so both behave the same.
//
// A non-nil error means the audio file could not be removed; nothing is deleted
// from the database in that case, so the user can retry instead of ending up
// with a library entry pointing at a file that still exists on disk.
func deleteDownloadedArtifacts(ctx context.Context, ds model.DataStore, pls playlistsvc.Playlists, task *model.DownloadTask, roots []string) (deleteResponse, error) {
	resp := deleteResponse{}
	if task == nil || ds == nil {
		return resp, nil
	}

	if task.FilePath != "" {
		switch {
		case !fileExists(task.FilePath):
			// Already gone: nothing to delete on disk (idempotent delete).
		case !withinLibraryRoots(task.FilePath, roots):
			resp.Warnings = append(resp.Warnings, "文件不在媒体库目录内，已跳过文件删除")
		default:
			if err := os.Remove(task.FilePath); err != nil && !os.IsNotExist(err) {
				return resp, fmt.Errorf("删除文件失败: %w", err)
			} else if err == nil {
				resp.DeletedFile = true
				// Older data may hold several records for one file; warn instead
				// of silently breaking them.
				if n := otherTasksSharingFile(ctx, ds, task); n > 0 {
					resp.Warnings = append(resp.Warnings,
						fmt.Sprintf("另有 %d 条下载记录指向该文件，其记录已失效", n))
				}
			}
		}
	}

	for _, mf := range findMediaFilesForTask(ctx, ds, task) {
		if err := ds.MediaFile(ctx).Delete(mf.ID); err != nil {
			log.Warn(ctx, "online delete: could not delete media file record", "id", mf.ID, err)
			resp.Warnings = append(resp.Warnings, "删除曲库记录失败: "+mf.ID)
			continue
		}
		resp.DeletedDB++
	}

	if repo := ds.DownloadTask(ctx); repo != nil {
		if err := repo.Delete(task.ID); err != nil {
			log.Warn(ctx, "online delete: could not delete task record", "task", task.ID, err)
			resp.Warnings = append(resp.Warnings, "删除任务记录失败")
		}
	}

	if resp.DeletedFile || resp.DeletedDB > 0 {
		// Library entries vanished from disk/DB: refresh so the song stops
		// showing up (best-effort, asynchronous).
		triggerLibraryScan(ctx, ds, pls)
	}
	return resp, nil
}