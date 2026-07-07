package audiobook

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

// [LeChenMusic-START:audiobook]

type Handler struct {
	ds model.DataStore
}

func NewHandler(ds model.DataStore) *Handler {
	return &Handler{ds: ds}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.listAudiobooks)
	r.Get("/genres", h.listGenres)
	r.Get("/narrators", h.listNarrators)
	r.Get("/starred", h.listStarred)
	r.Get("/{id}", h.getAudiobook)
	r.Get("/{id}/chapters", h.getChapters)
	r.Get("/{id}/chapters/{chapterId}/stream", h.streamChapter)
	r.Get("/{id}/progress", h.getProgress)
	r.Put("/{id}/progress", h.saveProgress)
	r.Get("/{id}/bookmarks", h.getBookmarks)
	r.Post("/{id}/bookmarks", h.saveBookmark)
	r.Delete("/{id}/bookmarks/{bookmarkId}", h.deleteBookmark)
	r.Post("/{id}/star", h.star)
	r.Delete("/{id}/star", h.unstar)
	r.Get("/{id}/cover", h.getCover)
	return r
}

func (h *Handler) listAudiobooks(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": books})
}

func (h *Handler) listGenres(w http.ResponseWriter, r *http.Request) {
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
	type genreInfo struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var genres []genreInfo
	for name, count := range genreMap {
		genres = append(genres, genreInfo{Name: name, Count: count})
	}
	writeJSON(w, map[string]any{"data": genres})
}

func (h *Handler) listNarrators(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type narratorInfo struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	narratorMap := map[string]int{}
	for _, b := range books {
		if b.Narrator != "" {
			narratorMap[b.Narrator]++
		}
	}
	var narrators []narratorInfo
	for name, count := range narratorMap {
		narrators = append(narrators, narratorInfo{Name: name, Count: count})
	}
	writeJSON(w, map[string]any{"data": narrators})
}

func (h *Handler) listStarred(w http.ResponseWriter, r *http.Request) {
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetStarred(user)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": books})
}

func (h *Handler) getAudiobook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(id)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	chapters, _ := repo.GetChapters(id)
	writeJSON(w, map[string]any{
		"data": map[string]any{"book": book, "chapters": chapters},
	})
}

func (h *Handler) getChapters(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	chapters, err := repo.GetChapters(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": chapters})
}

func (h *Handler) streamChapter(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) getProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
	repo := h.ds.Audiobook(r.Context())
	progress, err := repo.GetProgress(user, bookID)
	if err != nil {
		writeJSON(w, map[string]any{"data": nil})
		return
	}
	writeJSON(w, map[string]any{"data": progress})
}

func (h *Handler) saveProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
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
		UserID:        user,
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

func (h *Handler) getBookmarks(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
	repo := h.ds.Audiobook(r.Context())
	bookmarks, err := repo.GetBookmarks(user, bookID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": bookmarks})
}

func (h *Handler) saveBookmark(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
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
		UserID:      user,
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

func (h *Handler) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	bookmarkID := chi.URLParam(r, "bookmarkId")
	repo := h.ds.Audiobook(r.Context())
	if err := repo.DeleteBookmark(bookmarkID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *Handler) star(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Star(user, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *Handler) unstar(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	usr, ok := request.UserFrom(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}
	user := usr.UserName
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Unstar(user, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (h *Handler) getCover(w http.ResponseWriter, r *http.Request) {
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

	// Find cover file
	bookPath := filepath.Join(lib.Path, book.Path)
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.jpeg", "folder.png"} {
		coverPath := filepath.Join(bookPath, name)
		if _, err := os.Stat(coverPath); err == nil {
			http.ServeFile(w, r, coverPath)
			return
		}
	}
	http.Error(w, "No cover found", 404)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// MountAudiobookRoutes mounts the audiobook API routes
func MountAudiobookRoutes(r chi.Router, ds model.DataStore) {
	handler := NewHandler(ds)
	r.Mount("/api/audiobook", withAuth(handler.Routes()))
}

func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use Navidrome's auth middleware
		next.ServeHTTP(w, r)
	})
}

// [LeChenMusic-END:audiobook]
