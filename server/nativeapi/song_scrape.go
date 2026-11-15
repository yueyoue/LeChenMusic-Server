package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	onlinesvc "github.com/navidrome/navidrome/server/online"
)

// M5.2 「歌曲刮削」：对曲库中的任意歌曲做元数据刮削
// （封面/歌词/标签内嵌重写），支持单曲与批量（前端循环调用）。

// newSongScrapeIngestor builds an Ingestor used only for its scrape helpers
// (search/cover/lyrics + tag rewriting); no download manager is involved.
func newSongScrapeIngestor() *onlinesvc.Ingestor {
	libPath := conf.Server.OnlineDownload.LibPath
	if libPath == "" {
		libPath = conf.Server.MusicFolder
	}
	return onlinesvc.NewIngestor(libPath, onlinesvc.GetDefaultSource(), nil)
}

// scrapeSong: POST /api/scrape/song/{id}/scrape
// Body (optional): {"title":"...","artist":"...","album":"..."} — corrected
// keywords for the online search. Empty fields use the file's current tags.
func (h *scrapeHandler) scrapeSong(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := request.UserFrom(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Rewriting library files is an admin operation.
	if !user.IsAdmin {
		http.Error(w, "access denied: admin privileges required", http.StatusForbidden)
		return
	}

	songID := chi.URLParam(r, "id")
	mf, err := h.ds.MediaFile(ctx).Get(songID)
	if err != nil {
		http.Error(w, "song not found", http.StatusNotFound)
		return
	}
	if mf.Path == "" {
		http.Error(w, "song has no file on disk", http.StatusConflict)
		return
	}

	var req onlinesvc.SongScrapeRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine

	log.Info(ctx, "Song scrape started", "user", user.UserName, "song", mf.Title, "artist", mf.Artist)
	// mf.Path is library-relative; the scrape rewriter needs the real
	// on-disk path (LibraryPath + Path), otherwise os.Stat fails and the
	// request 500s with "file not available".
	absPath := mf.AbsolutePath()
	if absPath == "" {
		http.Error(w, "cannot resolve file path on disk", http.StatusConflict)
		return
	}
	result, err := h.ingest.ScrapeSong(ctx, absPath, req)
	if err != nil {
		log.Warn(ctx, "Song scrape failed", "song", mf.Title, err)
		writeOnlineOpLog(ctx, h.ds, user.ID, model.OpLogSongScrape, mf.Title, mf.Artist, "刮削失败: "+err.Error(), false)
		http.Error(w, "scrape failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh library metadata from the rewritten file (best-effort).
	triggerLibraryScan(ctx, h.ds, h.pls)

	log.Info(ctx, "Song scrape done",
		"song", result.Info.Title, "matched", result.Matched,
		"cover", result.CoverFetched, "lyrics", result.LyricsFetched)
	detail := fmt.Sprintf("匹配=%v 封面=%v 歌词=%v", result.Matched, result.CoverFetched, result.LyricsFetched)
	writeOnlineOpLog(ctx, h.ds, user.ID, model.OpLogSongScrape, result.Info.Title, result.Info.Artist, detail, true)
	writeJSON(w, map[string]any{
		"result": result,
		"path":   filepath.Base(mf.Path),
	})
}
