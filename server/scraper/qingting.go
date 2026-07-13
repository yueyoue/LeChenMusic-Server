package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func init() {
	Register(&qingtingScraper{})
}

type qingtingScraper struct{}

func (s *qingtingScraper) Name() string     { return "qingting" }
func (s *qingtingScraper) DisplayName() string { return "蜻蜓FM" }

type qingtingSearchResponse struct {
	Data struct {
		Items []qingtingItem `json:"items"`
	} `json:"data"`
}

type qingtingItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Thumbs      struct {
		LargeThumb string `json:"large_thumb"`
	} `json:"thumbs"`
	Podcasters []struct {
		Nickname string `json:"nickname"`
	} `json:"podcasters"`
	CategoryName string `json:"category_name"`
	ProgramCount int    `json:"program_count"`
	Star         int    `json:"star"`
}

func (s *qingtingScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	searchURL := fmt.Sprintf(
		"https://api.open.qtfm.cn/media/v7/search?kw=%s&page=%d&rows=10&category=album",
		url.QueryEscape(query), page,
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://www.qingting.fm/",
	})
	if err != nil {
		return nil, fmt.Errorf("qingting search: %w", err)
	}

	var resp qingtingSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("qingting parse: %w", err)
	}

	var results []ScrapeResult
	for _, item := range resp.Data.Items {
		narrator := ""
		if len(item.Podcasters) > 0 {
			narrator = item.Podcasters[0].Nickname
		}

		coverURL := item.Thumbs.LargeThumb

		results = append(results, ScrapeResult{
			Source:       "qingting",
			ID:           fmt.Sprintf("%d", item.ID),
			Title:        item.Title,
			Narrator:     narrator,
			CoverURL:     coverURL,
			Intro:        item.Description,
			Genre:        item.CategoryName,
			ChapterCount: item.ProgramCount,
		})
	}

	return results, nil
}

func (s *qingtingScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	detailURL := fmt.Sprintf("https://api.open.qtfm.cn/media/v7/channel/%s", sourceID)

	body, err := httpGet(detailURL, nil)
	if err != nil {
		return nil, fmt.Errorf("qingting detail: %w", err)
	}

	var resp struct {
		Data struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Thumbs      struct {
				LargeThumb string `json:"large_thumb"`
			} `json:"thumbs"`
			Podcasters []struct {
				Nickname string `json:"nickname"`
			} `json:"podcasters"`
			CategoryName string `json:"category_name"`
			ProgramCount int    `json:"program_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("qingting detail parse: %w", err)
	}

	narrator := ""
	if len(resp.Data.Podcasters) > 0 {
		narrator = resp.Data.Podcasters[0].Nickname
	}

	return &ScrapeDetail{
		Source:       "qingting",
		ID:           sourceID,
		Title:        resp.Data.Title,
		Narrator:     narrator,
		CoverURL:     resp.Data.Thumbs.LargeThumb,
		Intro:        resp.Data.Description,
		Genre:        resp.Data.CategoryName,
		ChapterCount: resp.Data.ProgramCount,
	}, nil
}

func (s *qingtingScraper) SearchArtists(query string) ([]ArtistResult, error) {
	return nil, nil
}
