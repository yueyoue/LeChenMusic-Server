package online

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
)

const (
	kuwoWebBaseURL       = "https://www.kuwo.cn"
	albumInfoCacheTTL    = 12 * time.Hour
	albumInfoMaxBody     = 2 << 20 // 2 MiB: album pages are ~200 KiB
	albumInfoHTTPTimeout = 25 * time.Second
	albumInfoMinGap      = 700 * time.Millisecond
	albumInfoDateWindow  = 120 // runes scanned after the "发行时间" label
)

// AlbumInfo is album-level metadata. The library derives "album artist" from
// ALBUMARTIST/TPE2 and the album year from date tags, so these two fields are
// what a scrape cannot get from song-level data alone.
type AlbumInfo struct {
	AlbumID     string `json:"albumId,omitempty"`
	AlbumName   string `json:"albumName,omitempty"`
	Artist      string `json:"artist,omitempty"`      // 专辑艺术家 (album artist)
	ReleaseDate string `json:"releaseDate,omitempty"` // YYYY[-MM[-DD]] as published upstream
	Year        int    `json:"year,omitempty"`
}

// AlbumInfoProvider resolves album-level metadata for a song.
// Implementations must be best-effort: an error only means the caller writes
// the tags without the album-level fields.
type AlbumInfoProvider interface {
	// AlbumInfo resolves album metadata from the album page when albumID is a
	// usable upstream id, otherwise from the song page (date only).
	AlbumInfo(ctx context.Context, albumID, songID string) (*AlbumInfo, error)
}

// kuwoAlbumInfo reads album artist and release date from the kuwo web pages:
//
//	GET /album_detail/<albumId>   → <p class="artist_name">…</p> + song_info (发行时间)
//	GET /play_detail/<songId>     → song_info (发行时间) only
//
// The pages are server-rendered HTML, so no token/callback signing is needed
// (the JSON API requires kw_token + csrf and rejects plain requests).
type kuwoAlbumInfo struct {
	baseURL string
	client  *http.Client

	fetchMu  sync.Mutex // serializes upstream fetches (politeness + dedupe)
	lastCall time.Time
	cache    sync.Map // "album:<id>" / "song:<id>" -> albumInfoEntry
}

type albumInfoEntry struct {
	info    *AlbumInfo
	expires time.Time
}

// NewKuwoAlbumInfo creates the default album metadata provider.
// baseURL is overridable for tests (empty means the public kuwo site).
func NewKuwoAlbumInfo(baseURL string) *kuwoAlbumInfo {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = kuwoWebBaseURL
	}
	return &kuwoAlbumInfo{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: albumInfoHTTPTimeout},
	}
}

var errNoAlbumKey = errors.New("online/albuminfo: no usable album or song id")

// AlbumInfo implements AlbumInfoProvider.
func (k *kuwoAlbumInfo) AlbumInfo(ctx context.Context, albumID, songID string) (*AlbumInfo, error) {
	key, path := "", ""
	if id := numericID(albumID); id != "" {
		key, path = "album:"+id, "/album_detail/"+id
	} else if id := numericID(songID); id != "" {
		key, path = "song:"+id, "/play_detail/"+id
	} else {
		return nil, errNoAlbumKey
	}
	if info := k.cached(key); info != nil {
		return info, nil
	}

	k.fetchMu.Lock()
	defer k.fetchMu.Unlock()
	// Another goroutine may have filled the cache while we waited for the lock.
	if info := k.cached(key); info != nil {
		return info, nil
	}

	body, err := k.get(ctx, path)
	if err != nil {
		return nil, err
	}
	info := parseKuwoAlbumInfo(body)
	if id := numericID(albumID); id != "" {
		info.AlbumID = id
	}
	k.cache.Store(key, albumInfoEntry{info: info, expires: time.Now().Add(albumInfoCacheTTL)})
	return info, nil
}

func (k *kuwoAlbumInfo) cached(key string) *AlbumInfo {
	entry, ok := k.cache.Load(key)
	if !ok {
		return nil
	}
	e, ok := entry.(albumInfoEntry)
	if !ok || time.Now().After(e.expires) {
		return nil
	}
	return e.info
}

// get fetches one page, enforcing a jittered minimum gap between requests.
func (k *kuwoAlbumInfo) get(ctx context.Context, path string) (string, error) {
	if err := k.throttle(ctx); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("online/albuminfo: build request: %w", err)
	}
	req.Header.Set("User-Agent", hicnUserAgents[rand.Intn(len(hicnUserAgents))])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", k.baseURL+"/")

	resp, err := k.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("online/albuminfo: request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("online/albuminfo: %s returned status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, albumInfoMaxBody))
	if err != nil {
		return "", fmt.Errorf("online/albuminfo: read response: %w", err)
	}
	return string(body), nil
}

func (k *kuwoAlbumInfo) throttle(ctx context.Context) error {
	gap := albumInfoMinGap + time.Duration(rand.Int63n(int64(albumInfoMinGap/2)+1))
	wait := gap - time.Since(k.lastCall)
	if wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	k.lastCall = time.Now()
	return nil
}

var (
	kuwoArtistBlock = regexp.MustCompile(`(?s)<p class="artist_name[^"]*"[^>]*>(.*?)</p>`)
	kuwoSongName    = regexp.MustCompile(`(?s)<p class="song_name[^"]*"[^>]*>(.*?)</p>`)
	kuwoInfoBlock   = regexp.MustCompile(`(?s)<p class="song_info[^"]*"[^>]*>(.*?)</p>`)
	kuwoSpan        = regexp.MustCompile(`(?s)<span([^>]*)>(.*?)</span>`)
	kuwoTag         = regexp.MustCompile(`<[^>]*>`)
	kuwoDate        = regexp.MustCompile(`(\d{4})(?:[-/.](\d{1,2})(?:[-/.](\d{1,2}))?)?`)
	spaceRun        = regexp.MustCompile(`\s+`)
)

// NormalizeAlbumID returns the upstream album id when it is usable (a
// non-zero numeric id), otherwise an empty string. Clients sometimes send the
// placeholder "0" for albums without a dedicated page.
func NormalizeAlbumID(s string) string { return numericID(s) }

// numericID returns s when it is a usable upstream numeric id (non-zero).
func numericID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 18 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	if strings.Trim(s, "0") == "" {
		return ""
	}
	return s
}

// parseKuwoAlbumInfo extracts album artist, release date and (when present)
// the album name from an album/song detail page.
func parseKuwoAlbumInfo(body string) *AlbumInfo {
	info := &AlbumInfo{
		AlbumName:   kuwoAlbumName(body),
		Artist:      kuwoAlbumArtist(body),
		ReleaseDate: kuwoReleaseDate(body),
	}
	if len(info.ReleaseDate) >= 4 {
		if year, err := strconv.Atoi(info.ReleaseDate[:4]); err == nil {
			info.Year = year
		}
	}
	return info
}

// kuwoAlbumArtist reads the album-level artist credit. Multi-artist credits are
// joined with the library's " • " separator.
func kuwoAlbumArtist(body string) string {
	m := kuwoArtistBlock.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	names := make([]string, 0, 4)
	for _, span := range kuwoSpan.FindAllStringSubmatch(m[1], -1) {
		attrs := span[1]
		// Badges such as "独家"/MV markers live in sibling spans; skip them.
		if strings.Contains(attrs, "isUgc") || strings.Contains(attrs, "iconfont") ||
			strings.Contains(attrs, "has_mv") {
			continue
		}
		if name := cleanHTMLText(span[2]); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		if name := cleanHTMLText(m[1]); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(dedupStrings(names), " • ")
}

// kuwoAlbumName reads the album title from the album page header.
func kuwoAlbumName(body string) string {
	m := kuwoSongName.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return cleanHTMLText(m[1])
}

// kuwoReleaseDate reads the value shown after the "发行时间" label.
//
// Tags are stripped before matching: the surrounding markup contains ids such as
// data-v-eba2071e, whose digits would otherwise be mistaken for a year.
func kuwoReleaseDate(body string) string {
	text := kuwoInfoText(body)
	idx := strings.Index(text, "发行时间")
	if idx < 0 {
		return ""
	}
	seg := []rune(text[idx:])
	if len(seg) > albumInfoDateWindow {
		seg = seg[:albumInfoDateWindow]
	}
	m := kuwoDate.FindStringSubmatch(string(seg))
	if m == nil {
		return ""
	}
	out := m[1]
	if m[2] != "" {
		out += fmt.Sprintf("-%02d", atoiOrZero(m[2]))
		if m[3] != "" {
			out += fmt.Sprintf("-%02d", atoiOrZero(m[3]))
		}
	}
	return out
}

// kuwoInfoText returns the page's song_info paragraph as plain text, falling
// back to the whole document when the paragraph is missing.
func kuwoInfoText(body string) string {
	if m := kuwoInfoBlock.FindStringSubmatch(body); m != nil {
		return cleanHTMLText(m[1])
	}
	return cleanHTMLText(body)
}

// cleanHTMLText strips tags, unescapes entities and collapses whitespace.
func cleanHTMLText(s string) string {
	s = kuwoTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(spaceRun.ReplaceAllString(s, " "))
}

func dedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// ---- wiring into the ingest / scrape pipelines ----

// WithAlbumInfo enables album-level enrichment (album artist + release year)
// for ingest and scrape. Without a provider both keep working as before; the
// two fields are then simply not written.
func (ing *Ingestor) WithAlbumInfo(p AlbumInfoProvider) *Ingestor {
	ing.albumInfo = p
	return ing
}

// albumInfoFor resolves album metadata for a download/scrape target.
//
// albumID is usually known (search results carry it), but older APP builds and
// existing queued tasks only send the song, so the source is asked to recover
// the album id from the song id. Best-effort: nil means "write without the
// album-level fields".
func (ing *Ingestor) albumInfoFor(ctx context.Context, albumID, songID, name, artist string) *AlbumInfo {
	if ing.albumInfo == nil {
		return nil
	}
	id := numericID(albumID)
	if id == "" {
		id = ing.lookupAlbumID(ctx, songID, name, artist)
	}
	if id == "" && numericID(songID) == "" {
		return nil
	}
	info, err := ing.albumInfo.AlbumInfo(ctx, id, songID)
	if err != nil {
		log.Debug(ctx, "online: album metadata unavailable", "albumId", id, "songId", songID, err)
		return nil
	}
	if info == nil {
		return nil
	}
	return info
}

// lookupAlbumID re-searches the source to recover the album id of a song we
// already know (clients that do not send albumId).
func (ing *Ingestor) lookupAlbumID(ctx context.Context, songID, name, artist string) string {
	if ing.lyrics == nil {
		return ""
	}
	name, artist = CleanSongTitle(name), NormalizeArtist(artist)
	keyword := strings.TrimSpace(name + " " + strings.NewReplacer(" / ", " ", " • ", " ", "&", " ").Replace(artist))
	if keyword == "" {
		return ""
	}
	res, err := ing.lyrics.SearchSong(ctx, SearchRequest{Keyword: keyword, Page: 1, PageSize: 10})
	if err != nil || res == nil {
		return ""
	}
	wantKey := NormalizeSongKey(name, artist)
	bestID, bestKey := "", ""
	for i := range res.Songs {
		s := &res.Songs[i]
		id := numericID(s.AlbumID)
		if id == "" {
			continue
		}
		// Same upstream song id: authoritative.
		if songID != "" && s.ID == songID {
			return id
		}
		if NormalizeSongKey(CleanSongTitle(s.Name), NormalizeArtist(s.Artist)) == wantKey {
			return id
		}
		if bestID == "" {
			bestID, bestKey = id, NormalizeSongKey(CleanSongTitle(s.Name), "")
		}
	}
	if bestID != "" && bestKey == NormalizeSongKey(name, "") {
		return bestID
	}
	return ""
}

// applyAlbumInfo fills the album-level fields the song-level scrape cannot know.
// Existing values always win: a rescrape must not overwrite tags it did not
// produce better data for.
func applyAlbumInfo(meta *EmbeddedMeta, info *AlbumInfo) {
	if info == nil {
		return
	}
	if meta.AlbumArtist == "" && info.Artist != "" {
		meta.AlbumArtist = NormalizeArtist(info.Artist)
	}
	if meta.Year == "" && info.Year > 0 {
		meta.Year = info.ReleaseDate
		if meta.Year == "" {
			meta.Year = strconv.Itoa(info.Year)
		}
	}
	if meta.Album == "" && info.AlbumName != "" {
		meta.Album = info.AlbumName
	}
}
