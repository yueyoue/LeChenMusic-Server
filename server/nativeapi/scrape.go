package nativeapi

import (
	"encoding/json"
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
		Source string                  `json:"source"`
		Name   string                  `json:"name"`
		Items  []scraper.ScrapeResult  `json:"items"`
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
			downloadCover(*req.CoverURL, bookPath, book, *lib)
		}
	}
	if err := repo.Put(book); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"data": book})
}

func (h *scrapeHandler) searchArtists(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "q parameter required", 400)
		return
	}
	results := scraper.SearchArtistsAll(query)
	writeJSON(w, map[string]any{"data": results})
}

func (h *scrapeHandler) applyArtistAvatar(w http.ResponseWriter, r *http.Request) {
	artistID := chi.URLParam(r, "id")
	repo := h.ds.Artist(r.Context())
	artist, err := repo.Get(artistID)
	if err != nil {
		http.Error(w, "Artist not found", 404)
		return
	}
	var req struct {
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.ImageURL == "" {
		http.Error(w, "imageUrl required", 400)
		return
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(req.ImageURL)
	if err != nil {
		http.Error(w, "Failed to download image: "+err.Error(), 400)
		return
	}
	defer resp.Body.Close()
	imageDir := filepath.Join("data", "artist-images")
	os.MkdirAll(imageDir, 0755)
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}
	imagePath := filepath.Join(imageDir, artistID+ext)
	imageData, _ := io.ReadAll(resp.Body)
	os.WriteFile(imagePath, imageData, 0644)
	imageURL := "/api/scrape/artist/" + artistID + "/image"
	artist.LargeImageUrl = imageURL
	artist.MediumImageUrl = imageURL
	artist.SmallImageUrl = imageURL
	if err := repo.Put(artist, "large_image_url", "medium_image_url", "small_image_url"); err != nil {
		log.Error(r.Context(), "Failed to update artist", "error", err)
	}
	writeJSON(w, map[string]any{"data": map[string]any{"imageUrl": imageURL}})
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
		BookID  string                         `json:"bookId"`
		Title   string                         `json:"title"`
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

func downloadCover(coverURL, bookPath string, book *model.Audiobook, lib model.Library) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(coverURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	}
	for _, name := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.jpeg", "folder.png"} {
		os.Remove(filepath.Join(bookPath, name))
	}
	coverPath := filepath.Join(bookPath, "cover"+ext)
	imageData, _ := io.ReadAll(resp.Body)
	os.WriteFile(coverPath, imageData, 0644)
	relCover, _ := filepath.Rel(lib.Path, coverPath)
	book.CoverPath = relCover
}
