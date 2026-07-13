package scraper

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// ScrapeResult represents a search result from any scraper source
type ScrapeResult struct {
	Source   string `json:"source"`   // e.g. "ximalaya", "qingting"
	ID       string `json:"id"`       // ID on the source platform
	Title    string `json:"title"`
	Author   string `json:"author"`
	Narrator string `json:"narrator"`
	CoverURL string `json:"coverUrl"`
	Intro    string `json:"intro"`
	Genre    string `json:"genre"`
	Year     int    `json:"year"`
	ChapterCount int `json:"chapterCount"`
}

// ScrapeDetail represents detailed metadata for a single audiobook
type ScrapeDetail struct {
	Source    string `json:"source"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Narrator  string `json:"narrator"`
	CoverURL  string `json:"coverUrl"`
	Intro     string `json:"intro"`
	Genre     string `json:"genre"`
	Year      int    `json:"year"`
	ChapterCount int `json:"chapterCount"`
	Tags      []string `json:"tags"`
}

// ArtistResult represents an artist search result (for avatar scraping)
type ArtistResult struct {
	Source    string `json:"source"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ImageURL  string `json:"imageUrl"`
	Platform  string `json:"platform"` // e.g. "netease", "qq"
}

// Scraper is the interface that all scraper implementations must satisfy
type Scraper interface {
	// Name returns the scraper source name (e.g. "ximalaya")
	Name() string
	// DisplayName returns a human-readable name (e.g. "喜马拉雅")
	DisplayName() string
	// SearchAudiobooks searches for audiobooks by keyword
	SearchAudiobooks(query string, page int) ([]ScrapeResult, error)
	// GetAudiobookDetail gets detailed metadata for a single audiobook
	GetAudiobookDetail(sourceID string) (*ScrapeDetail, error)
	// SearchArtists searches for artist images (optional, only for music platforms)
	SearchArtists(query string) ([]ArtistResult, error)
}

// Registry holds all registered scrapers
var scrapers = make(map[string]Scraper)

// Register adds a scraper to the registry
func Register(s Scraper) {
	scrapers[s.Name()] = s
}

// Get returns a scraper by name
func Get(name string) (Scraper, bool) {
	s, ok := scrapers[name]
	return s, ok
}

// GetAll returns all registered scrapers
func GetAll() []Scraper {
	var result []Scraper
	for _, s := range scrapers {
		result = append(result, s)
	}
	return result
}

// SearchAll searches all enabled scrapers and aggregates results
func SearchAll(query string, sources []string) map[string][]ScrapeResult {
	results := make(map[string][]ScrapeResult)
	for _, s := range scrapers {
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
		if err != nil {
			results[s.Name()] = []ScrapeResult{}
			continue
		}
		results[s.Name()] = res
	}
	return results
}

// SearchArtistsAll searches all platforms for artist images
func SearchArtistsAll(query string) []ArtistResult {
	var results []ArtistResult
	for _, s := range scrapers {
		res, err := s.SearchArtists(query)
		if err != nil {
			continue
		}
		results = append(results, res...)
	}
	return results
}

// httpClient is a shared HTTP client with reasonable timeouts
var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// httpGet performs a GET request with common headers
func httpGet(url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}
