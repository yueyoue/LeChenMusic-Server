package nativeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

		// Delete existing chapters (should be none, but just in case)
		_ = repo.DeleteChapters(book.ID)

		// Rescan chapters from filesystem
		bookPath := filepath.Join(lib.Path, book.Path)
		entries, readErr := os.ReadDir(bookPath)
		if readErr != nil {
			failed++
			continue
		}

		var audioFiles []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if audiobookAudioExts[ext] {
				audioFiles = append(audioFiles, e.Name())
			}
		}
		sort.Strings(audioFiles)

		var totalSize int64
		for j, fname := range audioFiles {
			chapterPath := filepath.Join(bookPath, fname)
			chapterTitle := strings.TrimSuffix(fname, filepath.Ext(fname))
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(fname)), ".")
			var fileSize int64
			if info, err := os.Stat(chapterPath); err == nil {
				fileSize = info.Size()
			}

			chapter := model.AudiobookChapter{
				ID:            fname,
				AudiobookID:   book.ID,
				Title:         chapterTitle,
				ChapterNumber: j + 1,
				Duration:      0,
				Format:        format,
				FileSize:      fileSize,
				Path:          fname,
			}
			if err := repo.PutChapter(&chapter); err != nil {
				log.Error(r.Context(), "RescanAll: Error saving chapter", "book", book.Title, "chapter", chapterTitle, err)
			}
			totalSize += fileSize
		}

		book.ChapterCount = len(audioFiles)
		book.Size = totalSize
		_ = repo.Put(book)
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

	avatarDir := filepath.Join("data", "narrator-avatars")
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

	avatarDir := filepath.Join("data", "narrator-avatars")
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

