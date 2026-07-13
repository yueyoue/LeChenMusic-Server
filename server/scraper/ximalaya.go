package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func init() {
	Register(&ximalayaScraper{})
}

type ximalayaScraper struct{}

func (s *ximalayaScraper) Name() string     { return "ximalaya" }
func (s *ximalayaScraper) DisplayName() string { return "喜马拉雅" }

type ximalayaSearchResponse struct {
	Ret  int `json:"ret"`
	Data struct {
		Result struct {
			Response struct {
				Docs []ximalayaAlbum `json:"docs"`
			} `json:"response"`
		} `json:"result"`
	} `json:"data"`
}

type ximalayaAlbum struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	CustomTitle  string `json:"custom_title"`
	Nickname     string `json:"nickname"`     // narrator
	Intro        string `json:"intro"`
	CoverPath    string `json:"cover_path"`
	CategoryTitle string `json:"category_title"`
	CreatedAt    int64  `json:"created_at"`
	Tags         string `json:"tags"`
	AlbumCount   int    `json:"album_count"`  // chapter count
	Author       string `json:"author"`
}

type ximalayaDetailResponse struct {
	Data struct {
		Intro struct {
			RichIntro string `json:"richIntro"`
		} `json:"intro"`
	} `json:"data"`
}

func (s *ximalayaScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	searchURL := fmt.Sprintf(
		"https://www.ximalaya.com/revision/search?core=album&kw=%s&page=%d&spellchecker=true&rows=10&condition=relation&device=web",
		url.QueryEscape(query), page,
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://www.ximalaya.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("ximalaya search: %w", err)
	}

	var resp ximalayaSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("ximalaya parse: %w", err)
	}

	if resp.Ret != 200 {
		return nil, fmt.Errorf("ximalaya api error: ret=%d", resp.Ret)
	}

	var results []ScrapeResult
	for _, doc := range resp.Data.Result.Response.Docs {
		coverURL := doc.CoverPath
		if coverURL != "" && !strings.HasPrefix(coverURL, "http") {
			coverURL = "https:" + coverURL
		}
		// Remove image processing suffix
		coverURL = strings.Split(coverURL, "!")[0]

		year := 0
		if doc.CreatedAt > 0 {
			year = time.Unix(doc.CreatedAt/1000, 0).Year()
		}

		results = append(results, ScrapeResult{
			Source:       "ximalaya",
			ID:           fmt.Sprintf("%d", doc.ID),
			Title:        doc.Title,
			Author:       doc.Author,
			Narrator:     doc.Nickname,
			CoverURL:     coverURL,
			Intro:        doc.Intro,
			Genre:        doc.CategoryTitle,
			Year:         year,
			ChapterCount: doc.AlbumCount,
		})
	}

	return results, nil
}

func (s *ximalayaScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	// Get rich intro from mobile API
	detailURL := fmt.Sprintf(
		"https://mobile.ximalaya.com/mobile-album/album/plant/detail?albumId=%s&identity=podcast&supportWebp=true",
		sourceID,
	)

	body, err := httpGet(detailURL, map[string]string{
		"Cookie": "1&_device=android&28b5647f-40d9-3cb6-802a-54905eccc23d&9.3.96",
	})
	if err != nil {
		return nil, fmt.Errorf("ximalaya detail: %w", err)
	}

	var resp ximalayaDetailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("ximalaya detail parse: %w", err)
	}

	detail := &ScrapeDetail{
		Source: "ximalaya",
		ID:     sourceID,
		Intro:  resp.Data.Intro.RichIntro,
	}

	// Clean HTML from rich intro
	detail.Intro = cleanHTML(detail.Intro)

	return detail, nil
}

func (s *ximalayaScraper) SearchArtists(query string) ([]ArtistResult, error) {
	// Ximalaya doesn't have artist search in the traditional sense
	return nil, nil
}

// cleanHTML removes HTML tags from text
func cleanHTML(html string) string {
	// Simple tag removal
	result := html
	result = strings.ReplaceAll(result, "<br>", "\n")
	result = strings.ReplaceAll(result, "<br/>", "\n")
	result = strings.ReplaceAll(result, "<br />", "\n")
	result = strings.ReplaceAll(result, "<p>", "")
	result = strings.ReplaceAll(result, "</p>", "\n")

	// Remove all remaining HTML tags
	for {
		start := strings.Index(result, "<")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}

	// Clean up whitespace
	result = strings.TrimSpace(result)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return result
}
