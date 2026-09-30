package nativeapi

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func (api *Router) addDuplicateSongsRoute(r chi.Router) {
	r.Get("/song/duplicates", func(w http.ResponseWriter, r *http.Request) {
		h := &duplicateSongsHandler{ds: api.ds}
		h.findDuplicates(w, r)
	})
	r.Get("/song/cross-source-duplicates", func(w http.ResponseWriter, r *http.Request) {
		h := &duplicateSongsHandler{ds: api.ds}
		h.findCrossSourceDuplicates(w, r)
	})
	r.Post("/song/duplicates/delete", func(w http.ResponseWriter, r *http.Request) {
		h := &duplicateSongsHandler{ds: api.ds}
		h.deleteSongs(w, r)
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
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	Duration    float32 `json:"duration"`
	BitRate     int     `json:"bitRate"`
	Size        int64   `json:"size"`
	Suffix      string  `json:"suffix"`
	Path        string  `json:"path"`
	Year        int     `json:"year"`
	LibraryPath string  `json:"libraryPath"`
	Source      string  `json:"source"` // local | cloud
}

// crossSourceGroup is a set of songs that look like the same audio file (identical size
// and duration) living in more than one media source — e.g. the same book/album kept both
// on a local folder and on a cloud drive. 评审 §4.6 P2-8: this is a HINT only — nothing is
// ever auto-deleted here; the user decides which copy (if any) to remove.
type crossSourceGroup struct {
	Size     int64           `json:"size"`
	Duration int             `json:"duration"`
	Count    int             `json:"count"`
	Sources  []string        `json:"sources"` // distinct sources in the group (local/cloud)
	Songs    []duplicateSong `json:"songs"`
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
			ID:          s.ID,
			Title:       s.Title,
			Artist:      s.Artist,
			Album:       s.Album,
			Duration:    s.Duration,
			BitRate:     s.BitRate,
			Size:        s.Size,
			Suffix:      s.Suffix,
			Path:        s.Path,
			Year:        s.Year,
			LibraryPath: s.LibraryPath,
			Source:      sourceKind(s.LibraryPath),
		})
	}

	// 筛选出有重复的组
	var duplicates []duplicateGroup
	for _, groupSongs := range groups {
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

// findCrossSourceDuplicates reports groups of songs that look like the same audio file
// (coarse match: identical size + duration) living in more than one media source — e.g.
// the same album kept both in a local folder and on a cloud drive.
//
// 评审 §4.6 P2-8：纯元数据比对，**不哈希、不下载文件内容**（红线 §15.1），
// 且只出提示、不自动删除；删不删、删哪份由用户自己决定。
func (h *duplicateSongsHandler) findCrossSourceDuplicates(w http.ResponseWriter, r *http.Request) {
	repo := h.ds.MediaFile(r.Context())
	songs, err := repo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type matchKey struct {
		Size     int64
		Duration int
	}
	groups := make(map[matchKey][]duplicateSong)
	for _, s := range songs {
		if s.Size <= 0 {
			continue // 没有有效指纹，不参与匹配
		}
		key := matchKey{Size: s.Size, Duration: int(math.Round(float64(s.Duration)))}
		groups[key] = append(groups[key], duplicateSong{
			ID:          s.ID,
			Title:       s.Title,
			Artist:      s.Artist,
			Album:       s.Album,
			Duration:    s.Duration,
			BitRate:     s.BitRate,
			Size:        s.Size,
			Suffix:      s.Suffix,
			Path:        s.Path,
			Year:        s.Year,
			LibraryPath: s.LibraryPath,
			Source:      sourceKind(s.LibraryPath),
		})
	}

	var result []crossSourceGroup
	for key, groupSongs := range groups {
		if len(groupSongs) < 2 {
			continue
		}
		// 只报“跨媒体源”的组：同库内的重复由标题+艺人的重复视图负责
		libSet := map[string]bool{}
		sourceSet := map[string]bool{}
		for _, s := range groupSongs {
			libSet[s.LibraryPath] = true
			sourceSet[s.Source] = true
		}
		if len(libSet) < 2 {
			continue
		}
		sources := make([]string, 0, len(sourceSet))
		for src := range sourceSet {
			sources = append(sources, src)
		}
		sort.Strings(sources)
		sort.Slice(groupSongs, func(i, j int) bool {
			// 本地优先（给“优先保留本地”的提示做铺垫），同类型按路径稳定排序
			if groupSongs[i].Source != groupSongs[j].Source {
				return groupSongs[i].Source == "local"
			}
			return groupSongs[i].Path < groupSongs[j].Path
		})
		result = append(result, crossSourceGroup{
			Size:     key.Size,
			Duration: key.Duration,
			Count:    len(groupSongs),
			Sources:  sources,
			Songs:    groupSongs,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Size > result[j].Size
	})

	writeJSON(w, map[string]any{"data": result})
}

// sourceKind classifies a library path as "cloud" (openlist://…) or "local" — same
// criterion as core/storage.IsRemoteURI and the UI SourceTag (评审 §4.6 P2-3).
func sourceKind(libraryPath string) string {
	if storage.IsRemoteURI(libraryPath) {
		return "cloud"
	}
	return "local"
}

func (h *duplicateSongsHandler) deleteSongs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "no IDs provided", 400)
		return
	}

	repo := h.ds.MediaFile(r.Context())
	libRepo := h.ds.Library(r.Context())
	var deleted, failed int
	var errMsgs []string

	for _, id := range req.IDs {
		// Get file info before deleting
		mf, err := repo.Get(id)
		if err != nil {
			log.Warn(r.Context(), "Duplicate delete: song not found", "id", id, "error", err)
			failed++
			errMsgs = append(errMsgs, "歌曲未找到: "+id)
			continue
		}

		// Resolve absolute path: prefer AbsolutePath(), fallback to library lookup
		filePath := mf.AbsolutePath()
		if filePath == "" || filePath == "." {
			// Fallback: look up library path manually
			libs, libErr := libRepo.GetAll()
			if libErr == nil {
				for _, lib := range libs {
					if lib.ID == mf.LibraryID {
						filePath = filepath.Join(lib.Path, mf.Path)
						break
					}
				}
			}
		}

		log.Info(r.Context(), "Duplicate delete: attempting", "id", id, "path", filePath, "libraryId", mf.LibraryID, "libraryPath", mf.LibraryPath, "relativePath", mf.Path)

		// Delete the actual file from disk FIRST
		if filePath != "" && filePath != "." {
			if err := os.Remove(filePath); err != nil {
				if os.IsNotExist(err) {
					log.Info(r.Context(), "Duplicate delete: file already gone", "path", filePath)
				} else {
					log.Warn(r.Context(), "Duplicate delete: file delete failed", "path", filePath, "error", err)
					errMsgs = append(errMsgs, "文件删除失败("+filePath+"): "+err.Error())
					failed++
					continue // skip DB delete if file can't be removed
				}
			} else {
				log.Info(r.Context(), "Duplicate delete: file deleted", "path", filePath)
			}
		} else {
			log.Warn(r.Context(), "Duplicate delete: could not resolve file path", "id", id)
			errMsgs = append(errMsgs, "无法解析文件路径: "+id)
			failed++
			continue
		}

		// Delete from DB
		if err := repo.Delete(id); err != nil {
			log.Warn(r.Context(), "Duplicate delete: DB delete failed", "id", id, "error", err)
			failed++
			errMsgs = append(errMsgs, "数据库删除失败: "+id)
			continue
		}

		deleted++
	}

	log.Info(r.Context(), "Duplicate delete completed", "deleted", deleted, "failed", failed)
	writeJSON(w, map[string]any{
		"success": true,
		"deleted": deleted,
		"failed":  failed,
		"errors":  errMsgs,
	})
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
