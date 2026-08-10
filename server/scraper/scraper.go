package scraper

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
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
		// Sort by title match relevance
		queryLower := strings.ToLower(query)
		sort.Slice(res, func(i, j int) bool {
			return titleMatchScore(res[i].Title, queryLower) > titleMatchScore(res[j].Title, queryLower)
		})
		results[s.Name()] = res
	}
	return results
}

// titleMatchScore calculates a relevance score for title matching
func titleMatchScore(title, queryLower string) int {
	titleLower := strings.ToLower(title)
	if titleLower == queryLower {
		return 100
	}
	if strings.HasPrefix(titleLower, queryLower) {
		return 80
	}
	if strings.Contains(titleLower, queryLower) {
		return 60
	}
	if strings.Contains(queryLower, titleLower) {
		return 40
	}
	return 0
}

// SourceQualityWeights defines reliability scores for each source when searching artist images.
// Higher weight = more reliable/accurate artist photos.
// Music platforms generally have better artist photos than audiobook platforms.
var SourceQualityWeights = map[string]int{
	"netease":  95,  // 网易云音乐 - 艺人图片最全、质量最高
	"qqmusic":  90,  // QQ音乐 - 官方高清头像
	"kugou":    80,  // 酷狗音乐 - 艺人图片较全
	"kuwo":     75,  // 酷我音乐 - 艺人图片质量不错
	"spotify":  85,  // Spotify - 国际艺人图片质量高
	"lastfm":   70,  // Last.fm - 社区贡献，质量参差不齐
	"ximalaya": 30,  // 喜马拉雅 - 有声书平台，音乐艺人图片少
	"qingting": 25,  // 蜻蜓FM - 有声书平台，音乐艺人图片少
}

// SearchArtistsAll searches all platforms for artist images, sorted by quality + match score
func SearchArtistsAll(query string) []ArtistResult {
	var results []ArtistResult
	for _, s := range scrapers {
		res, err := s.SearchArtists(query)
		if err != nil {
			continue
		}
		// Filter out results without images
		var filtered []ArtistResult
		for _, r := range res {
			if r.ImageURL != "" {
				filtered = append(filtered, r)
			}
		}
		results = append(results, filtered...)
	}

	// Sort by composite score: name match * 0.6 + source quality * 0.4
	queryLower := strings.ToLower(query)
	sort.Slice(results, func(i, j int) bool {
		iNameScore := nameMatchScore(results[i].Name, queryLower)
		jNameScore := nameMatchScore(results[j].Name, queryLower)
		iQuality := SourceQualityWeights[results[i].Source]
		jQuality := SourceQualityWeights[results[j].Source]
		if iQuality == 0 { iQuality = 50 }
		if jQuality == 0 { jQuality = 50 }
		iTotal := iNameScore*60 + iQuality*40
		jTotal := jNameScore*60 + jQuality*40
		return iTotal > jTotal
	})

	// Filter out results with low relevance scores to avoid showing irrelevant artists
	var filtered []ArtistResult
	for _, r := range results {
		score := nameMatchScore(r.Name, queryLower)
		if score >= 30 { // Only include results with meaningful matches
			filtered = append(filtered, r)
		}
	}

	// If no good matches found, return top results but limit count
	if len(filtered) == 0 && len(results) > 0 {
		limit := 5
		if len(results) < limit {
			limit = len(results)
		}
		return results[:limit]
	}

	return filtered
}

// SearchNarratorsAll searches audiobook platforms only for narrator/anchor avatars
func SearchNarratorsAll(query string) []ArtistResult {
	var results []ArtistResult
	audiobookSources := map[string]bool{"ximalaya": true, "qingting": true}
	for _, s := range scrapers {
		if !audiobookSources[s.Name()] {
			continue
		}
		res, err := s.SearchArtists(query)
		if err != nil {
			continue
		}
		var filtered []ArtistResult
		for _, r := range res {
			if r.ImageURL != "" {
				filtered = append(filtered, r)
			}
		}
		results = append(results, filtered...)
	}

	queryLower := strings.ToLower(query)
	sort.Slice(results, func(i, j int) bool {
		return nameMatchScore(results[i].Name, queryLower) > nameMatchScore(results[j].Name, queryLower)
	})

	return results
}


// nameMatchScore calculates a relevance score for name matching
func nameMatchScore(name, queryLower string) int {
	nameLower := strings.ToLower(name)
	if nameLower == queryLower {
		return 100 // Exact match
	}
	if strings.HasPrefix(nameLower, queryLower) {
		return 80 // Starts with query
	}
	if strings.HasSuffix(nameLower, queryLower) {
		return 60 // Ends with query
	}
	if strings.Contains(nameLower, queryLower) {
		return 40 // Contains query
	}
	// Check if query contains name (for short names)
	if strings.Contains(queryLower, nameLower) && len(nameLower) >= 2 {
		return 30
	}
	// For Chinese characters, require at least 2 consecutive characters match
	// to avoid false positives from single character overlap
	if len(queryLower) >= 2 {
		// Check for consecutive character match
		maxConsecutive := 0
		currentConsecutive := 0
		queryIdx := 0
		for _, c := range nameLower {
			if queryIdx < len(queryLower) && c == rune(queryLower[queryIdx]) {
				currentConsecutive++
				queryIdx++
				if currentConsecutive > maxConsecutive {
					maxConsecutive = currentConsecutive
				}
			} else {
				currentConsecutive = 0
				queryIdx = 0
				// Check if current char starts a new match
				if c == rune(queryLower[0]) {
					currentConsecutive = 1
					queryIdx = 1
				}
			}
		}
		// Require at least 2 consecutive characters for a meaningful match
		if maxConsecutive >= 2 {
			return maxConsecutive * 15 // Score based on consecutive match length
		}
	}
	return 0 // No meaningful match
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
