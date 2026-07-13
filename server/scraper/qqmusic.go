package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(&qqmusicScraper{})
}

type qqmusicScraper struct{}

func (s *qqmusicScraper) Name() string     { return "qqmusic" }
func (s *qqmusicScraper) DisplayName() string { return "QQ音乐" }

func (s *qqmusicScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	// QQ Music doesn't have a public audiobook search API easily accessible
	return nil, nil
}

func (s *qqmusicScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not implemented")
}

// SearchArtists searches for artist images on QQ Music
type qqmusicSearchResponse struct {
	Data struct {
		Singer struct {
			List []qqmusicSinger `json:"list"`
		} `json:"singer"`
	} `json:"data"`
}

type qqmusicSinger struct {
	SingerID   int64  `json:"singer_id"`
	SingerName string `json:"singer_name"`
	SingerPic  string `json:"singer_pic"`
}

func (s *qqmusicScraper) SearchArtists(query string) ([]ArtistResult, error) {
	searchURL := fmt.Sprintf(
		"https://c.y.qq.com/soso/fcgi-bin/client_search_cp?w=%s&format=json&p=1&n=5&cr=1&catZhida=1",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"Referer": "https://y.qq.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("qqmusic search: %w", err)
	}

	var resp qqmusicSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// Try alternative API
		return s.searchArtistsAlt(query)
	}

	var results []ArtistResult
	for _, singer := range resp.Data.Singer.List {
		imageURL := singer.SingerPic
		if imageURL == "" {
			continue
		}
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = "https:" + imageURL
		}

		results = append(results, ArtistResult{
			Source:   "qqmusic",
			ID:       fmt.Sprintf("%d", singer.SingerID),
			Name:     singer.SingerName,
			ImageURL: imageURL,
			Platform: "QQ音乐",
		})
	}

	return results, nil
}

func (s *qqmusicScraper) searchArtistsAlt(query string) ([]ArtistResult, error) {
	// Alternative QQ Music search API
	searchURL := fmt.Sprintf(
		"https://u.y.qq.com/cgi-bin/musicu.fcg?data={\"music.search.SearchCgiService\":{\"method\":\"DoSearchForQQMusicDesktop\",\"module\":\"music.search.SearchCgiService\",\"param\":{\"query\":\"%s\",\"num_per_page\":5,\"page_num\":1,\"search_type\":1}}}",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("qqmusic alt search: %w", err)
	}

	// Parse the nested JSON response
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("qqmusic alt parse: %w", err)
	}

	// Navigate the nested structure
	singerService, ok := resp["music.search.SearchCgiService"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response structure")
	}
	data, ok := singerService["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no data in response")
	}
	singer, ok := data["singer"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no singer in response")
	}
	list, ok := singer["list"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no singer list")
	}

	var results []ArtistResult
	for _, item := range list {
		s, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := s["singer_name"].(string)
		id, _ := s["singer_id"].(float64)
		pic, _ := s["singer_pic"].(string)

		if pic == "" || name == "" {
			continue
		}
		if !strings.HasPrefix(pic, "http") {
			pic = "https:" + pic
		}

		results = append(results, ArtistResult{
			Source:   "qqmusic",
			ID:       fmt.Sprintf("%.0f", id),
			Name:     name,
			ImageURL: pic,
			Platform: "QQ音乐",
		})
	}

	return results, nil
}
