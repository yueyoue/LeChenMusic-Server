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

// Search audiobooks (podcasts) on Netease
type neteaseSearchResponse struct {
	Result struct {
		DjRadios []neteaseDjRadio `json:"djRadios"`
	} `json:"result"`
}

type neteaseDjRadio struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Dj          struct {
		Nickname string `json:"nickname"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"dj"`
	Description string `json:"description"`
	PicURL      string `json:"picUrl"`
	ProgramCount int   `json:"programCount"`
	Category    string `json:"category"`
}

func (s *neteaseScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	searchURL := fmt.Sprintf(
		"https://music.163.com/api/search/get/web?s=%s&type=1009&offset=%d&limit=10",
		url.QueryEscape(query), (page-1)*10,
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://music.163.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("netease search: %w", err)
	}

	var resp neteaseSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease parse: %w", err)
	}

	var results []ScrapeResult
	for _, radio := range resp.Result.DjRadios {
		results = append(results, ScrapeResult{
			Source:       "netease",
			ID:           fmt.Sprintf("%d", radio.ID),
			Title:        radio.Name,
			Narrator:     radio.Dj.Nickname,
			CoverURL:     radio.PicURL,
			Intro:        radio.Description,
			Genre:        radio.Category,
			ChapterCount: radio.ProgramCount,
		})
	}

	return results, nil
}

func (s *neteaseScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	detailURL := fmt.Sprintf("https://music.163.com/api/dj/program/detail?id=%s", sourceID)

	body, err := httpGet(detailURL, map[string]string{
		"Referer": "https://music.163.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("netease detail: %w", err)
	}

	var resp struct {
		Data struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			PicURL      string `json:"picUrl"`
			Dj          struct {
				Nickname string `json:"nickname"`
			} `json:"dj"`
			Category string `json:"category"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("netease detail parse: %w", err)
	}

	return &ScrapeDetail{
		Source:   "netease",
		ID:       sourceID,
		Title:    resp.Data.Name,
		Narrator: resp.Data.Dj.Nickname,
		CoverURL: resp.Data.PicURL,
		Intro:    resp.Data.Description,
		Genre:    resp.Data.Category,
	}, nil
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
