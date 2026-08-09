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
	Name         string   `json:"name"`
	SongIDs      []string `json:"songIds"`
	CoverTheme   string   `json:"coverTheme,omitempty"`
	CoverEnabled bool     `json:"coverEnabled"`
	CoverURL     string   `json:"coverURL,omitempty"`
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
	{"汽水音乐", searchQishui},
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
	{"qq", regexp.MustCompile(`c\d*\.y\.qq\.com/base/fcgi-bin/u\?__=(\w+)`)},
	{"kuwo", regexp.MustCompile(`kuwo\.cn/playlist(?:_detail)?/(\d+)`)},
	{"kuwo", regexp.MustCompile(`kuwo\.cn.*?pid=(\d+)`)},
	{"kugou", regexp.MustCompile(`kugou\.com.*?special/(\d+)`)},
	{"kugou", regexp.MustCompile(`kugou\.com.*?code=(\w+)`)},
	{"qishui", regexp.MustCompile(`music\.douyin\.com/qishui/share/playlist\?.*?playlist_id=(\d+)`)},
	{"qishui", regexp.MustCompile(`qishui\.douyin\.com/s/(\w+)`)},
}

func parsePlaylistURL(url string) (platform, id string) {
	for _, p := range playlistURLPatterns {
		if m := p.Regex.FindStringSubmatch(url); len(m) > 1 {
			return p.Platform, m[1]
		}
	}
	return "", ""
}

func fetchPlaylistFromURL(url string) (string, string, []externalSong, error) {
	// 汽水音乐短链接需要先解析
	if strings.Contains(url, "qishui.douyin.com/s/") {
		realURL, err := resolveQishuiShortURL(url)
		if err == nil {
			url = realURL
		}
	}
	platform, pid := parsePlaylistURL(url)
	if platform == "" || pid == "" {
		return "", "", nil, fmt.Errorf("无法识别此链接格式，支持：网易云/QQ音乐/酷我/酷狗/汽水音乐歌单链接")
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
	case "qishui":
		return fetchQishuiPlaylist(pid)
	}
	return "", "", nil, fmt.Errorf("不支持的平台: %s", platform)
}

func fetchNeteasePlaylist(pid string) (string, string, []externalSong, error) {
	url := fmt.Sprintf("http://music.163.com/api/playlist/detail?id=%s", pid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://music.163.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Playlist struct {
			Name        string `json:"name"`
			CoverImgURL string `json:"coverImgUrl"`
			Tracks      []struct {
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
		return "", "", nil, err
	}
	name := result.Playlist.Name
	if name == "" {
		name = "网易云歌单"
	}
	coverURL := result.Playlist.CoverImgURL
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
	return name, coverURL, songs, nil
}

// isNumericID checks if a string is a pure numeric ID
func isNumericID(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// resolveQQShareAndFetch resolves a QQ Music share link token to a real playlist ID
func resolveQQShareAndFetch(token string) (string, string, []externalSong, error) {
	// Try to resolve the share link
	shareURL := fmt.Sprintf("https://c6.y.qq.com/base/fcgi-bin/u?__=%s", token)
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(shareURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("resolve QQ share link failed: %w", err)
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	if location == "" {
		return "", "", nil, fmt.Errorf("QQ share link returned no redirect")
	}
	// Extract playlist ID from the redirect URL
	re := regexp.MustCompile(`playlist/(\d+)`)
	m := re.FindStringSubmatch(location)
	if len(m) < 2 {
		return "", "", nil, fmt.Errorf("could not extract playlist ID from QQ share link: %s", location)
	}
	realPID := m[1]
	return fetchQQPlaylist(realPID)
}

func fetchQQPlaylist(pid string) (string, string, []externalSong, error) {
	// First check if pid looks like a share link token (not a numeric ID)
	if !isNumericID(pid) {
		return resolveQQShareAndFetch(pid)
	}
	url := fmt.Sprintf("https://c6.y.qq.com/qzone/fcg-bin/fcg_ucc_getcdinfo_byids_cp.fcg?type=1&json=1&utf8=1&onlysong=0&new_format=1&disstid=%s&platform=yqq.json&needNewCode=0", pid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Cdlist []struct {
			Dissname string `json:"dissname"`
			Logo     string `json:"logo"`
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", nil, err
	}
	if len(result.Cdlist) == 0 {
		return "QQ音乐歌单", "", nil, nil
	}
	cd := result.Cdlist[0]
	name := cd.Dissname
	if name == "" {
		name = "QQ音乐歌单"
	}
	coverURL := cd.Logo
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
	return name, coverURL, songs, nil
}

func fetchKuwoPlaylist(pid string) (string, string, []externalSong, error) {
	url := fmt.Sprintf("http://www.kuwo.cn/playlist_detail/%s", pid)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", "", nil, err
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

	// 提取封面图
	coverURL := ""
	coverRe := regexp.MustCompile(`class="imgbox"[^>]*>\s*<img[^>]*src="([^"]+)"`)
	if cm := coverRe.FindStringSubmatch(text); len(cm) > 1 {
		coverURL = cm[1]
	}
	if coverURL == "" {
		coverRe2 := regexp.MustCompile(`<meta[^>]*property="og:image"[^>]*content="([^"]+)"`)
		if cm2 := coverRe2.FindStringSubmatch(text); len(cm2) > 1 {
			coverURL = cm2[1]
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
	return name, coverURL, songs, nil
}

func fetchKugouPlaylist(pid string) (string, string, []externalSong, error) {
	url := fmt.Sprintf("http://mobilecdn.kugou.com/api/v3/special/song?specialid=%s&page=1&pagesize=100&plat=2&version=8970", pid)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", "", nil, err
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
		return "", "", nil, err
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
	return "酷狗歌单", "", songs, nil
}

// ==================== 汽水音乐 ====================

func searchQishui(keyword string, limit int) []externalSong {
	var songs []externalSong
	// 汽水音乐 Web 搜索 API
	searchURL := fmt.Sprintf("https://music.douyin.com/qishui/share/search?keyword=%s&count=%d", keyword, limit)
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("Referer", "https://music.douyin.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Warn("AI Playlist: Qishui search failed", "error", err)
		return songs
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			SongList []struct {
				Title  string `json:"title"`
				Author string `json:"author"`
				Album  string `json:"album"`
			} `json:"song_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// 尝试备用解析：从HTML提取
		text := string(body)
		titleRe := regexp.MustCompile(`"title":"([^"]+)"`)
		artistRe := regexp.MustCompile(`"author":"([^"]+)"`)
		titles := titleRe.FindAllStringSubmatch(text, -1)
		artists := artistRe.FindAllStringSubmatch(text, -1)
		for i := 0; i < len(titles) && i < limit; i++ {
			title := titles[i][1]
			artist := ""
			if i < len(artists) {
				artist = artists[i][1]
			}
			if title != "" {
				songs = append(songs, externalSong{Title: title, Artist: artist, Source: "汽水音乐"})
			}
		}
		return songs
	}
	for _, item := range result.Data.SongList {
		if item.Title != "" {
			songs = append(songs, externalSong{Title: item.Title, Artist: item.Author, Source: "汽水音乐"})
		}
	}
	return songs
}

// resolveQishuiShortURL 解析汽水音乐短链接获取真实URL
func resolveQishuiShortURL(shortURL string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 不自动跟随重定向
		},
	}
	resp, err := client.Get(shortURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	if location != "" {
		return location, nil
	}
	// 如果没有重定向，从页面中提取
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	re := regexp.MustCompile(`https?://music\.douyin\.com/qishui/share/playlist\?playlist_id=(\d+)`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return m[0], nil
	}
	return "", fmt.Errorf("无法解析短链接")
}

func fetchQishuiPlaylist(pid string) (string, string, []externalSong, error) {
	url := fmt.Sprintf("https://music.douyin.com/qishui/share/playlist?playlist_id=%s", pid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://music.douyin.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// 提取歌单名称
	name := "汽水音乐歌单"
	nameRe := regexp.MustCompile(`"playlist_name":"([^"]+)"`)
	if m := nameRe.FindStringSubmatch(text); len(m) > 1 {
		name = m[1]
	}
	if name == "汽水音乐歌单" {
		titleRe := regexp.MustCompile(`<title>(.*?)</title>`)
		if m := titleRe.FindStringSubmatch(text); len(m) > 1 {
			t := strings.TrimSpace(m[1])
			if t != "" && t != "汽水音乐" {
				name = t
			}
		}
	}

	// 提取封面
	coverURL := ""
	coverRe := regexp.MustCompile(`"cover_url":"([^"]+)"`)
	if m := coverRe.FindStringSubmatch(text); len(m) > 1 {
		coverURL = m[1]
	}
	if coverURL == "" {
		ogImageRe := regexp.MustCompile(`<meta[^>]*property="og:image"[^>]*content="([^"]+)"`)
		if m := ogImageRe.FindStringSubmatch(text); len(m) > 1 {
			coverURL = m[1]
		}
	}

	// 提取歌曲列表
	var songs []externalSong

	// 方式1: JSON数据 (song_list格式)
	songListRe := regexp.MustCompile(`"song_list":\[\{(.*?)\}\]`)
	if m := songListRe.FindStringSubmatch(text); len(m) > 0 {
		arrText := "[{" + m[1] + "}]"
		var items []struct {
			Title  string `json:"title"`
			Author string `json:"author"`
			Album  string `json:"album"`
		}
		if json.Unmarshal([]byte(arrText), &items) == nil {
			for _, item := range items {
				if item.Title != "" {
					songs = append(songs, externalSong{Title: item.Title, Artist: item.Author, Album: item.Album, Source: "汽水音乐"})
				}
			}
		}
	}

	// 方式2: SSR渲染的HTML格式
	// 汽水音乐页面是SSR渲染，歌曲数据在HTML中格式为:
	// <div>序号</div><div><div><p>歌名</p></div><div><p>歌手 • 专辑</p></div></div>
	if len(songs) == 0 {
		ssrRe := regexp.MustCompile(`>(\d{1,4})</div><div[^>]*><div[^>]*><p[^>]*>([^<]{1,200})</p></div><div[^>]*><p[^>]*>([^<]{1,300})</p></div>`)
		for _, m := range ssrRe.FindAllStringSubmatch(text, -1) {
			title := strings.TrimSpace(m[2])
			artistLine := strings.TrimSpace(m[3])
			artist := artistLine
			album := ""
			if idx := strings.Index(artistLine, " • "); idx > 0 {
				artist = strings.TrimSpace(artistLine[:idx])
				album = strings.TrimSpace(artistLine[idx+len(" • "):])
			}
			if title != "" && artist != "" && title != name {
				songs = append(songs, externalSong{Title: title, Artist: artist, Album: album, Source: "汽水音乐"})
			}
		}
	}

	// 方式3: 更宽泛的正则 - 匹配 "title":"xxx" 和 "author":"xxx" 模式
	if len(songs) == 0 {
		titles := regexp.MustCompile(`"title":"([^"]{2,200})"`).FindAllStringSubmatch(text, -1)
		artists := regexp.MustCompile(`"author":"([^"]{1,200})"`).FindAllStringSubmatch(text, -1)
		for i := 0; i < len(titles); i++ {
			title := titles[i][1]
			artist := ""
			if i < len(artists) {
				artist = artists[i][1]
			}
			if title != "汽水音乐" && title != name {
				songs = append(songs, externalSong{Title: title, Artist: artist, Source: "汽水音乐"})
			}
		}
	}

	return name, coverURL, songs, nil
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
		r.Post("/import-txt", api.aiPlaylistImportTXT)
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

	playlistName, coverURL, urlSongs, err := fetchPlaylistFromURL(req.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(urlSongs) == 0 {
		http.Error(w, "未能从该链接获取到歌曲。汽水音乐链接可能需要浏览器环境解析，建议使用TXT文件导入方式：先在浏览器打开歌单页面，将歌曲列表复制到TXT文件后导入", 400)
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
		"coverURL":      coverURL,
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

	// Set cover: prefer imported cover URL, fallback to generated cover
	if req.CoverURL != "" {
		coverReq, err := http.NewRequest("GET", req.CoverURL, nil)
		if err == nil {
			// 添加 Referer 头防止防盗链拦截
			coverReq.Header.Set("Referer", "https://music.163.com/")
			coverReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			coverResp, err := httpClient.Do(coverReq)
			if err == nil {
				defer coverResp.Body.Close()
				if coverResp.StatusCode == 200 {
					contentType := coverResp.Header.Get("Content-Type")
					ext := "jpg"
					if strings.Contains(contentType, "png") {
						ext = "png"
					} else if strings.Contains(contentType, "webp") {
						ext = "webp"
					}
					if err := pls.SetImage(r.Context(), plID, coverResp.Body, ext); err != nil {
						log.Warn(r.Context(), "AI Playlist: Set imported cover failed", "error", err)
					} else {
						log.Info(r.Context(), "AI Playlist: Imported cover set successfully", "url", req.CoverURL)
					}
				} else {
					log.Warn(r.Context(), "AI Playlist: Cover fetch HTTP error", "status", coverResp.StatusCode, "url", req.CoverURL)
				}
			} else {
				log.Warn(r.Context(), "AI Playlist: Cover fetch network error", "error", err, "url", req.CoverURL)
			}
		}
	} else if req.CoverEnabled {
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


// ==================== TXT文件导入 ====================

func (api *Router) aiPlaylistImportTXT(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB max
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "请上传TXT文件", 400)
		return
	}
	defer file.Close()

	// Validate file extension
	filename := header.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".txt") {
		http.Error(w, "只支持 .txt 格式的文件", 400)
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "读取文件失败", 500)
		return
	}

	text := string(content)
	lines := strings.Split(text, "\n")
	var parsed []externalSong

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Try multiple formats:
		// 1. "歌名 - 歌手"
		// 2. "歌名	歌手"
		// 3. "歌名|歌手"
		// 4. "歌手 - 歌名" (some exports use this)
		// 5. Just the song name (no artist)
		var title, artist string

		if strings.Contains(line, " - ") {
			parts := strings.SplitN(line, " - ", 2)
			title = strings.TrimSpace(parts[0])
			artist = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, "\t") {
			parts := strings.SplitN(line, "\t", 2)
			title = strings.TrimSpace(parts[0])
			artist = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, "|") {
			parts := strings.SplitN(line, "|", 2)
			title = strings.TrimSpace(parts[0])
			artist = strings.TrimSpace(parts[1])
		} else {
			title = line
		}

		// Remove leading numbers like "1. " or "1、"
		re := regexp.MustCompile(`^\d+[.、\)\]]\s*`)
		title = re.ReplaceAllString(title, "")

		if title != "" {
			parsed = append(parsed, externalSong{Title: title, Artist: artist, Source: "TXT导入"})
		}
	}

	if len(parsed) == 0 {
		http.Error(w, "未能从文件中解析出任何歌曲", 400)
		return
	}

	// Match with library
	mediaRepo := api.ds.MediaFile(r.Context())
	library, err := mediaRepo.GetAll(model.QueryOptions{})
	if err != nil {
		http.Error(w, "获取曲库失败", 500)
		return
	}

	matched, unmatched := matchWithLibrary(parsed, library)
	if len(unmatched) > 50 {
		unmatched = unmatched[:50]
	}

	// Derive playlist name from filename
	playlistName := strings.TrimSuffix(filename, ".txt")
	playlistName = strings.TrimSpace(playlistName)

	writeJSON(w, map[string]any{
		"playlistName":    playlistName,
		"source":          "TXT导入",
		"searchTotal":     len(parsed),
		"matched":         matched,
		"matchedCount":    len(matched),
		"unmatched":       unmatched,
		"unmatchedCount":  len(unmatched),
	})
}

// [LeChenMusic-END:ai-playlist]
