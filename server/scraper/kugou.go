package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(&kugouScraper{})
}

type kugouScraper struct{}

func (s *kugouScraper) Name() string      { return "kugou" }
func (s *kugouScraper) DisplayName() string { return "酷狗音乐" }

func (s *kugouScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	return nil, nil
}

func (s *kugouScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not supported")
}

type kugouSearchResponse struct {
	Status string `json:"status"`
	Data   struct {
		Info []kugouArtist `json:"info"`
	} `json:"data"`
}

type kugouArtist struct {
	SingerID   int64  `json:"singer_id"`
	SingerName string `json:"singer_name"`
	SingerPic  string `json:"singer_pic"`
	ImgURL     string `json:"img_url"`
}

func (s *kugouScraper) SearchArtists(query string) ([]ArtistResult, error) {
	// Use Kugou's mobile search API
	searchURL := fmt.Sprintf(
		"https://mobileservice.kugou.com/api/v3/search/singer?keyword=%s&page=1&pagesize=5",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"User-Agent": "Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36 Mobile Safari/537.36",
	})
	if err != nil {
		// Try alternative API
		return s.searchArtistsAlt(query)
	}

	var resp kugouSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return s.searchArtistsAlt(query)
	}

	var results []ArtistResult
	for _, artist := range resp.Data.Info {
		imageURL := artist.SingerPic
		if imageURL == "" {
			imageURL = artist.ImgURL
		}
		if imageURL == "" {
			continue
		}
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL
		}

		results = append(results, ArtistResult{
			Source:   "kugou",
			ID:       fmt.Sprintf("%d", artist.SingerID),
			Name:     artist.SingerName,
			ImageURL: imageURL,
			Platform: "酷狗音乐",
		})
	}

	return results, nil
}

func (s *kugouScraper) searchArtistsAlt(query string) ([]ArtistResult, error) {
	// Alternative: use Kugou's web search
	searchURL := fmt.Sprintf(
		"https://complexsearch.kugou.com/v2/search/mix?keyword=%s&page=1&pagesize=5&search_type=2&issubtitle=1",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer":    "https://www.kugou.com/",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
	})
	if err != nil {
		return nil, fmt.Errorf("kugou alt search: %w", err)
	}

	var resp struct {
		Data struct {
			Singers []struct {
				SingerID   int64  `json:"SingerID"`
				SingerName string `json:"SingerName"`
				SingerPic  string `json:"SingerPic"`
			} `json:"singers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kugou alt parse: %w", err)
	}

	var results []ArtistResult
	for _, artist := range resp.Data.Singers {
		imageURL := artist.SingerPic
		if imageURL == "" {
			continue
		}
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL
		}

		results = append(results, ArtistResult{
			Source:   "kugou",
			ID:       fmt.Sprintf("%d", artist.SingerID),
			Name:     artist.SingerName,
			ImageURL: imageURL,
			Platform: "酷狗音乐",
		})
	}

	return results, nil
}
