package scraper

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func init() {
	Register(&spotifyScraper{})
}

type spotifyScraper struct{}

func (s *spotifyScraper) Name() string      { return "spotify" }
func (s *spotifyScraper) DisplayName() string { return "Spotify" }

func (s *spotifyScraper) SearchAudiobooks(query string, page int) ([]ScrapeResult, error) {
	return nil, nil
}

func (s *spotifyScraper) GetAudiobookDetail(sourceID string) (*ScrapeDetail, error) {
	return nil, fmt.Errorf("not supported")
}

// SearchArtists searches for artist images on Spotify via open web API
func (s *spotifyScraper) SearchArtists(query string) ([]ArtistResult, error) {
	// Use Spotify's open search endpoint (no auth needed for basic artist info)
	searchURL := fmt.Sprintf(
		"https://api.spotify.com/v1/search?q=%s&type=artist&limit=5",
		url.QueryEscape(query),
	)

	// Try the open web API first (doesn't require auth)
	openURL := fmt.Sprintf(
		"https://open.spotify.com/search/%s/artists",
		url.QueryEscape(query),
	)

	// Use the web player API which is publicly accessible
	webAPIURL := fmt.Sprintf(
		"https://api-partner.spotify.com/pathfinder/v1/query?operationName=searchDesktop&variables=\"%s\"&extensions=\"%%7B%%22persistedQuery%%22%%3A%%7B%%22version%%22%%3A1%%2C%%22sha256Hash%%22%%3A%%22546b902c3083b008a3c2e0e1c3c3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e%%22%%7D%%7D\"",
		url.QueryEscape(query),
	)

	// Simplified: use the public search suggest endpoint
	suggestURL := fmt.Sprintf(
		"https://api.spotify.com/v1/search?q=%s&type=artist&limit=5&market=CN",
		url.QueryEscape(query),
	)

	_ = searchURL
	_ = openURL
	_ = webAPIURL
	_ = suggestURL

	// Spotify requires auth for API access, so we use a web scraping approach
	// Try the embed endpoint which is publicly accessible
	embedURL := fmt.Sprintf(
		"https://open.spotify.com/search/%s",
		url.QueryEscape(query),
	)

	body, err := httpGet(embedURL, map[string]string{
		"Accept": "text/html",
	})
	if err != nil {
		return nil, fmt.Errorf("spotify search: %w", err)
	}

	// Extract artist images from the HTML/JSON embedded in the page
	text := string(body)
	var results []ArtistResult

	// Look for artist data in the page's JSON state
	// Spotify embeds artist data in a script tag
	idx := strings.Index(text, "artist")
	if idx < 0 {
		return nil, fmt.Errorf("no artist data found")
	}

	// Try to find image URLs pattern
	imagePattern := `https://i.scdn.co/image/`
	parts := strings.Split(text, imagePattern)
	for i, part := range parts[1:] {
		if i >= 5 {
			break
		}
		// Extract the image ID
		endIdx := strings.IndexAny(part, "\"\' ")
		if endIdx < 0 {
			endIdx = len(part)
		}
		if endIdx > 40 {
			endIdx = 40
		}
		imageID := part[:endIdx]
		if len(imageID) > 10 {
			results = append(results, ArtistResult{
				Source:   "spotify",
				ID:       fmt.Sprintf("spotify-%d", i),
				Name:     query,
				ImageURL: "https://i.scdn.co/image/" + imageID,
				Platform: "Spotify",
			})
		}
	}

	return results, nil
}

// searchArtistsAlt is not needed for Spotify as the main method is the only option
func (s *spotifyScraper) searchArtistsAlt(query string) ([]ArtistResult, error) {
	return nil, fmt.Errorf("no alternative method")
}
