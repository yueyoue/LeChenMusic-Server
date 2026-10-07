package nativeapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	fspath "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/cloudsource"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/model/request"
	taglib "go.senan.xyz/taglib"
)

// [LeChenMusic-START:audiobook]

var audiobookAudioExts = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".flac": true,
	".ogg": true, ".wav": true, ".opus": true, ".wma": true, ".aac": true,
}

func (api *Router) addAudiobookRoute(r chi.Router) {
	h := &audiobookHandler{ds: api.ds}
	r.Route("/audiobook", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/search", h.search)
		r.Get("/genres", h.genres)
		r.Get("/narrators", h.narrators)
		r.Get("/narrator/{name}", h.narratorDetail)
		r.Get("/starred", h.starred)
		r.Get("/with-progress", h.listWithProgress)
		r.Get("/recent-progress", h.recentProgress)
		r.Get("/{id}", h.get)
		r.Get("/{id}/chapters", h.chapters)
		r.Get("/{id}/chapters/{chapterId}/stream", h.stream)
		r.Get("/{id}/progress", h.getProgress)
		r.Put("/{id}/progress", h.saveProgress)
		r.Get("/{id}/bookmarks", h.getBookmarks)
		r.Post("/{id}/bookmarks", h.saveBookmark)
		r.Delete("/{id}/bookmarks/{bookmarkId}", h.deleteBookmark)
		r.Post("/{id}/star", h.star)
		r.Delete("/{id}/star", h.unstar)
		r.Put("/{id}/metadata", h.updateMetadata)
		r.Get("/{id}/cover", h.cover)
		r.Post("/{id}/cover", h.uploadCover) // Upload cover image (file or URL)
		r.Post("/{id}/rescan", h.rescan)
		r.Post("/rescan-all", h.rescanAll)                        // Batch rescan all audiobooks
		r.Post("/purge-missing", h.purgeMissing)                  // Immediately drop books whose folder is gone (admin)
		r.Post("/narrator/{name}/avatar", h.uploadNarratorAvatar) // Upload narrator avatar
		r.Get("/narrator/{name}/avatar", h.getNarratorAvatar)     // Serve narrator avatar
	})
}

type audiobookHandler struct {
	ds model.DataStore
}

func (h *audiobookHandler) list(w http.ResponseWriter, r *http.Request) {
	WarmBookCovers(h.ds) // kick off background cover pre-warm (no-op after the first call)
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if books == nil {
		books = model.Audiobooks{}
	}
	writeCachedJSON(w, r, map[string]any{"data": books}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) listWithProgress(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if books == nil {
		books = model.Audiobooks{}
	}
	progressList, _ := repo.GetUserProgress(usr.ID)
	progressMap := make(map[string]*model.AudiobookProgress)
	for i := range progressList {
		p := &progressList[i]
		progressMap[p.AudiobookID] = p
	}
	type bookWithProgress struct {
		model.Audiobook
		Progress *model.AudiobookProgress `json:"progress"`
	}
	result := make([]bookWithProgress, 0, len(books))
	for _, b := range books {
		result = append(result, bookWithProgress{
			Audiobook: b,
			Progress:  progressMap[b.ID],
		})
	}
	writeCachedJSON(w, r, map[string]any{"data": result}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) recentProgress(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	progressList, _ := repo.GetUserProgress(usr.ID)
	if len(progressList) == 0 {
		writeCachedJSON(w, r, map[string]any{"data": []any{}}, cacheMaxAgeVolatile)
		return
	}
	// 批量取回书目 + 章节数（两条查询）。原实现逐本 Get + GetChapters，
	// 还把整章列表读进内存只为数个数——20 本在读就是 40 次查询 + 上万行读取，
	// 进入有声书首页光这一个接口就要 400ms+，是"进入有声书卡"的服务端主因。
	ids := make([]string, 0, len(progressList))
	for _, p := range progressList {
		ids = append(ids, p.AudiobookID)
	}
	books, err := repo.GetMany(ids)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	counts, _ := repo.ChapterCounts(ids)
	byID := make(map[string]*model.Audiobook, len(books))
	for i := range books {
		byID[books[i].ID] = &books[i]
	}
	type bookWithProgress struct {
		model.Audiobook
		Progress *model.AudiobookProgress `json:"progress"`
	}
	result := make([]bookWithProgress, 0, len(progressList))
	for _, p := range progressList {
		book, found := byID[p.AudiobookID]
		if !found {
			continue // 书目已被清理（等价于旧实现的逐本 miss）
		}
		b := *book
		if c, ok := counts[b.ID]; ok {
			b.ChapterCount = c
		}
		pCopy := p
		result = append(result, bookWithProgress{
			Audiobook: b,
			Progress:  &pCopy,
		})
	}
	writeCachedJSON(w, r, map[string]any{"data": result}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeCachedJSON(w, r, map[string]any{"data": []any{}}, cacheMaxAgeVolatile)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	q := strings.ToLower(query)
	results := make([]model.Audiobook, 0)
	for _, b := range books {
		if strings.Contains(strings.ToLower(b.Title), q) ||
			strings.Contains(strings.ToLower(b.Author), q) ||
			strings.Contains(strings.ToLower(b.Narrator), q) ||
			strings.Contains(strings.ToLower(b.Series), q) {
			results = append(results, b)
		}
	}
	writeCachedJSON(w, r, map[string]any{"data": results}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) genres(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	genreMap := map[string]int{}
	for _, b := range books {
		g := b.Genre
		if g == "" {
			g = "有声读物"
		}
		genreMap[g]++
	}
	type gi struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var genres []gi
	for name, count := range genreMap {
		genres = append(genres, gi{Name: name, Count: count})
	}
	writeCachedJSON(w, r, map[string]any{"data": genres}, cacheMaxAgeStable)
}

func (h *audiobookHandler) narrators(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type ni struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	nm := map[string]int{}
	for _, b := range books {
		if b.Narrator != "" {
			nm[b.Narrator]++
		}
	}
	narrators := make([]ni, 0)
	for name, count := range nm {
		narrators = append(narrators, ni{Name: name, Count: count})
	}
	writeCachedJSON(w, r, map[string]any{"data": narrators}, cacheMaxAgeStable)
}

func (h *audiobookHandler) narratorDetail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var works []model.Audiobook
	for _, b := range books {
		if strings.EqualFold(b.Narrator, name) {
			works = append(works, b)
		}
	}
	writeCachedJSON(w, r, map[string]any{"data": map[string]any{"name": name, "works": works}}, cacheMaxAgeStable)
}

func (h *audiobookHandler) starred(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetStarred(usr.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if books == nil {
		books = model.Audiobooks{}
	}
	// Populate starred timestamps in one query (was one GetStarredAt per book: N+1)
	starredAtMap, _ := repo.GetStarredAtMap(usr.ID)
	for i := range books {
		if starredAt, ok := starredAtMap[books[i].ID]; ok && starredAt != "" {
			books[i].Starred = starredAt
		}
	}
	writeCachedJSON(w, r, map[string]any{"data": books}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(id)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	// Populate starred status and progress for current user
	usr, ok := request.UserFrom(r.Context())
	var progress *model.AudiobookProgress
	if ok {
		starredAt, starredErr := repo.GetStarredAt(usr.ID, id)
		log.Debug(r.Context(), "GetStarredAt result", "userID", usr.ID, "bookID", id, "starredAt", starredAt, "error", starredErr)
		if starredAt != "" {
			book.Starred = starredAt
		}
		p, err := repo.GetProgress(usr.ID, id)
		if err == nil && p != nil {
			progress = p
		}
	}
	chapters, _ := repo.GetChapters(id)
	if chapters == nil {
		chapters = model.AudiobookChapters{}
	}
	writeCachedJSON(w, r, map[string]any{"data": map[string]any{"book": book, "chapters": chapters, "progress": progress}}, cacheMaxAgeVolatile)
}

func (h *audiobookHandler) chapters(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	chapters, err := repo.GetChapters(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if chapters == nil {
		chapters = model.AudiobookChapters{}
	}
	writeCachedJSON(w, r, map[string]any{"data": chapters}, cacheMaxAgeStable)
}

func (h *audiobookHandler) stream(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	chapterID := chi.URLParam(r, "chapterId")
	repo := h.ds.Audiobook(r.Context())

	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Audiobook not found", 404)
		return
	}
	chapter, err := repo.GetChapter(chapterID)
	if err != nil {
		http.Error(w, "Chapter not found", 404)
		return
	}
	lib, err := h.ds.Library(r.Context()).Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}
	// Served through the storage abstraction, so a cloud library (openlist://...) streams
	// exactly like a local one: 302 direct link when available, relay otherwise.
	relPath := fspath.Join(book.Path, chapter.Path)
	if err := storage.ServeFile(r.Context(), w, r, lib.Path, relPath); err != nil {
		// 存储网关无响应/超时不能伪装成 404：返回 504 + 明确文案，
		// APP 端才能给出"资源失效/超时"提示，而不是一直转圈假死。
		if isTimeoutError(err) {
			http.Error(w, "资源读取超时：存储网关无响应", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "Not found", 404)
		return
	}
}

// isTimeoutError reports whether err is a network timeout (dead cloud gateway etc.), so the
// stream endpoint can answer 504 instead of a misleading 404.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}

// audiobookLooksFinished reports whether this playback position means the WHOLE book has been
// played: it is in the book's last chapter and the position is at (or past) the end of that chapter.
//
// This is the fallback for clients that do not report `completed` themselves (older APP builds,
// third-party clients). Without it a finished book sits in "继续收听" forever, because nothing ever
// sets the flag. Returning to an earlier chapter immediately un-finishes the book again.
func audiobookLooksFinished(repo model.AudiobookRepository, bookID, chapterID string, reqChapterNumber, position int) bool {
	chapter, err := repo.GetChapter(chapterID)
	if err != nil || chapter.Duration <= 0 {
		return false // 时长未知（快速模式不读标签）：不瞎猜
	}
	last := reqChapterNumber
	if chapter.ChapterNumber > last {
		last = chapter.ChapterNumber
	}
	book, err := repo.Get(bookID)
	if err != nil || book.ChapterCount <= 0 || last < book.ChapterCount {
		return false
	}
	// 5 秒容差：最后一章自然播完时上报位置可能比 duration 略小。
	return position >= chapter.Duration-5
}

func (h *audiobookHandler) getProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	progress, err := repo.GetProgress(usr.ID, bookID)
	if err != nil {
		writeJSON(w, map[string]any{"data": nil})
		return
	}
	writeJSON(w, map[string]any{"data": progress})
}

func (h *audiobookHandler) saveProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())

	var req struct {
		ChapterID     string  `json:"chapterId"`
		ChapterNumber int     `json:"chapterNumber"`
		Position      int     `json:"position"`
		PlaybackSpeed float64 `json:"playbackSpeed"`
		SkipIntro     int     `json:"skipIntro"`
		SkipOutro     int     `json:"skipOutro"`
		// Completed 是可选的：指针区分“客户端没说”和“客户端说了 false”。
		// 不带就按位置推断，带了就以客户端为准。
		Completed *bool `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	// 「已听完」= 书已经全部播放完。继续收听列表要把这些书踢出去，所以这个标记必须
	// 真的被写进库：旧实现根本没接收 completed 字段，列里那句 !completed 过滤永远不生效。
	completed := false
	if req.Completed != nil {
		completed = *req.Completed
	} else {
		completed = audiobookLooksFinished(repo, bookID, req.ChapterID, req.ChapterNumber, req.Position)
	}
	progress := &model.AudiobookProgress{
		UserID:        usr.ID,
		AudiobookID:   bookID,
		ChapterID:     req.ChapterID,
		ChapterNumber: req.ChapterNumber,
		Position:      req.Position,
		PlaybackSpeed: req.PlaybackSpeed,
		SkipIntro:     req.SkipIntro,
		SkipOutro:     req.SkipOutro,
		Completed:     completed,
	}
	if err := repo.SaveProgress(progress); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": progress})
}

func (h *audiobookHandler) getBookmarks(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	bookmarks, err := repo.GetBookmarks(usr.ID, bookID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if bookmarks == nil {
		bookmarks = []model.AudiobookBookmark{}
	}
	writeJSON(w, map[string]any{"data": bookmarks})
}

func (h *audiobookHandler) saveBookmark(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	var req struct {
		ChapterID string `json:"chapterId"`
		Position  int    `json:"position"`
		Title     string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	bookmark := &model.AudiobookBookmark{
		UserID:      usr.ID,
		AudiobookID: bookID,
		ChapterID:   req.ChapterID,
		Position:    req.Position,
		Title:       req.Title,
	}
	if err := repo.SaveBookmark(bookmark); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": bookmark})
}

func (h *audiobookHandler) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	bookmarkID := chi.URLParam(r, "bookmarkId")
	repo := h.ds.Audiobook(r.Context())
	if err := repo.DeleteBookmark(bookmarkID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *audiobookHandler) star(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Star(usr.ID, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *audiobookHandler) unstar(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Unstar(usr.ID, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *audiobookHandler) updateMetadata(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Author      *string `json:"author"`
		Narrator    *string `json:"narrator"`
		Description *string `json:"description"`
		Genre       *string `json:"genre"`
		Year        *int    `json:"year"`
		Series      *string `json:"series"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Title != nil {
		book.Title = *req.Title
	}
	if req.Author != nil {
		book.Author = *req.Author
	}
	if req.Narrator != nil {
		book.Narrator = *req.Narrator
	}
	if req.Description != nil {
		book.Description = *req.Description
	}
	if req.Genre != nil {
		book.Genre = *req.Genre
	}
	if req.Year != nil {
		book.Year = *req.Year
	}
	if req.Series != nil {
		book.Series = *req.Series
	}
	if err := repo.Put(book); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": book})
}

func (h *audiobookHandler) cover(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	// ?dim= 缩略图边长：APP 列表页可请求小图（约省 2/3 流量），不传保持 640 兼容 WEB 管理端。
	dim := maxCoverDimension
	if v := r.URL.Query().Get("dim"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 64 && n <= 1024 {
			dim = n
		}
	}
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	lib, err := h.ds.Library(r.Context()).Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}
	// Local override cover (uploaded/scraped for books in cloud libraries, 评审 §4 P1-2)
	// wins over the library copy.
	if p := audiobookCoverOverride(book.ID); p != "" {
		if data, readErr := os.ReadFile(p); readErr == nil {
			serveCoverSized(w, r, filepath.Base(p), time.Time{}, data, dim)
			return
		}
		noCover(w)
		return
	}
	// Resolve the whole cover chain (library cover file -> embedded audio art -> scraped CoverUrl)
	// ONCE per book and cache it locally; every later request is a pure disk read. This is what
	// makes a shelf of cloud covers load like local files -- the previous code ran a per-request
	// directory listing against the gateway just to find the cover file, so even an already-cached
	// image still cost a round-trip. Keyed on the book row (rotates on rescan) so a swapped cover
	// is picked up on the next scan.
	item := &coverCacheItem{
		keyStr:        bookCoverCacheKey(book.ID, book.UpdatedAt),
		bookDir:       book.Path,
		libPath:       lib.Path,
		coverURL:      book.CoverUrl,
		probeEmbedded: probeEmbeddedArt(lib.Path),
	}
	// The walk behind that cache can cost seconds against a cloud gateway, so it runs as a
	// background job this handler never waits on (see cover_resolver.go). Waiting is what used to
	// let a shelf of covers occupy every browser connection and stall the audiobook detail page
	// behind them.
	res := coverResolverInst.resolve(r.Context(), item.Key(), coverServeBudget, func() ([]byte, bool, error) {
		return resolveCachedCover(context.Background(), getCoverCache(), item)
	})
	if res.ready && res.err == nil && !res.none {
		serveCoverSized(w, r, "cover.jpg", time.Time{}, res.data, dim)
		return
	}
	// Confidently coverless, failed, or still walking: the resolver says how long this miss may be
	// cached, so a slow book settles into instant answers instead of a slow walk per request.
	noCoverIn(w, res.retryIn)
}

// rescan rebuilds a book's chapter list from its folder. The folder is enumerated
// through the storage abstraction (core/storage), so local and cloud libraries share one
// code path.
//
// Safety (评审 §4 P0-1): the new chapter list is built completely BEFORE the database is
// touched. If the folder cannot be read — e.g. the cloud gateway is unreachable — the
// existing chapters are left untouched. The previous implementation deleted all chapters
// first and read the folder afterwards, which wiped a cloud book on every failed rescan.
// Chapters that still exist keep their ID, title and measured duration, so playback
// progress and bookmarks (which reference chapter IDs) survive a rescan.
func (h *audiobookHandler) rescan(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	lib, err := h.ds.Library(r.Context()).Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}

	chapters, err := h.buildRescanChapters(r, repo, book, lib)
	if err != nil {
		// Nothing has been written yet: the old chapters are still intact.
		http.Error(w, err.Error(), 500)
		return
	}
	if err := h.replaceChapters(r, repo, book, chapters); err != nil {
		http.Error(w, "Error saving chapters: "+err.Error(), 500)
		return
	}

	writeJSON(w, map[string]any{"data": map[string]any{"book": book, "chapters": chapters}})
}

// buildRescanChapters enumerates the book folder and builds the new chapter list WITHOUT
// touching the database. Existing chapters are matched by path (chapter.Path is relative
// to the book folder) so their identity and measured values survive the rescan. Only
// genuinely new files get their tags read — which on a cloud source means a bounded Range
// read instead of a full download (design doc §15.1).
func (h *audiobookHandler) buildRescanChapters(r *http.Request, repo model.AudiobookRepository, book *model.Audiobook, lib *model.Library) ([]model.AudiobookChapter, error) {
	fsys, err := storage.FSFor(r.Context(), lib.Path)
	if err != nil {
		return nil, fmt.Errorf("library storage not accessible: %w", err)
	}
	entries, err := fs.ReadDir(fsys, book.Path)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(fspath.Ext(e.Name()))
		if audiobookAudioExts[ext] {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		// Refuse to wipe a book just because the folder looks empty or the extension
		// convention changed — the caller keeps the existing chapters.
		return nil, fmt.Errorf("no audio files found in %q, keeping existing chapters", book.Path)
	}
	sort.Strings(names)

	byPath := map[string]model.AudiobookChapter{}
	if existing, err := repo.GetChapters(book.ID); err == nil {
		for _, ch := range existing {
			byPath[ch.Path] = ch
		}
	}

	chapters := make([]model.AudiobookChapter, 0, len(names))
	for i, name := range names {
		chapterPath := fspath.Join(book.Path, name)
		chapterTitle := strings.TrimSuffix(name, fspath.Ext(name))
		format := strings.TrimPrefix(strings.ToLower(fspath.Ext(name)), ".")
		var fileSize int64
		if info, err := fs.Stat(fsys, chapterPath); err == nil {
			fileSize = info.Size()
		}

		chapter := model.AudiobookChapter{
			ID:            id.NewRandom(),
			AudiobookID:   book.ID,
			Title:         chapterTitle,
			ChapterNumber: i + 1,
			Format:        format,
			FileSize:      fileSize,
			Path:          name,
			CreatedAt:     time.Now(),
		}
		if old, known := byPath[name]; known {
			// Same file as before: keep identity and measured values. This preserves
			// progress/bookmark references and avoids re-reading tags from the cloud.
			chapter.ID = old.ID
			chapter.Title = old.Title
			chapter.Duration = old.Duration
			chapter.CreatedAt = old.CreatedAt
		} else if cloudsource.TagModeForLibrary(lib.Path) == "filename" {
			// 评审 P2-11 快速模式：新文件也不读内容——标题用文件名、时长留空。
		} else if rs, closer, err := openChapterForTag(fsys, chapterPath); err == nil {
			if f, err := taglib.OpenStream(rs, taglib.WithReadStyle(taglib.ReadStyleFast), taglib.WithFilename(chapterPath)); err == nil {
				allTags := f.AllTags()
				props := f.Properties()
				f.Close()
				if v, ok := allTags.Tags["TITLE"]; ok && len(v) > 0 && v[0] != "" {
					chapter.Title = v[0]
				}
				if props.Length > 0 {
					chapter.Duration = int(props.Length.Seconds())
				}
			}
			_ = closer.Close()
		}
		chapters = append(chapters, chapter)
	}
	return chapters, nil
}

// openChapterForTag opens a file through the library FS as a seekable reader for taglib.
// On a cloud source the handle is a lazy Range reader (same contract as the scanner's
// openForTag helper).
func openChapterForTag(fsys storage.MusicFS, filePath string) (io.ReadSeeker, io.Closer, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		_ = f.Close()
		return nil, nil, fmt.Errorf("audiobook: %s is not seekable", filePath)
	}
	return rs, f, nil
}

// replaceChapters swaps the chapter list of a book and refreshes its stats. Only called
// after buildRescanChapters has produced a complete new list.
func (h *audiobookHandler) replaceChapters(r *http.Request, repo model.AudiobookRepository, book *model.Audiobook, chapters []model.AudiobookChapter) error {
	if err := repo.DeleteChapters(book.ID); err != nil {
		return err
	}
	var totalSize int64
	var totalDuration int
	for i := range chapters {
		if err := repo.PutChapter(&chapters[i]); err != nil {
			log.Error(r.Context(), "Rescan: Error saving chapter", "chapter", chapters[i].Title, err)
			return err
		}
		totalSize += chapters[i].FileSize
		totalDuration += chapters[i].Duration
	}
	book.ChapterCount = len(chapters)
	book.Size = totalSize
	book.TotalDuration = totalDuration
	return repo.Put(book)
}

// rescanAll rescans all audiobooks that have 0 chapters
func (h *audiobookHandler) rescanAll(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	rescanned := 0
	skipped := 0
	failed := 0
	for i := range books {
		book := &books[i]
		// Only rescan books with 0 chapters
		if book.ChapterCount > 0 {
			skipped++
			continue
		}
		lib, libErr := h.ds.Library(r.Context()).Get(book.LibraryID)
		if libErr != nil {
			failed++
			continue
		}

		// Build the new list first (storage abstraction, local + cloud). On failure the
		// book is counted as failed and its chapters are left untouched.
		chapters, buildErr := h.buildRescanChapters(r, repo, book, lib)
		if buildErr != nil {
			log.Warn(r.Context(), "RescanAll: cannot rescan book, keeping existing chapters", "book", book.Title, buildErr)
			failed++
			continue
		}
		if err := h.replaceChapters(r, repo, book, chapters); err != nil {
			log.Error(r.Context(), "RescanAll: Error saving chapters", "book", book.Title, err)
			failed++
			continue
		}
		rescanned++
	}

	writeJSON(w, map[string]any{"data": map[string]any{
		"rescanned": rescanned,
		"skipped":   skipped,
		"failed":    failed,
		"total":     len(books),
	}})
}

// uploadCover handles audiobook cover image upload (file upload or URL download)
func (h *audiobookHandler) uploadCover(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Audiobook not found", 404)
		return
	}
	lib, err := h.ds.Library(r.Context()).Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}
	// Check if URL-based upload
	imageURL := r.FormValue("url")
	var imageData []byte
	var ext string

	if imageURL != "" {
		// Download image from URL (SSRF-hardened, see image_url_guard.go)
		data, contentType, err := fetchRemoteImage(r.Context(), imageURL)
		if err != nil {
			http.Error(w, "Failed to download image: "+err.Error(), 400)
			return
		}
		imageData = data
		ext = extFromContentType(contentType)
	} else {
		// File upload
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file or url provided", 400)
			return
		}
		defer file.Close()
		imageData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file", 500)
			return
		}
		ext = strings.ToLower(filepath.Ext(header.Filename))
	}

	// Validate image type
	validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	if !validExts[ext] {
		http.Error(w, "Unsupported image type: "+ext, 400)
		return
	}

	// Save the cover: next to the audio files for local libraries (unchanged behavior),
	// into the local override directory for cloud/read-only libraries (评审 §4 P1-2).
	relCover, err := saveAudiobookCover(book, lib, imageData, ext)
	if err != nil {
		http.Error(w, "Failed to save cover: "+err.Error(), 500)
		return
	}
	book.CoverPath = relCover
	_ = repo.Put(book)

	writeJSON(w, map[string]any{"data": map[string]any{"coverPath": relCover}})
}

// uploadNarratorAvatar handles narrator avatar upload
// Narrator avatars are stored in data/narrator-avatars/<sanitized-name>.<ext>
func (h *audiobookHandler) uploadNarratorAvatar(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Narrator name required", 400)
		return
	}

	avatarDir := filepath.Join(conf.Server.DataFolder.String(), "narrator-avatars")
	os.MkdirAll(avatarDir, 0755)

	// Sanitize filename
	safeName := strings.ReplaceAll(name, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	safeName = strings.ReplaceAll(safeName, "..", "_")

	var imageData []byte
	var ext string

	// Check if URL-based upload
	imageURL := r.FormValue("url")
	if imageURL != "" {
		// SSRF-hardened fetch, see image_url_guard.go
		data, contentType, err := fetchRemoteImage(r.Context(), imageURL)
		if err != nil {
			http.Error(w, "Failed to download image: "+err.Error(), 400)
			return
		}
		imageData = data
		ext = extFromContentType(contentType)
	} else {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file or url provided", 400)
			return
		}
		defer file.Close()
		imageData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file", 500)
			return
		}
		ext = strings.ToLower(filepath.Ext(header.Filename))
	}

	// Remove old avatar files
	for _, oldExt := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		os.Remove(filepath.Join(avatarDir, safeName+oldExt))
	}

	// Save new avatar
	avatarPath := filepath.Join(avatarDir, safeName+ext)
	if err := os.WriteFile(avatarPath, imageData, 0644); err != nil {
		http.Error(w, "Failed to save avatar: "+err.Error(), 500)
		return
	}

	writeJSON(w, map[string]any{"data": map[string]any{"path": "/api/audiobook/narrator/" + name + "/avatar"}})
}

// getNarratorAvatar serves narrator avatar images
func (h *audiobookHandler) getNarratorAvatar(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	safeName := strings.ReplaceAll(name, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")

	avatarDir := filepath.Join(conf.Server.DataFolder.String(), "narrator-avatars")
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		path := filepath.Join(avatarDir, safeName+ext)
		if _, err := os.Stat(path); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeFile(w, r, path)
			return
		}
	}
	http.Error(w, "No avatar found", 404)
}

func extFromContentType(contentType string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}

const (
	cacheMaxAgeVolatile = 15 // 含进度等易变数据
	cacheMaxAgeStable   = 60 // 章节/分类/演播者等基本不变
)

// writeCachedJSON serves a JSON payload with client caching (short freshness + ETag
// revalidation).
//
// 为什么加缓存：进入有声书首页/详情每次都要重新拉数据，在慢网络上整包 JSON 的传输
// 就是"Loading 转圈"的主要时间。给这些只读接口加上短新鲜期后，短时间内重复进入
// 直接命中客户端缓存（零网络），过期后带 If-None-Match 走 304，不再重复传整包 JSON。
func writeCachedJSON(w http.ResponseWriter, r *http.Request, data any, maxAgeSeconds int) {
	buf, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sum := sha256.Sum256(buf)
	etag := "W/\"" + hex.EncodeToString(sum[:8]) + "\""
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d, must-revalidate", maxAgeSeconds))
	w.Header().Set("ETag", etag)
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buf)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// purgeMissing removes audiobook rows whose folder no longer exists on disk / in the cloud —
// immediately, without waiting for a scan. It is intentionally conservative: a book is removed
// only when its directory is definitively missing (ENOENT); permission / network / gateway errors
// keep the book (a transient outage must never wipe the library). dryRun=1 reports what would
// happen without deleting. The response lists a per-book decision so admins can audit exactly why
// each book was kept or removed.
func (h *audiobookHandler) purgeMissing(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == "1"
	ctx := r.Context()
	books, err := h.ds.Audiobook(ctx).GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type result struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Path   string `json:"path"`
		Action string `json:"action"` // "deleted" | "would-delete" | "kept"
		Reason string `json:"reason"`
	}
	mk := func(id, title, path, action, reason string) result {
		return result{ID: id, Title: title, Path: path, Action: action, Reason: reason}
	}
	results := make([]result, 0, len(books))
	fsCache := map[int]storage.MusicFS{}
	deleted := 0
	for _, book := range books {
		fsys, ok := fsCache[book.LibraryID]
		if !ok {
			lib, lerr := h.ds.Library(ctx).Get(book.LibraryID)
			if lerr != nil {
				results = append(results, mk(book.ID, book.Title, book.Path, "kept", "library not found"))
				continue
			}
			fsys, _ = storage.FSFor(ctx, lib.Path)
			fsCache[book.LibraryID] = fsys
		}
		if fsys == nil {
			results = append(results, mk(book.ID, book.Title, book.Path, "kept", "library storage not accessible"))
			continue
		}
		_, serr := fs.Stat(fsys, book.Path)
		switch {
		case serr == nil:
			results = append(results, mk(book.ID, book.Title, book.Path, "kept", "folder still present"))
		case errors.Is(serr, fs.ErrNotExist):
			if dryRun {
				results = append(results, mk(book.ID, book.Title, book.Path, "would-delete", "folder missing"))
				continue
			}
			if derr := h.ds.Audiobook(ctx).DeleteWithRelations(book.ID); derr != nil {
				results = append(results, mk(book.ID, book.Title, book.Path, "kept", "delete failed: "+derr.Error()))
				continue
			}
			deleted++
			results = append(results, mk(book.ID, book.Title, book.Path, "deleted", "folder missing"))
		default:
			results = append(results, mk(book.ID, book.Title, book.Path, "kept", "unreachable: "+serr.Error()))
		}
	}
	writeJSON(w, map[string]any{
		"dryRun":  dryRun,
		"total":   len(books),
		"deleted": deleted,
		"data":    results,
	})
}

// [LeChenMusic-END:audiobook]
