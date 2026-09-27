package nativeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	fspath "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
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

var audiobookCoverNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png",
	"folder.jpg", "folder.jpeg", "folder.png",
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
		r.Post("/rescan-all", h.rescanAll) // Batch rescan all audiobooks
		r.Post("/narrator/{name}/avatar", h.uploadNarratorAvatar) // Upload narrator avatar
		r.Get("/narrator/{name}/avatar", h.getNarratorAvatar) // Serve narrator avatar
	})
}

type audiobookHandler struct {
	ds model.DataStore
}

func (h *audiobookHandler) list(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if books == nil { books = model.Audiobooks{} }
	writeJSON(w, map[string]any{"data": books})
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
	if books == nil { books = model.Audiobooks{} }
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
	writeJSON(w, map[string]any{"data": result})
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
		writeJSON(w, map[string]any{"data": []any{}})
		return
	}
	type bookWithProgress struct {
		model.Audiobook
		Progress *model.AudiobookProgress `json:"progress"`
	}
	var result []bookWithProgress
	for _, p := range progressList {
		book, err := repo.Get(p.AudiobookID)
		if err != nil {
			continue
		}
		chapters, _ := repo.GetChapters(book.ID)
		if chapters == nil {
			chapters = model.AudiobookChapters{}
		}
		book.ChapterCount = len(chapters)
		pCopy := p
		result = append(result, bookWithProgress{
			Audiobook: *book,
			Progress:  &pCopy,
		})
	}
	writeJSON(w, map[string]any{"data": result})
}

func (h *audiobookHandler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, map[string]any{"data": []any{}})
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
	writeJSON(w, map[string]any{"data": results})
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
	writeJSON(w, map[string]any{"data": genres})
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
	writeJSON(w, map[string]any{"data": narrators})
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
	writeJSON(w, map[string]any{"data": map[string]any{"name": name, "works": works}})
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
	if books == nil { books = model.Audiobooks{} }
	// Populate starred timestamp for each book
	for i := range books {
		starredAt, _ := repo.GetStarredAt(usr.ID, books[i].ID)
		if starredAt != "" {
			books[i].Starred = starredAt
		}
	}
	writeJSON(w, map[string]any{"data": books})
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
		log.Info(r.Context(), "GetStarredAt result", "userID", usr.ID, "bookID", id, "starredAt", starredAt, "error", starredErr)
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
	writeJSON(w, map[string]any{"data": map[string]any{"book": book, "chapters": chapters, "progress": progress}})
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
	writeJSON(w, map[string]any{"data": chapters})
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
		http.Error(w, "Not found", 404)
		return
	}
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
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
	// Cover lookup goes through the storage abstraction so cloud libraries work too. On a
	// cloud source this costs at most one cached directory listing, and the image itself is
	// served as a 302 direct link whenever the gateway can hand one out.
	fsys, err := storage.FSFor(r.Context(), lib.Path)
	if err != nil {
		http.Error(w, "Library not accessible", 500)
		return
	}
	for _, name := range audiobookCoverNames {
		relCover := fspath.Join(book.Path, name)
		if _, err := fs.Stat(fsys, relCover); err != nil {
			continue
		}
		// Set cache headers for cover images
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if err := storage.ServeFile(r.Context(), w, r, lib.Path, relCover); err != nil {
			http.Error(w, "No cover found", 404)
		}
		return
	}
	// [LeChenMusic-START:audiobook-cover-fallback]
	// 本地没有封面文件时，从数据库中的cover_url代理获取
	if book.CoverUrl != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequest("GET", book.CoverUrl, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == 200 {
				defer resp.Body.Close()
				w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
				w.Header().Set("Cache-Control", "public, max-age=86400")
				io.Copy(w, resp.Body)
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
	// [LeChenMusic-END:audiobook-cover-fallback]
	http.Error(w, "No cover found", 404)
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
	bookPath := filepath.Join(lib.Path, book.Path)

	// Check if URL-based upload
	imageURL := r.FormValue("url")
	var imageData []byte
	var ext string

	if imageURL != "" {
		// Download image from URL
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(imageURL)
		if err != nil {
			http.Error(w, "Failed to download image: "+err.Error(), 400)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			http.Error(w, "Failed to download image: HTTP "+resp.Status, 400)
			return
		}
		imageData, err = io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Failed to read image data", 500)
			return
		}
		contentType := resp.Header.Get("Content-Type")
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

	// Remove old cover files
	for _, name := range audiobookCoverNames {
		oldPath := filepath.Join(bookPath, name)
		os.Remove(oldPath)
	}

	// Save new cover
	coverName := "cover" + ext
	coverPath := filepath.Join(bookPath, coverName)
	if err := os.WriteFile(coverPath, imageData, 0644); err != nil {
		http.Error(w, "Failed to save cover: "+err.Error(), 500)
		return
	}

	// Update book's coverPath
	relCover, _ := filepath.Rel(lib.Path, coverPath)
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
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(imageURL)
		if err != nil {
			http.Error(w, "Failed to download image: "+err.Error(), 400)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			http.Error(w, "Failed to download image: HTTP "+resp.Status, 400)
			return
		}
		imageData, err = io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Failed to read image data", 500)
			return
		}
		contentType := resp.Header.Get("Content-Type")
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

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// [LeChenMusic-END:audiobook]

