package scraper

import (
	"fmt"
	"net/url"
	"strings"
)

func init() { Register(&spotifyScraper{}) }

type spotifyScraper struct{}
func (s *spotifyScraper) Name() string        { return "spotify" }
func (s *spotifyScraper) DisplayName() string  { return "Spotify" }
func (s *spotifyScraper) SearchAudiobooks(q string, p int) ([]ScrapeResult, error) { return nil, nil }
func (s *spotifyScraper) GetAudiobookDetail(id string) (*ScrapeDetail, error) { return nil, fmt.Errorf("not supported") }

func (s *spotifyScraper) SearchArtists(query string) ([]ArtistResult, error) {
	searchURL := fmt.Sprintf("https://open.spotify.com/search/%s", url.PathEscape(query))
	body, err := httpGet(searchURL, map[string]string{"Accept": "text/html"})
	if err != nil {
		return nil, fmt.Errorf("spotify search: %w", err)
	}
	text := string(body)
	var results []ArtistResult
	prefix := "https://i.scdn.co/image/"
	parts := strings.Split(text, prefix)
	for i, part := range parts[1:] {
		if i >= 5 { break }
		end := strings.IndexAny(part, "\"'; ")
		if end < 0 || end > 60 { end = 40 }
		id := strings.TrimSpace(part[:end])
		if len(id) > 10 {
			results = append(results, ArtistResult{Source: "spotify", ID: fmt.Sprintf("spotify-%d", i), Name: query, ImageURL: prefix + id, Platform: "Spotify"})
		}
	}
	return results, nil
}
