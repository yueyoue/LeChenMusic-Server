package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/scraper"
)

func (api *Router) addScrapeRoute(r chi.Router) {
	h := &scrapeHandler{ds: api.ds}
	r.Route("/scrape", func(r chi.Router) {
		r.Get("/audiobook", h.searchAudiobooks)
		r.Get("/audiobook/detail", h.getAudiobookDetail)
		r.Get("/sources", h.getSources)
		r.Post("/audiobook/{id}/apply", h.applyScrape)
		r.Get("/artist", h.searchArtists)
		r.Post("/artist/{id}/avatar", h.applyArtistAvatar)
		r.Post("/batch", h.batchScrape)
		// Serve saved images (narrator avatars, etc.)
		r.Get("/image/{type}/{id}", h.serveImage)
	})
}

type scrapeHandler struct {
	ds model.DataStore
}

func (h *scrapeHandler) getSources(w http.ResponseWriter, r *http.Request) {
	sources := scraper.GetAll()
	type sourceInfo struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}
	var result []sourceInfo
	for _, s := range sources {
		result = append(result, sourceInfo{Name: s.Name(), DisplayName: s.DisplayName()})
	}
	writeJSON(w, map[string]any{"data": result})
}

func (h *scrapeHandler) searchAudiobooks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "q parameter required", 400)
		return
	}
	sourcesParam := r.URL.Query().Get("sources")
	var sources []string
	if sourcesParam != "" {
		sources = strings.Split(sourcesParam, ",")
	}
	allSources := scraper.GetAll()
	type srcResult struct {
		Source string                 `json:"source"`
		Name   string                 `json:"name"`
		Items  []scraper.ScrapeResult `json:"items"`
	}
	var results []srcResult
	for _, s := range allSources {
		if len(sources) > 0 {
			found := false
			for _, src := range sources {
				if src == s.Name() {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		res, err := s.SearchAudiobooks(query, 1)
		if err != nil || len(res) == 0 {
			continue
		}
		results = append(results, srcResult{Source: s.Name(), Name: s.DisplayName(), Items: res})
	}
	writeJSON(w, map[string]any{"data": results})
}

func (h *scrapeHandler) getAudiobookDetail(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	id := r.URL.Query().Get("id")
	if source == "" || id == "" {
		http.Error(w, "source and id required", 400)
		return
	}
	s, ok := scraper.Get(source)
	if !ok {
		http.Error(w, "Unknown source: "+source, 400)
		return
	}
	detail, err := s.GetAudiobookDetail(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": detail})
}

func (h *scrapeHandler) applyScrape(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	repo := h.ds.Audiobook(r.Context())
	book, err := repo.Get(bookID)
	if err != nil {
		http.Error(w, "Audiobook not found", 404)
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Author      *string `json:"author"`
		Narrator    *string `json:"narrator"`
		Description *string `json:"description"`
		Genre       *string `json:"genre"`
		CoverURL    *string `json:"coverUrl"`
		Source      string  `json:"source"`      // 刮削源名称，用于封面URL失效时重新获取
		SourceID    string  `json:"sourceId"`    // 刮削源上的ID
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
	if req.CoverURL != nil && *req.CoverURL != "" {
		lib, libErr := h.ds.Library(r.Context()).Get(book.LibraryID)
		if libErr == nil {
			bookPath := filepath.Join(lib.Path, book.Path)
			dlErr := downloadCover(*req.CoverURL, bookPath, book, *lib)
			if dlErr != nil {
				log.Warn(r.Context(), "Cover download failed, will try to re-scrape", "error", dlErr, "url", *req.CoverURL)
				// 尝试重新从刮削源获取新鲜的封面URL并重试
				freshURL := h.refreshCoverURL(r.Context(), req.Source, req.SourceID, book.Title)
				if freshURL != "" && freshURL != *req.CoverURL {
					log.Info(r.Context(), "Retrying cover download with fresh URL", "url", freshURL)
					dlErr = downloadCover(freshURL, bookPath, book, *lib)
					if dlErr != nil {
						log.Error(r.Context(), "Cover retry also failed", "error", dlErr, "url", freshURL)
					}
				} else {
					log.Warn(r.Context(), "Could not get fresh cover URL from scraper")
				}
			}
		} else {
			log.Error(r.Context(), "Failed to get library for cover download", "error", libErr)
		}
	}
	if err := repo.Put(book); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": book})
}

// refreshCoverURL 尝试从刮削源重新获取封面URL
// 当原始封面URL过期（如喜马拉雅CDN URL含时效token）时调用
func (h *scrapeHandler) refreshCoverURL(ctx context.Context, source, sourceID, bookTitle string) string {
	if source == "" {
		return ""
	}
	s, ok := scraper.Get(source)
	if !ok {
		return ""
	}
	// 优先使用sourceID获取详情
	if sourceID != "" {
		detail, err := s.GetAudiobookDetail(sourceID)
		if err == nil && detail.CoverURL != "" {
			return detail.CoverURL
		}
	}
	// 降级：用书名重新搜索
	if bookTitle != "" {
		results, err := s.SearchAudiobooks(bookTitle, 1)
		if err == nil {
			for _, r := range results {
				if r.CoverURL != "" {
					return r.CoverURL
				}
			}
		}
	}
	return ""
}

func (h *scrapeHandler) searchArtists(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "q parameter required", 400)
		return
	}
	// type=narrator searches only audiobook platforms (ximalaya, qingting)
	searchType := r.URL.Query().Get("type")
	var results []scraper.ArtistResult
	if searchType == "narrator" {
		results = scraper.SearchNarratorsAll(query)
	} else {
		results = scraper.SearchArtistsAll(query)
	}
	writeJSON(w, map[string]any{"data": results})
}

func (h *scrapeHandler) applyArtistAvatar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	log.Info(r.Context(), "applyArtistAvatar called", "id", id)

	var req struct {
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error(r.Context(), "Failed to decode request", err)
		http.Error(w, "invalid request", 400)
		return
	}
	log.Info(r.Context(), "applyArtistAvatar request", "imageUrl", req.ImageURL)

	if req.ImageURL == "" {
		http.Error(w, "imageUrl required", 400)
		return
	}

	// Download image
	log.Info(r.Context(), "Downloading image", "url", req.ImageURL)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(req.ImageURL)
	if err != nil {
		log.Error(r.Context(), "Failed to download image", err)
		http.Error(w, "Failed to download image: "+err.Error(), 400)
		return
	}
	defer resp.Body.Close()
	log.Info(r.Context(), "Image downloaded", "status", resp.StatusCode, "contentType", resp.Header.Get("Content-Type"))

	if resp.StatusCode != 200 {
		http.Error(w, "Failed to download image: HTTP "+resp.Status, 400)
		return
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(r.Context(), "Failed to read image", err)
		http.Error(w, "Failed to read image", 500)
		return
	}
	log.Info(r.Context(), "Image data read", "size", len(imageData))

	// Determine extension
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}

	// Determine save location based on ID prefix
	var imageURL string
	if strings.HasPrefix(id, "narrator-") {
		// Narrator avatar
		narratorName := strings.TrimPrefix(id, "narrator-")
		avatarDir := filepath.Join("data", "narrator-avatars")
		if mkErr := os.MkdirAll(avatarDir, 0755); mkErr != nil {
			log.Error(r.Context(), "Failed to create dir", mkErr)
		}

		// Sanitize filename
		safeName := strings.ReplaceAll(narratorName, "/", "_")
		safeName = strings.ReplaceAll(safeName, "\\", "_")
		safeName = strings.ReplaceAll(safeName, "..", "_")

		// Remove old files
		for _, oldExt := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			os.Remove(filepath.Join(avatarDir, safeName+oldExt))
		}

		// Save new file
		savePath := filepath.Join(avatarDir, safeName+ext)
		log.Info(r.Context(), "Saving narrator avatar", "path", savePath)
		if err := os.WriteFile(savePath, imageData, 0644); err != nil {
			log.Error(r.Context(), "Failed to save image", err, "path", savePath)
			http.Error(w, "Failed to save image: "+err.Error(), 500)
			return
		}
		log.Info(r.Context(), "Narrator avatar saved successfully", "path", savePath)
		imageURL = "/api/scrape/image/narrator/" + safeName
	} else {
		// Real artist - save to artist-images and update DB
		imageDir := filepath.Join("data", "artist-images")
		os.MkdirAll(imageDir, 0755)

		// Remove old files
		for _, oldExt := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			os.Remove(filepath.Join(imageDir, id+oldExt))
		}

		savePath := filepath.Join(imageDir, id+ext)
		log.Info(r.Context(), "Saving artist image", "path", savePath)
		if err := os.WriteFile(savePath, imageData, 0644); err != nil {
			log.Error(r.Context(), "Failed to save image", err)
			http.Error(w, "Failed to save image: "+err.Error(), 500)
			return
		}

		// Update artist in DB
		repo := h.ds.Artist(r.Context())
		artist, err := repo.Get(id)
		if err != nil {
			log.Error(r.Context(), "Artist not found in DB", err, "id", id)
		} else {
			imageURL = "/api/scrape/image/artist/" + id
			artist.LargeImageUrl = imageURL
			artist.MediumImageUrl = imageURL
			artist.SmallImageUrl = imageURL
			if err := repo.Put(artist, "large_image_url", "medium_image_url", "small_image_url"); err != nil {
				log.Error(r.Context(), "Failed to update artist", "error", err)
			}
		}
	}

	log.Info(r.Context(), "applyArtistAvatar success", "imageURL", imageURL)
	writeJSON(w, map[string]any{"data": map[string]any{"imageUrl": imageURL}})
}

// serveImage serves saved images (narrator avatars, artist images)
func (h *scrapeHandler) serveImage(w http.ResponseWriter, r *http.Request) {
	imgType := chi.URLParam(r, "type") // "narrator" or "artist"
	id := chi.URLParam(r, "id")

	// Sanitize narrator name to match upload logic
	if imgType == "narrator" {
		id = strings.ReplaceAll(id, "/", "_")
		id = strings.ReplaceAll(id, "\\", "_")
		id = strings.ReplaceAll(id, "..", "_")
	}

	var dir string
	switch imgType {
	case "narrator":
		dir = filepath.Join("data", "narrator-avatars")
	case "artist":
		dir = filepath.Join("data", "artist-images")
	default:
		http.Error(w, "Invalid image type", 400)
		return
	}

	// Try each extension
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		path := filepath.Join(dir, id+ext)
		if _, err := os.Stat(path); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, path)
			return
		}
	}

	http.Error(w, "Image not found", 404)
}

func (h *scrapeHandler) batchScrape(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.Audiobook(r.Context())
	books, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var req struct {
		Sources []string `json:"sources"`
		Fields  []string `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	type batchItem struct {
		BookID  string                           `json:"bookId"`
		Title   string                           `json:"title"`
		Results map[string][]scraper.ScrapeResult `json:"results"`
	}
	var results []batchItem
	for _, book := range books {
		searchResults := scraper.SearchAll(book.Title, req.Sources)
		if len(searchResults) > 0 {
			results = append(results, batchItem{BookID: book.ID, Title: book.Title, Results: searchResults})
		}
	}
	writeJSON(w, map[string]any{"data": results})
}

// validateURL 验证URL是否可访问（HEAD请求）
func validateURL(urlStr string) bool {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func downloadCover(coverURL, bookPath string, book *model.Audiobook, lib model.Library) error {
	if coverURL == "" {
		return fmt.Errorf("empty cover URL")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", coverURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", coverURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download cover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download cover: HTTP %d", resp.StatusCode)
	}
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.jpeg", "folder.png"} {
		os.Remove(filepath.Join(bookPath, name))
	}
	coverPath := filepath.Join(bookPath, "cover"+ext)
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read cover data: %w", err)
	}
	if len(imageData) < 100 {
		return fmt.Errorf("cover data too small: %d bytes", len(imageData))
	}
	if err := os.WriteFile(coverPath, imageData, 0644); err != nil {
		return fmt.Errorf("write cover file: %w", err)
	}
	relCover, _ := filepath.Rel(lib.Path, coverPath)
	book.CoverPath = relCover
	return nil
}
