package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func init() {
	Register(&neteaseScraper{})
}

type neteaseScraper struct{}

func (s *neteaseScraper) Name() string     { return "netease" }
func (s *neteaseScraper) DisplayName() string { return "网易云音乐" }

// SearchAudiobooks - disabled, Netease is only used for artist avatar search
func (s *neteaseScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	return nil, nil
}

func (s *neteaseScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not supported")
}

// SearchArtists searches for artist images on Netease Cloud Music
type neteaseArtistSearchResponse struct {
	Result struct {
		Artists []neteaseArtist `json:"artists"`
	} `json:"result"`
}

type neteaseArtist struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	PicURL   string `json:"picUrl"`
	Img1V1URL string `json:"img1v1Url"`
}

func (s *neteaseScraper) SearchArtists(query string) ([]ArtistResult, error) {
	searchURL := fmt.Sprintf(
		"https://music.163.com/api/search/get/web?s=%s&type=100&limit=5",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://music.163.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("netease artist search: %w", err)
	}

	var resp neteaseArtistSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease artist parse: %w", err)
	}

	var results []ArtistResult
	for _, artist := range resp.Result.Artists {
		imageURL := artist.PicURL
		if imageURL == "" {
			imageURL = artist.Img1V1URL
		}
		if imageURL == "" {
			continue
		}

		results = append(results, ArtistResult{
			Source:   "netease",
			ID:       fmt.Sprintf("%d", artist.ID),
			Name:     artist.Name,
			ImageURL: imageURL,
			Platform: "网易云音乐",
		})
	}

	return results, nil
}
