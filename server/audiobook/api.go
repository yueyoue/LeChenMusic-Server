package audiobook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
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
	r.Use(server.JWTRefresher)
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

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) listAudiobooks(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	genre := r.URL.Query().Get("genre")
	narrator := r.URL.Query().Get("narrator")
	if genre == "" && narrator == "" {
		writeJSON(w, map[string]any{"data": books})
		return
	}

	var result model.Audiobooks
	for _, b := range books {
		if genre != "" && b.Genre != genre {
			continue
		}
		if narrator != "" && b.Narrator != narrator {
			continue
		}
		result = append(result, b)
	}
	writeJSON(w, map[string]any{"data": result})
}

func (h *Handler) listGenres(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type genreInfo struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	genreMap := map[string]int{}
	for _, b := range books {
		g := b.Genre
		if g == "" {
			g = "有声读物"
		}
		genreMap[g]++
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
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetStarred(user.ID)
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

	user, ok := getUser(r)
	var progress *model.AudiobookProgress
	if ok {
		progress, _ = repo.GetProgress(user.ID, id)
	}

	var isStarred bool
	if ok {
		isStarred, _ = repo.IsStarred(user.ID, id)
	}

	writeJSON(w, map[string]any{
		"data": map[string]any{
			"book":      book,
			"chapters":  chapters,
			"progress":  progress,
			"starred":   isStarred,
		},
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

	// Find the library to resolve the full path
	libRepo := h.ds.Library(r.Context())
	lib, err := libRepo.Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}

	fullPath := filepath.Join(lib.Path, book.Path, chapter.Path)
	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "File not found", 404)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Cannot stat file", 500)
		return
	}

	contentType := "audio/mpeg"
	switch chapter.Format {
	case "flac":
		contentType = "audio/flac"
	case "ogg", "opus":
		contentType = "audio/ogg"
	case "m4a", "m4b", "aac":
		contentType = "audio/mp4"
	case "wav":
		contentType = "audio/wav"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, chapter.Title, stat.ModTime(), file)
}

func (h *Handler) getProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	progress, err := repo.GetProgress(user.ID, bookID)
	if err != nil {
		writeJSON(w, map[string]any{"data": nil})
		return
	}
	writeJSON(w, map[string]any{"data": progress})
}

func (h *Handler) saveProgress(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}

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

	repo := h.ds.Audiobook(r.Context())
	existing, _ := repo.GetProgress(user.ID, bookID)

	if existing != nil {
		existing.ChapterID = req.ChapterID
		existing.ChapterNumber = req.ChapterNumber
		existing.Position = req.Position
		if req.PlaybackSpeed > 0 {
			existing.PlaybackSpeed = req.PlaybackSpeed
		}
		existing.SkipIntro = req.SkipIntro
		existing.SkipOutro = req.SkipOutro
		if err := repo.SaveProgress(existing); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		progress := &model.AudiobookProgress{
			UserID:        user.ID,
			AudiobookID:   bookID,
			ChapterID:     req.ChapterID,
			ChapterNumber: req.ChapterNumber,
			Position:      req.Position,
			PlaybackSpeed: req.PlaybackSpeed,
			SkipIntro:     req.SkipIntro,
			SkipOutro:     req.SkipOutro,
		}
		if progress.PlaybackSpeed == 0 {
			progress.PlaybackSpeed = 1.0
		}
		if err := repo.SaveProgress(progress); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, map[string]any{"data": "ok"})
}

func (h *Handler) getBookmarks(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	bookmarks, err := repo.GetBookmarks(user.ID, bookID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": bookmarks})
}

func (h *Handler) saveBookmark(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}

	var req struct {
		ChapterID string `json:"chapterId"`
		Position  int    `json:"position"`
		Title     string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}

	repo := h.ds.Audiobook(r.Context())
	bookmark := &model.AudiobookBookmark{
		UserID:      user.ID,
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
	writeJSON(w, map[string]any{"data": "ok"})
}

func (h *Handler) star(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Star(user.ID, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": "ok"})
}

func (h *Handler) unstar(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	user, ok := getUser(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	repo := h.ds.Audiobook(r.Context())
	if err := repo.Unstar(user.ID, bookID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": "ok"})
}

func (h *Handler) getCover(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}

	if book.CoverPath == "" {
		http.Error(w, "No cover", 404)
		return
	}

	libRepo := h.ds.Library(r.Context())
	lib, err := libRepo.Get(book.LibraryID)
	if err != nil {
		http.Error(w, "Library not found", 404)
		return
	}

	coverPath := filepath.Join(lib.Path, book.CoverPath)
	file, err := os.Open(coverPath)
	if err != nil {
		http.Error(w, "Cover not found", 404)
		return
	}
	defer file.Close()

	stat, _ := file.Stat()
	ext := strings.ToLower(filepath.Ext(coverPath))
	contentType := "image/jpeg"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, file)
}

func getUser(r *http.Request) (*model.User, bool) {
	ctx := r.Context()
	u, ok := ctx.Value("user").(*model.User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// [LeChenMusic-END:audiobook]
