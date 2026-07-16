package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(&kuwoScraper{})
}

type kuwoScraper struct{}

func (s *kuwoScraper) Name() string      { return "kuwo" }
func (s *kuwoScraper) DisplayName() string { return "酷我音乐" }

func (s *kuwoScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	return nil, nil
}

func (s *kuwoScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not supported")
}

type kuwoSearchResponse struct {
	Artists struct {
		List []kuwoArtist `json:"list"`
	} `json:"artists"`
}

type kuwoArtist struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Pic    string `json:"pic"`
	Pic120 string `json:"pic120"`
	Pic70  string `json:"pic70"`
}

func (s *kuwoScraper) SearchArtists(query string) ([]ArtistResult, error) {
	searchURL := fmt.Sprintf(
		"https://search.kuwo.cn/r.s?all=%s&ft=artist&newsearch=1&alflac=1&encoding=utf8&rformat=json&mobi=1",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://www.kuwo.cn/",
	})
	if err != nil {
		return nil, fmt.Errorf("kuwo artist search: %w", err)
	}

	var resp kuwoSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kuwo artist parse: %w", err)
	}

	var results []ArtistResult
	for _, artist := range resp.Artists.List {
		imageURL := artist.Pic
		if imageURL == "" {
			imageURL = artist.Pic120
		}
		if imageURL == "" {
			imageURL = artist.Pic70
		}
		if imageURL == "" {
			continue
		}
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL
		}

		results = append(results, ArtistResult{
			Source:   "kuwo",
			ID:       fmt.Sprintf("%d", artist.ID),
			Name:     artist.Name,
			ImageURL: imageURL,
			Platform: "酷我音乐",
		})
	}

	return results, nil
}
