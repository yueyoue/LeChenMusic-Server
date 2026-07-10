package nativeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// [LeChenMusic-START:ai-playlist]

// ==================== 数据结构 ====================

type externalSong struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Source string `json:"source"`
}

func (s externalSong) MatchKey() string {
	t := cleanMatchStr(s.Title)
	a := cleanMatchStr(s.Artist)
	return t + "|" + a
}

type matchResult struct {
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	ID        string `json:"id"`
	Source    string `json:"source"`
	MatchType string `json:"matchType,omitempty"`
}

type unmatchedSong struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Source string `json:"source"`
}

type searchResponse struct {
	Query        string                  `json:"query"`
	SourceStats  map[string]int          `json:"sourceStats"`
	SearchTotal  int                     `json:"searchTotal"`
	Matched      []matchResult           `json:"matched"`
	MatchedCount int                     `json:"matchedCount"`
	Unmatched    []unmatchedSong         `json:"unmatched"`
	UnmatchedCount int                   `json:"unmatchedCount"`
}

type createPlaylistRequest struct {
	Name        string   `json:"name"`
	SongIDs     []string `json:"songIds"`
	CoverTheme string   `json:"coverTheme,omitempty"`
	CoverEnabled bool   `json:"coverEnabled"`
}

// ==================== 搜索引擎 ====================

var httpClient = &http.Client{Timeout: 15 * time.Second}

func cleanMatchStr(s string) string {
	re := regexp.MustCompile(`[\s\-\(\)（）\[\]【】「」《》]`)
	s = re.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	re2 := regexp.MustCompile(`(?i)(feat\.?|ft\.?|合唱|对唱|live|remix|cover|翻唱|伴奏|dj.*版|完整版|&|、|，)`)
	s = re2.ReplaceAllString(s, "")
	return s
}

func searchKuwo(keyword string, limit int) []externalSong {
	var songs []externalSong
	url := fmt.Sprintf("http://search.kuwo.cn/r.s?all=%s&ft=music&rformat=json&encoding=utf8&pn=0&rn=%d", keyword, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Warn("AI Playlist: Kuwo search failed", "error", err)
		return songs
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := strings.ReplaceAll(string(body), "'", "\"")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	var data struct {
		Abslist []struct {
			SongName string `json:"SONGNAME"`
			Artist   string `json:"ARTIST"`
			Album    string `json:"ALBUM"`
		} `json:"abslist"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		log.Warn("AI Playlist: Kuwo parse failed", "error", err)
		return songs
	}
	for _, item := range data.Abslist {
		title := strings.TrimSpace(item.SongName)
		artist := strings.TrimSpace(item.Artist)
		album := strings.TrimSpace(item.Album)
		if title != "" && artist != "" {
			songs = append(songs, externalSong{Title: title, Artist: artist, Album: album, Source: "酷我"})
		}
	}
	return songs
}

func searchNetease(keyword string, limit int) []externalSong {
	var songs []externalSong
	url := "http://music.163.com/api/search/get/web"
	data := fmt.Sprintf("s=%s&type=1&limit=%d&offset=0", keyword, limit)
	req, _ := http.NewRequest("POST", url, strings.NewReader(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://music.163.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Warn("AI Playlist: Netease search failed", "error", err)
		return songs
	}
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Songs []struct {
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return songs
	}
	for _, item := range result.Result.Songs {
		artists := make([]string, len(item.Artists))
		for i, a := range item.Artists {
			artists[i] = a.Name
		}
		artist := strings.Join(artists, "/")
		if item.Name != "" && artist != "" {
			songs = append(songs, externalSong{Title: item.Name, Artist: artist, Album: item.Album.Name, Source: "网易云"})
		}
	}
	return songs
}

func searchQQ(keyword string, limit int) []externalSong {
	var songs []externalSong
	url := fmt.Sprintf("https://c.y.qq.com/soso/fcgi-bin/search_for_qq_cp?w=%s&format=json&p=1&n=%d&cr=1", keyword, limit)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Warn("AI Playlist: QQ search failed", "error", err)
		return songs
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Song struct {
				List []struct {
					SongName string `json:"songname"`
					Singer   []struct {
						Name string `json:"name"`
					} `json:"singer"`
					AlbumName string `json:"albumname"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return songs
	}
	for _, item := range result.Data.Song.List {
		artists := make([]string, len(item.Singer))
		for i, s := range item.Singer {
			artists[i] = s.Name
		}
		artist := strings.Join(artists, "/")
		if item.SongName != "" && artist != "" {
			songs = append(songs, externalSong{Title: item.SongName, Artist: artist, Album: item.AlbumName, Source: "QQ音乐"})
		}
	}
	return songs
}

func searchKugou(keyword string, limit int) []externalSong {
	var songs []externalSong
	url := fmt.Sprintf("http://mobilecdn.kugou.com/api/v3/search/song?keyword=%s&format=json&page=1&pagesize=%d", keyword, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Warn("AI Playlist: Kugou search failed", "error", err)
		return songs
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Info []struct {
				SongName   string `json:"songname"`
				SingerName string `json:"singername"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return songs
	}
	for _, item := range result.Data.Info {
		title := item.SongName
		if idx := strings.Index(title, " - "); idx > 0 {
			title = title[:idx]
		}
		if title != "" && item.SingerName != "" {
			songs = append(songs, externalSong{Title: title, Artist: item.SingerName, Source: "酷狗"})
		}
	}
	return songs
}

type searcherFunc func(string, int) []externalSong

var allSearchers = []struct {
	Name string
	Fn   searcherFunc
}{
	{"酷我", searchKuwo},
	{"网易云", searchNetease},
	{"QQ音乐", searchQQ},
	{"酷狗", searchKugou},
}

// ==================== 歌单链接解析 ====================

var playlistURLPatterns = []struct {
	Platform string
	Regex    *regexp.Regexp
}{
	{"netease", regexp.MustCompile(`music\.163\.com.*?playlist[/\?].*?id=(\d+)`)},
	{"netease", regexp.MustCompile(`music\.163\.com/playlist/(\d+)`)},
	{"qq", regexp.MustCompile(`y\.qq\.com.*?playlist/(\d+)`)},
	{"qq", regexp.MustCompile(`y\.qq\.com.*?id=(\d+)`)},
	{"kuwo", regexp.MustCompile(`kuwo\.cn/playlist(?:_detail)?/(\d+)`)},
	{"kuwo", regexp.MustCompile(`kuwo\.cn.*?pid=(\d+)`)},
	{"kugou", regexp.MustCompile(`kugou\.com.*?special/(\d+)`)},
	{"kugou", regexp.MustCompile(`kugou\.com.*?code=(\w+)`)},
}

func parsePlaylistURL(url string) (platform, id string) {
	for _, p := range playlistURLPatterns {
		if m := p.Regex.FindStringSubmatch(url); len(m) > 1 {
			return p.Platform, m[1]
		}
	}
	return "", ""
}

func fetchPlaylistFromURL(url string) (string, []externalSong, error) {
	platform, pid := parsePlaylistURL(url)
	if platform == "" || pid == "" {
		return "", nil, fmt.Errorf("无法识别此链接格式，请确认是网易云/QQ音乐/酷我/酷狗的歌单链接")
	}
	switch platform {
	case "netease":
		return fetchNeteasePlaylist(pid)
	case "qq":
		return fetchQQPlaylist(pid)
	case "kuwo":
		return fetchKuwoPlaylist(pid)
	case "kugou":
		return fetchKugouPlaylist(pid)
	}
	return "", nil, fmt.Errorf("不支持的平台: %s", platform)
}

func fetchNeteasePlaylist(pid string) (string, []externalSong, error) {
	url := fmt.Sprintf("http://music.163.com/api/playlist/detail?id=%s", pid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://music.163.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Playlist struct {
			Name   string `json:"name"`
			Tracks []struct {
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
			} `json:"tracks"`
		} `json:"playlist"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}
	name := result.Playlist.Name
	if name == "" {
		name = "网易云歌单"
	}
	var songs []externalSong
	for _, t := range result.Playlist.Tracks {
		artists := make([]string, len(t.Artists))
		for i, a := range t.Artists {
			artists[i] = a.Name
		}
		artist := strings.Join(artists, "/")
		if t.Name != "" && artist != "" {
			songs = append(songs, externalSong{Title: t.Name, Artist: artist, Album: t.Album.Name, Source: "网易云"})
		}
	}
	return name, songs, nil
}

func fetchQQPlaylist(pid string) (string, []externalSong, error) {
	url := fmt.Sprintf("https://c.y.qq.com/v8/fcg-bin/fcg_v8_playlist_cp.fcg?disstid=%s&type=1&json=1&utf8=1&onlysong=0&new_format=1&loginUin=0&hostUin=0&format=json&inCharset=utf-8&outCharset=utf-8&notice=0&platform=yqq.json&needNewCode=0", pid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Cdlist []struct {
				Dissname string `json:"dissname"`
				Songlist []struct {
					Name      string `json:"name"`
					SongName  string `json:"songname"`
					Singer    []struct {
						Name string `json:"name"`
					} `json:"singer"`
					SingerName string `json:"singername"`
					Album      struct {
						Name string `json:"name"`
					} `json:"album"`
					AlbumName string `json:"albumname"`
				} `json:"songlist"`
			} `json:"cdlist"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}
	if len(result.Data.Cdlist) == 0 {
		return "QQ音乐歌单", nil, nil
	}
	cd := result.Data.Cdlist[0]
	name := cd.Dissname
	if name == "" {
		name = "QQ音乐歌单"
	}
	var songs []externalSong
	for _, item := range cd.Songlist {
		title := item.Name
		if title == "" {
			title = item.SongName
		}
		var artist string
		if len(item.Singer) > 0 {
			artists := make([]string, len(item.Singer))
			for i, s := range item.Singer {
				artists[i] = s.Name
			}
			artist = strings.Join(artists, "/")
		} else {
			artist = item.SingerName
		}
		album := item.Album.Name
		if album == "" {
			album = item.AlbumName
		}
		if title != "" && artist != "" {
			songs = append(songs, externalSong{Title: title, Artist: artist, Album: album, Source: "QQ音乐"})
		}
	}
	return name, songs, nil
}

func fetchKuwoPlaylist(pid string) (string, []externalSong, error) {
	url := fmt.Sprintf("http://www.kuwo.cn/playlist_detail/%s", pid)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	name := "酷我歌单"
	titleRe := regexp.MustCompile(`<title>(.*?)</title>`)
	if m := titleRe.FindStringSubmatch(text); len(m) > 1 {
		parts := strings.Split(m[1], "_")
		if len(parts) > 0 {
			name = strings.TrimSpace(parts[0])
		}
	}

	var songs []externalSong
	linkRe := regexp.MustCompile(`<a[^>]*title="([^"]+)"[^>]*href="/play_detail/(\d+)"[^>]*>`)
	artistRe := regexp.MustCompile(`class="song_artist"[^>]*>.*?<span[^>]*title="([^"]+)"`)

	matches := linkRe.FindAllStringSubmatchIndex(text, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		songName := text[m[2]:m[3]]
		after := text[m[1]:]
		if len(after) > 800 {
			after = after[:800]
		}
		artist := ""
		if am := artistRe.FindStringSubmatch(after); len(am) > 1 {
			artist = am[1]
			artist = strings.ReplaceAll(artist, "&amp;", "&")
			artist = strings.ReplaceAll(artist, "&lt;", "<")
			artist = strings.ReplaceAll(artist, "&gt;", ">")
		}
		if songName != "" && artist != "" {
			songs = append(songs, externalSong{Title: songName, Artist: artist, Source: "酷我"})
		}
	}
	return name, songs, nil
}

func fetchKugouPlaylist(pid string) (string, []externalSong, error) {
	url := fmt.Sprintf("http://mobilecdn.kugou.com/api/v3/special/song?specialid=%s&page=1&pagesize=100&plat=2&version=8970", pid)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Info []struct {
				SongName   string `json:"songname"`
				SingerName string `json:"singername"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}
	var songs []externalSong
	for _, item := range result.Data.Info {
		title := item.SongName
		if idx := strings.Index(title, " - "); idx > 0 {
			title = title[:idx]
		}
		if title != "" && item.SingerName != "" {
			songs = append(songs, externalSong{Title: title, Artist: item.SingerName, Source: "酷狗"})
		}
	}
	return "酷狗歌单", songs, nil
}

// ==================== 曲库匹配 ====================

func matchWithLibrary(externalSongs []externalSong, library []model.MediaFile) (matched []matchResult, unmatched []unmatchedSong) {
	// Build library index
	type libEntry struct {
		song model.MediaFile
		key  string
	}
	libIndex := make(map[string]libEntry)
	var libEntries []libEntry

	for _, s := range library {
		key := cleanMatchStr(s.Title) + "|" + cleanMatchStr(s.Artist)
		if _, exists := libIndex[key]; !exists {
			entry := libEntry{song: s, key: key}
			libIndex[key] = entry
			libEntries = append(libEntries, entry)
		}
	}

	// Deduplicate external songs
	seen := make(map[string]bool)
	var unique []externalSong
	for _, s := range externalSongs {
		key := s.MatchKey()
		if !seen[key] {
			seen[key] = true
			unique = append(unique, s)
		}
	}

	// Exact match first
	fuzzyCandidates := make(map[string]bool)
	for _, es := range unique {
		key := es.MatchKey()
		if entry, ok := libIndex[key]; ok {
			matched = append(matched, matchResult{
				Title:  entry.song.Title,
				Artist: entry.song.Artist,
				Album:  entry.song.Album,
				ID:     entry.song.ID,
				Source: es.Source,
			})
		} else {
			fuzzyCandidates[key] = true
			unmatched = append(unmatched, unmatchedSong{
				Title:  es.Title,
				Artist: es.Artist,
				Source: es.Source,
			})
		}
	}

	// Fuzzy match for unmatched songs
	if len(unmatched) > 0 {
		var fuzzyMatched []unmatchedSong
		for _, us := range unmatched {
			titleClean := cleanMatchStr(us.Title)
			if utf8.RuneCountInString(titleClean) < 2 {
				continue
			}
			found := false
			for _, entry := range libEntries {
				libTitle := cleanMatchStr(entry.song.Title)
				if titleClean != "" && libTitle != "" &&
					(strings.Contains(titleClean, libTitle) || strings.Contains(libTitle, titleClean)) {
					matched = append(matched, matchResult{
						Title:     entry.song.Title,
						Artist:    entry.song.Artist,
						Album:     entry.song.Album,
						ID:        entry.song.ID,
						Source:    us.Source + "(模糊)",
						MatchType: "fuzzy",
					})
					found = true
					break
				}
			}
			if !found {
				fuzzyMatched = append(fuzzyMatched, us)
			}
		}
		unmatched = fuzzyMatched
	}

	// Sort matched by source
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Source < matched[j].Source
	})

	return
}

// ==================== API Handler ====================

func (api *Router) addAIPlaylistRoute(r chi.Router) {
	r.Route("/ai-playlist", func(r chi.Router) {
		r.Post("/search", api.aiPlaylistSearch)
		r.Post("/match", api.aiPlaylistMatch)
		r.Post("/from-url", api.aiPlaylistFromURL)
		r.Post("/create", api.aiPlaylistCreate)
		r.Post("/cover/preview", api.aiPlaylistCoverPreview)
		r.Get("/cover/themes", api.aiPlaylistCoverThemes)
	})
}

func (api *Router) aiPlaylistSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query   string   `json:"query"`
		Sources []string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Query == "" {
		http.Error(w, "搜索关键词不能为空", 400)
		return
	}

	log.Info(r.Context(), "AI Playlist: Searching", "query", req.Query, "sources", req.Sources)

	type sourceResult struct {
		Name   string         `json:"name"`
		Songs  []externalSong `json:"songs"`
		Count  int            `json:"count"`
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var results []sourceResult

	for _, s := range allSearchers {
		if len(req.Sources) > 0 {
			found := false
			for _, src := range req.Sources {
				if src == s.Name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		wg.Add(1)
		go func(name string, fn searcherFunc) {
			defer wg.Done()
			songs := fn(req.Query, 30)
			mu.Lock()
			results = append(results, sourceResult{Name: name, Songs: songs, Count: len(songs)})
			mu.Unlock()
		}(s.Name, s.Fn)
	}
	wg.Wait()

	total := 0
	sourceStats := make(map[string]int)
	for _, r := range results {
		total += r.Count
		sourceStats[r.Name] = r.Count
	}

	writeJSON(w, map[string]any{
		"query":       req.Query,
		"sourceStats": sourceStats,
		"total":       total,
		"results":     results,
	})
}

func (api *Router) aiPlaylistMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query   string   `json:"query"`
		Sources []string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Query == "" {
		http.Error(w, "搜索关键词不能为空", 400)
		return
	}

	log.Info(r.Context(), "AI Playlist: Matching", "query", req.Query)

	// Search all platforms
	var allSongs []externalSong
	sourceStats := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, s := range allSearchers {
		if len(req.Sources) > 0 {
			found := false
			for _, src := range req.Sources {
				if src == s.Name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		wg.Add(1)
		go func(name string, fn searcherFunc) {
			defer wg.Done()
			songs := fn(req.Query, 30)
			mu.Lock()
			sourceStats[name] = len(songs)
			allSongs = append(allSongs, songs...)
			mu.Unlock()
		}(s.Name, s.Fn)
	}
	wg.Wait()

	// Get library
	mediaRepo := api.ds.MediaFile(r.Context())
	library, err := mediaRepo.GetAll(model.QueryOptions{})
	if err != nil {
		log.Error(r.Context(), "AI Playlist: Failed to get library", err)
		http.Error(w, "获取曲库失败", 500)
		return
	}

	// Match
	matched, unmatched := matchWithLibrary(allSongs, library)

	// Limit unmatched to 50
	if len(unmatched) > 50 {
		unmatched = unmatched[:50]
	}

	writeJSON(w, searchResponse{
		Query:          req.Query,
		SourceStats:    sourceStats,
		SearchTotal:    len(allSongs),
		Matched:        matched,
		MatchedCount:   len(matched),
		Unmatched:      unmatched,
		UnmatchedCount: len(unmatched),
	})
}

func (api *Router) aiPlaylistFromURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.URL == "" {
		http.Error(w, "链接不能为空", 400)
		return
	}

	log.Info(r.Context(), "AI Playlist: Importing from URL", "url", req.URL)

	playlistName, urlSongs, err := fetchPlaylistFromURL(req.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(urlSongs) == 0 {
		http.Error(w, "未能从该链接获取到歌曲", 400)
		return
	}

	// Get library
	mediaRepo := api.ds.MediaFile(r.Context())
	library, err := mediaRepo.GetAll(model.QueryOptions{})
	if err != nil {
		http.Error(w, "获取曲库失败", 500)
		return
	}

	// Match
	matched, unmatched := matchWithLibrary(urlSongs, library)
	if len(unmatched) > 50 {
		unmatched = unmatched[:50]
	}

	writeJSON(w, map[string]any{
		"playlistName":  playlistName,
		"source":        urlSongs[0].Source,
		"searchTotal":   len(urlSongs),
		"matched":       matched,
		"matchedCount":  len(matched),
		"unmatched":     unmatched,
		"unmatchedCount": len(unmatched),
	})
}

func (api *Router) aiPlaylistCreate(w http.ResponseWriter, r *http.Request) {
	var req createPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Name == "" {
		http.Error(w, "歌单名称不能为空", 400)
		return
	}
	if len(req.SongIDs) == 0 {
		http.Error(w, "歌曲列表不能为空", 400)
		return
	}

	log.Info(r.Context(), "AI Playlist: Creating playlist", "name", req.Name, "songs", len(req.SongIDs))

	// Use the existing playlist creation mechanism
	pls := api.playlists
	// Create takes (ctx, playlistId, name, ids) - empty playlistId means create new
	plID, err := pls.Create(r.Context(), "", req.Name, req.SongIDs)
	if err != nil {
		log.Error(r.Context(), "AI Playlist: Create failed", err)
		http.Error(w, "创建歌单失败: "+err.Error(), 500)
		return
	}

	// Generate and set cover if enabled
	if req.CoverEnabled {
		coverData, err := generatePlaylistCover(req.Name, len(req.SongIDs), req.CoverTheme)
		if err != nil {
			log.Warn(r.Context(), "AI Playlist: Cover generation failed", "error", err)
		} else {
			if err := pls.SetImage(r.Context(), plID, strings.NewReader(string(coverData)), "jpg"); err != nil {
				log.Warn(r.Context(), "AI Playlist: Set cover failed", "error", err)
			}
		}
	}

	writeJSON(w, map[string]any{
		"success":    true,
		"playlistId": plID,
		"name":       req.Name,
		"songCount":  len(req.SongIDs),
	})
}

func (api *Router) aiPlaylistCoverPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Theme string `json:"theme"`
		Count int    `json:"songCount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}

	coverData, err := generatePlaylistCover(req.Title, req.Count, req.Theme)
	if err != nil {
		http.Error(w, "封面生成失败", 500)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(coverData)
}

func (api *Router) aiPlaylistCoverThemes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"themes": getAvailableThemes(),
	})
}

// [LeChenMusic-END:ai-playlist]
