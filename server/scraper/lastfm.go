package scraper

import (
	"fmt"
	"net/url"
	"strings"
)

func init() { Register(&lastfmScraper{}) }

type lastfmScraper struct{}
func (s *lastfmScraper) Name() string        { return "lastfm" }
func (s *lastfmScraper) DisplayName() string  { return "Last.fm" }
func (s *lastfmScraper) SearchAudiobooks(q string, p int) ([]ScrapeResult, error) { return nil, nil }
func (s *lastfmScraper) GetAudiobookDetail(id string) (*ScrapeDetail, error) { return nil, fmt.Errorf("not supported") }

func (s *lastfmScraper) SearchArtists(query string) ([]ArtistResult, error) {
	searchURL := fmt.Sprintf("https://www.last.fm/search/artists?q=%s", url.QueryEscape(query))
	body, err := httpGet(searchURL, map[string]string{"Accept": "text/html"})
	if err != nil {
		return nil, fmt.Errorf("lastfm search: %w", err)
	}
	text := string(body)
	var results []ArtistResult
	prefix := "https://lastfm.freetls.fastly.net/i/u/"
	parts := strings.Split(text, prefix)
	for i, part := range parts[1:] {
		if i >= 5 { break }
		end := strings.IndexAny(part, "\"' ")
		if end < 0 || end > 200 { continue }
		imgPath := strings.TrimSpace(part[:end])
		name := query
		if ni := strings.Index(part, "artist-grid-with-faces-item-name"); ni > 0 {
			ns := part[ni:]
			if gi := strings.Index(ns, ">"); gi > 0 {
				if ci := strings.Index(ns[gi+1:], "<"); ci > 0 {
					name = strings.TrimSpace(ns[gi+1 : gi+1+ci])
				}
			}
		}
		if len(imgPath) > 10 {
			results = append(results, ArtistResult{Source: "lastfm", ID: fmt.Sprintf("lastfm-%d", i), Name: name, ImageURL: prefix + imgPath, Platform: "Last.fm"})
		}
	}
	return results, nil
}
