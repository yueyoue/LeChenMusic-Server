package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(&lastfmScraper{})
}

type lastfmScraper struct{}

func (s *lastfmScraper) Name() string      { return "lastfm" }
func (s *lastfmScraper) DisplayName() string { return "Last.fm" }

func (s *lastfmScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	return nil, nil
}

func (s *lastfmScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not supported")
}

// SearchArtists searches for artist images on Last.fm
func (s *lastfmScraper) SearchArtists(query string) ([]ArtistResult, error) {
	// Last.fm has a public API for artist search
	// Using the web endpoint which doesn't require an API key
	searchURL := fmt.Sprintf(
		"https://www.last.fm/search/artists?q=%s",
		url.QueryEscape(query),
	)

	body, err := httpGet(searchURL, map[string]string{
		"Accept": "text/html",
	})
	if err != nil {
		return nil, fmt.Errorf("lastfm search: %w", err)
	}

	text := string(body)
	var results []ArtistResult

	// Extract artist data from the HTML
	// Last.fm renders artist cards with images
	// Look for image URLs in artist cards
	parts := strings.Split(text, "class=\"artist-grid-with-faces-item-cover-image-link\"")
	for i, part := range parts[1:] {
		if i >= 5 {
			break
		}
		// Find the image URL
		imgIdx := strings.Index(part, "src=\"")
		if imgIdx < 0 {
			continue
		}
		imgPart := part[imgIdx+5:]
		endIdx := strings.Index(imgPart, "\"")
		if endIdx < 0 {
			continue
		}
		imageURL := imgPart[:endIdx]

		// Find artist name
		nameIdx := strings.Index(part, "class=\"artist-grid-with-faces-item-name\"")
		name := query
		if nameIdx > 0 {
			namePart := part[nameIdx:]
			gtIdx := strings.Index(namePart, ">")
			if gtIdx > 0 {
				nameEnd := strings.Index(namePart[gtIdx+1:], "<")
				if nameEnd > 0 {
					name = strings.TrimSpace(namePart[gtIdx+1 : gtIdx+1+nameEnd])
				}
			}
		}

		if imageURL != "" && !strings.Contains(imageURL, "default") {
			results = append(results, ArtistResult{
				Source:   "lastfm",
				ID:       fmt.Sprintf("lastfm-%d", i),
				Name:     name,
				ImageURL: imageURL,
				Platform: "Last.fm",
			})
		}
	}

	return results, nil
}
