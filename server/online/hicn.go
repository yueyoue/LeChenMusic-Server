package online

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	Register(NewHicnSource(hicnDefaultBaseURL))
}

const (
	hicnDefaultBaseURL = "https://flac.music.hi.cn"
	hicnURLCacheTTL    = time.Hour
	hicnMaxAttempts    = 3
)

// hicnUserAgents rotates per request to look like ordinary browser traffic.
var hicnUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:127.0) Gecko/20100101 Firefox/127.0",
}

// hicnSource implements Source against the flac.music.hi.cn aggregation site.
//
// The site exposes a small form-encoded JSON API (no public docs):
//
//	POST /ajax.php?act=search   platform, keyword, page, size
//	POST /ajax.php?act=getUrl   platform, songid, format, bitrate, time, sign
//	POST /ajax.php?act=getLyric platform, songid, time, sign
//
// All responses share the envelope {"code":0,"msg":"ok","data":...}.
// The whole upstream protocol is isolated in this file so that upstream
// changes only require edits here.
type hicnSource struct {
	baseURL     string
	client      *http.Client
	minInterval time.Duration

	mu       sync.Mutex
	lastCall time.Time

	urlCache sync.Map // cache key -> cachedURL
}

type cachedURL struct {
	media   *ResolvedMedia
	expires time.Time
}

// NewHicnSource creates a hicn download source. baseURL is overridable for tests.
func NewHicnSource(baseURL string) *hicnSource {
	if baseURL == "" {
		baseURL = hicnDefaultBaseURL
	}
	return &hicnSource{
		baseURL:     strings.TrimRight(baseURL, "/"),
		client:      &http.Client{Timeout: 60 * time.Second},
		minInterval: 2 * time.Second,
	}
}

func (s *hicnSource) Name() string        { return "hicn" }
func (s *hicnSource) DisplayName() string { return "无损音乐下载" }

// ---- API envelope & payload shapes ----

type hicnEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type hicnSearchData struct {
	List  []hicnSong `json:"list"`
	Total int        `json:"total"`
}

type hicnSong struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Artist    string      `json:"artist"`
	AlbumName string      `json:"album_name"`
	PicURL    string      `json:"pic_url"`
	Duration  string      `json:"duration"` // seconds, stringly typed upstream
	Minfo     []hicnMinfo `json:"minfo"`
	Link      string      `json:"link"`
	Time      int64       `json:"time"`
	Sign      string      `json:"sign"`
}

type hicnMinfo struct {
	Level   string `json:"level"`
	Bitrate string `json:"bitrate"`
	Format  string `json:"format"`
	Size    string `json:"size"`
}

type hicnURLData struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Bitrate  int    `json:"bitrate"`
	Duration int    `json:"duration"`
}

// ---- Source implementation ----

func (s *hicnSource) SearchSong(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	keyword := strings.TrimSpace(req.Keyword)
	if keyword == "" {
		return nil, fmt.Errorf("online/hicn: empty keyword")
	}
	page, size := req.Page, req.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	data, err := s.post(ctx, "search", map[string]string{
		"platform": "kuwo",
		"keyword":  keyword,
		"page":     strconv.Itoa(page),
		"size":     strconv.Itoa(size),
	})
	if err != nil {
		return nil, err
	}

	var payload hicnSearchData
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("online/hicn: parse search data: %w", err)
	}

	result := &SearchResult{Songs: make([]Song, 0, len(payload.List)), Total: payload.Total}
	for _, hs := range payload.List {
		result.Songs = append(result.Songs, s.normalizeSong(hs))
	}
	return result, nil
}

func (s *hicnSource) normalizeSong(hs hicnSong) Song {
	duration, _ := strconv.Atoi(strings.TrimSpace(hs.Duration))
	qualities := make([]QualityOption, 0, len(hs.Minfo))
	for _, m := range hs.Minfo {
		bitrate, err := strconv.Atoi(strings.TrimSpace(m.Bitrate))
		if err != nil {
			continue
		}
		qualities = append(qualities, QualityOption{
			Format:  strings.ToLower(strings.TrimSpace(m.Format)),
			Bitrate: bitrate,
			Level:   m.Level,
			Size:    m.Size,
		})
	}
	return Song{
		Source:    "hicn",
		Platform:  "kuwo",
		ID:        hs.ID,
		Name:      hs.Name,
		Artist:    hs.Artist,
		AlbumName: hs.AlbumName,
		CoverURL:  hs.PicURL,
		Duration:  duration,
		Qualities: qualities,
		Link:      hs.Link,
		Time:      hs.Time,
		Sign:      hs.Sign,
	}
}

func (s *hicnSource) ResolveURL(ctx context.Context, song SongRef, quality Quality) (*ResolvedMedia, error) {
	if song.ID == "" {
		return nil, fmt.Errorf("online/hicn: empty song id")
	}
	if quality.IsZero() {
		quality = DefaultQualityChain[0]
	}

	if cached, ok := s.urlCache.Load(urlCacheKey(song, quality)); ok {
		if entry, ok := cached.(cachedURL); ok && time.Now().Before(entry.expires) {
			return entry.media, nil
		}
	}

	data, err := s.post(ctx, "getUrl", map[string]string{
		"platform": song.Platform,
		"songid":   song.ID,
		"format":   quality.Format,
		"bitrate":  strconv.Itoa(quality.Bitrate),
		"time":     strconv.FormatInt(song.Time, 10),
		"sign":     song.Sign,
	})
	if err != nil {
		return nil, err
	}

	var payload hicnURLData
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("online/hicn: parse url data: %w", err)
	}
	if payload.URL == "" {
		return nil, fmt.Errorf("online/hicn: empty media url")
	}

	media := &ResolvedMedia{
		URL:      payload.URL,
		Format:   payload.Format,
		Bitrate:  payload.Bitrate,
		Duration: payload.Duration,
	}
	s.urlCache.Store(urlCacheKey(song, quality), cachedURL{media: media, expires: time.Now().Add(hicnURLCacheTTL)})
	return media, nil
}

func urlCacheKey(song SongRef, q Quality) string {
	return song.Platform + "|" + song.ID + "|" + q.Format + "|" + strconv.Itoa(q.Bitrate)
}

func (s *hicnSource) GetLyrics(ctx context.Context, song SongRef) (string, error) {
	if song.ID == "" {
		return "", fmt.Errorf("online/hicn: empty song id")
	}
	data, err := s.post(ctx, "getLyric", map[string]string{
		"platform": song.Platform,
		"songid":   song.ID,
		"time":     strconv.FormatInt(song.Time, 10),
		"sign":     song.Sign,
	})
	if err != nil {
		return "", err
	}

	// The upstream lyric payload shape is not documented; accept a plain
	// string or a small object with a well-known key.
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		return asString, nil
	}
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(data, &asObject); err == nil {
		for _, key := range []string{"lyric", "lrc", "content", "text"} {
			if raw, ok := asObject[key]; ok {
				var value string
				if err := json.Unmarshal(raw, &value); err == nil {
					return value, nil
				}
			}
		}
	}
	return "", fmt.Errorf("online/hicn: unrecognized lyric payload")
}

// ---- transport ----

// post performs a form-encoded POST against the upstream ajax endpoint,
// enforcing a minimum interval between calls and decoding the response envelope.
// Transient failures (network, 429, 5xx) are retried with exponential backoff.
func (s *hicnSource) post(ctx context.Context, act string, params map[string]string) (json.RawMessage, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	var lastErr error
	for attempt := 0; attempt < hicnMaxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 2s, 4s
			backoff += time.Duration(rand.Int63n(int64(backoff/2) + 1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		data, retryable, err := s.postOnce(ctx, act, form)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("online/hicn: %s failed after %d attempts: %w", act, hicnMaxAttempts, lastErr)
}

// postOnce performs a single request. The second return value reports whether
// the failure is transient (worth retrying).
func (s *hicnSource) postOnce(ctx context.Context, act string, form url.Values) (json.RawMessage, bool, error) {
	if err := s.throttle(ctx); err != nil {
		return nil, false, err
	}

	endpoint := s.baseURL + "/ajax.php?act=" + url.QueryEscape(act)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("online/hicn: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Referer", s.baseURL+"/")
	req.Header.Set("Origin", s.baseURL)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", hicnUserAgents[rand.Intn(len(hicnUserAgents))])

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("online/hicn: request %s: %w", act, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, fmt.Errorf("online/hicn: read response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("online/hicn: %s returned status %d", act, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("online/hicn: %s returned status %d", act, resp.StatusCode)
	}

	var env hicnEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, false, fmt.Errorf("online/hicn: parse envelope: %w", err)
	}
	if env.Code != 0 {
		return nil, false, fmt.Errorf("online/hicn: %s failed: code=%d msg=%s", act, env.Code, env.Msg)
	}
	return env.Data, false, nil
}

// throttle enforces a jittered minimum interval between upstream calls
// (spacing ∈ [minInterval, 1.5×minInterval)) to avoid predictable request patterns.
func (s *hicnSource) throttle(ctx context.Context) error {
	interval := s.minInterval + time.Duration(rand.Int63n(int64(s.minInterval/2)+1))
	s.mu.Lock()
	wait := interval - time.Since(s.lastCall)
	if wait > 0 {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		s.mu.Lock()
	}
	s.lastCall = time.Now()
	s.mu.Unlock()
	return nil
}
