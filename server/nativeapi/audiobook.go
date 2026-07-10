package nativeapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
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
		r.Post("/{id}/rescan", h.rescan)
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
	// Populate starred status for current user
	usr, ok := request.UserFrom(r.Context())
	if ok {
		starred, _ := repo.IsStarred(usr.ID, id)
		book.Starred = starred
	}
	chapters, _ := repo.GetChapters(id)
	writeJSON(w, map[string]any{"data": map[string]any{"book": book, "chapters": chapters}})
}

func (h *audiobookHandler) chapters(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	chapters, err := repo.GetChapters(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
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
	filePath := filepath.Join(lib.Path, book.Path, chapter.Path)
	http.ServeFile(w, r, filePath)
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
	bookPath := filepath.Join(lib.Path, book.Path)
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.jpeg", "folder.png"} {
		coverPath := filepath.Join(bookPath, name)
		if _, err := os.Stat(coverPath); err == nil {
			// Set cache headers for cover images
			w.Header().Set("Cache-Control", "public, max-age=3600")
			http.ServeFile(w, r, coverPath)
			return
		}
	}
	http.Error(w, "No cover found", 404)
}

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

	// Delete existing chapters
	_ = repo.DeleteChapters(bookID)

	// Rescan chapters from filesystem
	bookPath := filepath.Join(lib.Path, book.Path)
	entries, readErr := os.ReadDir(bookPath)
	if readErr != nil {
		http.Error(w, "Cannot read directory: "+readErr.Error(), 500)
		return
	}

	type audioFile struct {
		name string
		path string
	}
	var audioFiles []audioFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if audiobookAudioExts[ext] {
			audioFiles = append(audioFiles, audioFile{name: e.Name(), path: e.Name()})
		}
	}

	sort.Slice(audioFiles, func(i, j int) bool {
		return audioFiles[i].name < audioFiles[j].name
	})

	var totalSize int64
	var chapters []model.AudiobookChapter
	for i, af := range audioFiles {
		chapterPath := filepath.Join(bookPath, af.name)
		chapterTitle := strings.TrimSuffix(af.name, filepath.Ext(af.name))
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(af.name)), ".")
		var fileSize int64
		if info, err := os.Stat(chapterPath); err == nil {
			fileSize = info.Size()
		}

		chapter := model.AudiobookChapter{
			ID:            af.name, // Use filename as ID for simplicity
			AudiobookID:   bookID,
			Title:         chapterTitle,
			ChapterNumber: i + 1,
			Duration:      0,
			Format:        format,
			FileSize:      fileSize,
			Path:          af.path,
		}
		if err := repo.PutChapter(&chapter); err != nil {
			log.Error(r.Context(), "Rescan: Error saving chapter", "chapter", chapter.Title, err)
		}
		totalSize += fileSize
		chapters = append(chapters, chapter)
	}

	// Update book stats
	book.ChapterCount = len(audioFiles)
	book.Size = totalSize
	_ = repo.Put(book)

	writeJSON(w, map[string]any{"data": map[string]any{"book": book, "chapters": chapters}})
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// [LeChenMusic-END:audiobook]

