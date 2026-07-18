package nativeapi

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
)

func (api *Router) addDuplicateSongsRoute(r chi.Router) {
	r.Get("/song/duplicates", func(w http.ResponseWriter, r *http.Request) {
		h := &duplicateSongsHandler{ds: api.ds}
		h.findDuplicates(w, r)
	})
}

type duplicateSongsHandler struct {
	ds model.DataStore
}

type duplicateGroup struct {
	Title  string          `json:"title"`
	Artist string          `json:"artist"`
	Count  int             `json:"count"`
	Songs  []duplicateSong `json:"songs"`
}

type duplicateSong struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float32 `json:"duration"`
	BitRate  int     `json:"bitRate"`
	Size     int64   `json:"size"`
	Suffix   string  `json:"suffix"`
	Path     string  `json:"path"`
	Year     int     `json:"year"`
}

func (h *duplicateSongsHandler) findDuplicates(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.MediaFile(r.Context())
	songs, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// 按 title+artist 分组
	type groupKey struct {
		Title  string
		Artist string
	}
	groups := make(map[groupKey][]duplicateSong)

	for _, s := range songs {
		key := groupKey{
			Title:  normalizeForDuplicate(s.Title),
			Artist: normalizeForDuplicate(s.Artist),
		}
		if key.Title == "" {
			continue
		}
		groups[key] = append(groups[key], duplicateSong{
			ID:       s.ID,
			Title:    s.Title,
			Artist:   s.Artist,
			Album:    s.Album,
			Duration: s.Duration,
			BitRate:  s.BitRate,
			Size:     s.Size,
			Suffix:   s.Suffix,
			Path:     s.Path,
			Year:     s.Year,
		})
	}

	// 筛选出有重复的组
	var duplicates []duplicateGroup
	for key, groupSongs := range groups {
		if len(groupSongs) > 1 {
			// 按路径排序，方便用户对比
			sort.Slice(groupSongs, func(i, j int) bool {
				return groupSongs[i].Path < groupSongs[j].Path
			})
			duplicates = append(duplicates, duplicateGroup{
				Title:  groupSongs[0].Title,
				Artist: groupSongs[0].Artist,
				Count:  len(groupSongs),
				Songs:  groupSongs,
			})
		}
	}

	// 按重复数量降序排列
	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].Count > duplicates[j].Count
	})

	writeJSON(w, map[string]any{"data": duplicates})
}

// normalizeForDuplicate 用于重复检测的字符串规范化
func normalizeForDuplicate(s string) string {
	if s == "" {
		return ""
	}
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	result := s[start:end]
	if result == "" {
		return ""
	}
	// 转小写
	b := make([]byte, len(result))
	for i := 0; i < len(result); i++ {
		c := result[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
