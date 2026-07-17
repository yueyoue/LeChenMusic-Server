package scanner

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	taglib "go.senan.xyz/taglib"
)

// [LeChenMusic-START:audiobook]

var audiobookAudioExts = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".flac": true,
	".ogg": true, ".wav": true, ".opus": true, ".wma": true, ".aac": true,
}

var audiobookCoverNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png",
	"folder.jpg", "folder.jpeg", "folder.png",
}

var genreKeywords = map[string]string{
	"有声书":   "有声读物",
	"有声读物": "有声读物",
	"小说":    "有声读物",
	"评书":    "评书",
	"相声":    "相声",
	"戏曲":    "戏曲",
	"儿童":    "儿童",
	"教育":    "教育",
}

type AudiobookScanner struct {
	ds model.DataStore
}

func NewAudiobookScanner(ds model.DataStore) *AudiobookScanner {
	return &AudiobookScanner{ds: ds}
}

func (s *AudiobookScanner) ScanLibrary(ctx context.Context, library model.Library) error {
	log.Info(ctx, "Audiobook scanner: Starting scan", "library", library.Name, "path", library.Path)

	repo := s.ds.Audiobook(ctx)
	var scanned, created, updated int

	err := filepath.WalkDir(library.Path, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if path == library.Path {
			return nil
		}

		// Check if this directory contains audio files
		hasAudio := false
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() && audiobookAudioExts[strings.ToLower(filepath.Ext(e.Name()))] {
				hasAudio = true
				break
			}
		}
		if !hasAudio {
			return nil
		}

		// This directory is an audiobook
		relPath, _ := filepath.Rel(library.Path, path)
		bookHash := audiobookHash(relPath)

		// Check if already exists
		existing, existErr := repo.GetAll(model.QueryOptions{
			Filters: Eq{"library_id": library.ID, "path": relPath},
		})
		if existErr == nil && len(existing) > 0 {
			book := existing[0]
			if book.Hash != bookHash || book.ChapterCount == 0 {
				book.Hash = bookHash
				// Re-read narrator from tags if empty
				if book.Narrator == "" {
					path := filepath.Join(library.Path, book.Path)
					_, _, _, _, _, tagNarr := readFirstAudioFileTags(path)
					if tagNarr != "" {
						book.Narrator = tagNarr
					}
				}
				s.scanChapters(ctx, &book, library, repo)
				if err := repo.Put(&book); err != nil {
					log.Error(ctx, "Audiobook scanner: Error updating", "book", book.Title, err)
				} else {
					updated++
				}
			}
			scanned++
			return filepath.SkipDir
		}

		// Create new audiobook
		book := s.createAudiobookFromDir(ctx, library, path, relPath, bookHash)
		// IMPORTANT: Save the book FIRST before scanning chapters,
		// because audiobook_chapter has a foreign key referencing audiobook(id).
		// If the book doesn't exist in DB yet, chapter inserts will fail.
		if err := repo.Put(&book); err != nil {
			log.Error(ctx, "Audiobook scanner: Error creating", "book", book.Title, err)
			return nil
		}
		s.scanChapters(ctx, &book, library, repo)
		created++
		scanned++
		return filepath.SkipDir
	})

	if err != nil {
		log.Error(ctx, "Audiobook scanner: Walk error", err)
	}

	log.Info(ctx, "Audiobook scanner: Scan complete", "scanned", scanned, "created", created, "updated", updated)
	return nil
}

func (s *AudiobookScanner) createAudiobookFromDir(ctx context.Context, library model.Library, fullPath, relPath, bookHash string) model.Audiobook {
	dirName := filepath.Base(fullPath)
	author, title := parseAudiobookDirName(dirName)
	genre := detectGenreFromPath(relPath)

	// [LeChenMusic-START:audiobook-id3-tags]
	// Try to read metadata from the first audio file's ID3 tags
	tagArtist, tagTitle, tagAlbum, tagGenre, tagYear, tagNarrator := readFirstAudioFileTags(fullPath)
	if tagTitle != "" {
		stripped := stripChapterSuffix(tagTitle)
		if stripped != "" && !isNumericOnly(stripped) && len([]rune(stripped)) > 1 {
			if title == dirName {
				title = stripped
			}
		}
	}
	if tagAlbum != "" && title == "" {
		title = tagAlbum
	}
	if tagGenre != "" && genre == "有声读物" {
		genre = tagGenre
	}
	var year int
	if tagYear > 0 {
		year = tagYear
	}
	// In audiobook files, ARTIST tag typically contains the narrator (演播者), not the author (作者).
	// Priority for narrator: explicit narrator tag > ARTIST/ALBUMARTIST
	// Priority for author: directory name parsing only (ARTIST is NOT used as author)
	narrator := tagNarrator
	if narrator == "" && tagArtist != "" {
		narrator = tagArtist
	}
	// [LeChenMusic-END:audiobook-id3-tags]

	coverPath := ""
	for _, coverName := range audiobookCoverNames {
		coverFile := filepath.Join(fullPath, coverName)
		if _, err := os.Stat(coverFile); err == nil {
			relCover, _ := filepath.Rel(library.Path, coverFile)
			coverPath = relCover
			break
		}
	}

	return model.Audiobook{
		ID:        id.NewRandom(),
		LibraryID: library.ID,
		Title:     title,
		Author:    author,
		Narrator:  narrator,
		Genre:     genre,
		Year:      year,
		CoverPath: coverPath,
		Path:      relPath,
		Hash:      bookHash,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (s *AudiobookScanner) scanChapters(ctx context.Context, book *model.Audiobook, library model.Library, repo model.AudiobookRepository) {
	_ = repo.DeleteChapters(book.ID)

	audiobookPath := filepath.Join(library.Path, book.Path)
	entries, err := os.ReadDir(audiobookPath)
	if err != nil {
		log.Error(ctx, "Audiobook scanner: Error reading dir", "path", audiobookPath, err)
		return
	}

	type audioFile struct {
		name string
		path string
	}
	var audioFiles []audioFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if audiobookAudioExts[ext] {
			audioFiles = append(audioFiles, audioFile{name: e.Name(), path: e.Name()})
		}
	}

	sort.Slice(audioFiles, func(i, j int) bool {
		return audioFiles[i].name < audioFiles[j].name
	})

	var totalSize int64
	var successCount int
	for i, af := range audioFiles {
		chapterPath := filepath.Join(audiobookPath, af.name)
		chapterTitle := strings.TrimSuffix(af.name, filepath.Ext(af.name))
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(af.name)), ".")
		var fileSize int64
		if info, err := os.Stat(chapterPath); err == nil {
			fileSize = info.Size()
		}
		// [LeChenMusic-START:audiobook-id3-tags]
		// Try to read chapter title and duration from ID3 tags
		var chapterDuration int
		if file, err := os.Open(chapterPath); err == nil {
			if f, err := taglib.OpenStream(file, taglib.WithReadStyle(taglib.ReadStyleFast), taglib.WithFilename(chapterPath)); err == nil {
				allTags := f.AllTags()
				props := f.Properties()
				f.Close()
				if v, ok := allTags.Tags["TITLE"]; ok && len(v) > 0 && v[0] != "" {
					chapterTitle = v[0]
				}
				if props.Length > 0 {
					chapterDuration = int(props.Length.Seconds())
				}
			}
			file.Close()
		}
		// [LeChenMusic-END:audiobook-id3-tags]

		chapter := model.AudiobookChapter{
			ID:            id.NewRandom(),
			AudiobookID:   book.ID,
			Title:         chapterTitle,
			ChapterNumber: i + 1,
			Duration:      chapterDuration,
			Format:        format,
			FileSize:      fileSize,
			Path:          af.path,
			CreatedAt:     time.Now(),
		}
		if err := repo.PutChapter(&chapter); err != nil {
			log.Error(ctx, "Audiobook scanner: Error saving chapter", "chapter", chapter.Title, err)
		} else {
			successCount++
			totalSize += fileSize
		}
	}

	// [LeChenMusic-START:audiobook-id3-tags]
	// Recalculate total duration from chapters
	var totalDuration int
	chapters, _ := repo.GetChapters(book.ID)
	for _, ch := range chapters {
		totalDuration += ch.Duration
	}
	book.ChapterCount = successCount
	book.TotalDuration = totalDuration
	book.Size = totalSize
	// [LeChenMusic-END:audiobook-id3-tags]
	if book.Title == "" {
		book.Title = filepath.Base(audiobookPath)
	}
}

// [LeChenMusic-START:audiobook-id3-tags]
// readFirstAudioFileTags reads ID3/metadata tags from the first audio file in a directory.
// Returns (artist, title, album, genre, year, narrator). Empty strings/zeros if not found.
// Note: In audiobook files, ARTIST/ALBUMARTIST typically contains the narrator, not the book author.
func readFirstAudioFileTags(dirPath string) (artist, title, album, genre string, year int, narrator string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !audiobookAudioExts[ext] {
			continue
		}
		filePath := filepath.Join(dirPath, e.Name())
		// Use taglib to read tags
		file, err := os.Open(filePath)
		if err != nil {
			continue
		}
		f, err := taglib.OpenStream(file, taglib.WithReadStyle(taglib.ReadStyleFast), taglib.WithFilename(filePath))
		if err != nil {
			file.Close()
			continue
		}
		allTags := f.AllTags()
		f.Close()
		file.Close()
		// Extract common tag fields (taglib returns UPPERCASE keys)
		tags := allTags.Tags
		if v, ok := tags["ARTIST"]; ok && len(v) > 0 {
			artist = v[0]
		}
		if v, ok := tags["TITLE"]; ok && len(v) > 0 {
			title = v[0]
		}
		if v, ok := tags["ALBUM"]; ok && len(v) > 0 {
			album = v[0]
		}
		if v, ok := tags["GENRE"]; ok && len(v) > 0 {
			genre = v[0]
		}
		if v, ok := tags["DATE"]; ok && len(v) > 0 {
			if y, parseErr := strconv.Atoi(v[0]); parseErr == nil {
				year = y
			}
		}
		// Also try ALBUMARTIST for author
		if artist == "" {
			if v, ok := tags["ALBUMARTIST"]; ok && len(v) > 0 {
				artist = v[0]
			}
		}
		// Read narrator from multiple tag sources (common for audiobooks)
		// Priority: COMPOSER > CONDUCTOR > DIRECTOR > TXXX:NARRATOR > TXXX:ARTISTSORT
		narratorSources := []string{"COMPOSER", "CONDUCTOR", "DIRECTOR", "TXXX:NARRATOR"}
		for _, tag := range narratorSources {
			if v, ok := tags[tag]; ok && len(v) > 0 && v[0] != "" {
				narrator = v[0]
				break
			}
		}
		return
	}
	return
}
// [LeChenMusic-END:audiobook-id3-tags]

func audiobookHash(path string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(path)))
}

// isNumericOnly checks if a string contains only digits (and optional leading/trailing whitespace)
func isNumericOnly(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// stripChapterSuffix removes common chapter/episode number suffixes from a title.
// This fixes cases where ID3 TITLE tags contain "BookName-01" or "BookName_01" instead of just "BookName".
// IMPORTANT: Only strips numbers that follow clear separators (-, _, space, parentheses).
// Does NOT strip bare numbers appended to text (e.g. "鬼吹灯1" stays as-is, since "1" is part of the book name).
func stripChapterSuffix(title string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return trimmed
	}

	// Helper: check if a string is a chapter-like number (1-4 digits)
	isChapterNum := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" || len(s) > 4 {
			return false
		}
		n, err := strconv.Atoi(s)
		return err == nil && n >= 1 && n <= 9999
	}

	// Only process if the title has a clear separator before the number
	// Pattern 1: "Title-01" (dash followed by digits at end)
	if idx := strings.LastIndex(trimmed, "-"); idx > 0 {
		suffix := strings.TrimSpace(trimmed[idx+1:])
		if isChapterNum(suffix) {
			result := strings.TrimSpace(trimmed[:idx])
			if result != "" {
				return result
			}
		}
	}

	// Pattern 2: "Title_01" (underscore followed by digits at end)
	if idx := strings.LastIndex(trimmed, "_"); idx > 0 {
		suffix := strings.TrimSpace(trimmed[idx+1:])
		if isChapterNum(suffix) {
			result := strings.TrimSpace(trimmed[:idx])
			if result != "" {
				return result
			}
		}
	}

	// Pattern 3: "Title 01" (space followed by digits at end)
	lastSpace := strings.LastIndex(trimmed, " ")
	if lastSpace > 0 {
		suffix := trimmed[lastSpace+1:]
		if isChapterNum(suffix) {
			result := strings.TrimSpace(trimmed[:lastSpace])
			if result != "" {
				return result
			}
		}
	}

	// Pattern 4: "Title(01)" or "Title(01)"
	for _, pair := range []struct{ open, close string }{
		{"(", ")"}, {"（", "）"},
	} {
		closeIdx := strings.LastIndex(trimmed, pair.close)
		if closeIdx == len(trimmed)-len(pair.close) {
			openIdx := strings.LastIndex(trimmed[:closeIdx], pair.open)
			if openIdx > 0 {
				numStr := strings.TrimSpace(trimmed[openIdx+len(pair.open) : closeIdx])
				if isChapterNum(numStr) {
					result := strings.TrimSpace(trimmed[:openIdx])
					if result != "" {
						return result
					}
				}
			}
		}
	}

	// Pattern 5: "第01章" or "第1章" (Chinese chapter markers)
	if idx := strings.LastIndex(trimmed, "章"); idx > 0 && idx == len(trimmed)-len("章") {
		prefix := trimmed[:idx]
		if diIdx := strings.LastIndex(prefix, "第"); diIdx >= 0 {
			numPart := strings.TrimSpace(prefix[diIdx+len("第"):])
			if isChapterNum(numPart) {
				result := strings.TrimSpace(prefix[:diIdx])
				if result != "" {
					return result
				}
			}
		}
	}

	// Pattern 6: Bare numbers with leading zeros (e.g. "贝姨01" → "贝姨")
	// Numbers with leading zeros (01, 02, 001, 002) are almost always chapter numbers.
	// Single digits without leading zeros (like "鬼吹灯1") are part of the book name.
	numStart := -1
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] >= '0' && trimmed[i] <= '9' {
			numStart = i
		} else {
			break
		}
	}
	if numStart > 0 {
		numStr := trimmed[numStart:]
		if len(numStr) >= 2 && numStr[0] == '0' {
			result := strings.TrimSpace(trimmed[:numStart])
			if result != "" && !isNumericOnly(result) {
				return result
			}
		}
	}

	return trimmed
}

func parseAudiobookDirName(name string) (author, title string) {
	// Pattern 1: "Author - Title"
	if idx := strings.Index(name, " - "); idx > 0 {
		return strings.TrimSpace(name[:idx]), strings.TrimSpace(name[idx+3:])
	}
	// Pattern 2: "Title (Author)"
	if start := strings.LastIndex(name, "("); start > 0 {
		if end := strings.LastIndex(name, ")"); end > start {
			return strings.TrimSpace(name[start+1 : end]), strings.TrimSpace(name[:start])
		}
	}
	// Pattern 3: "X Title Narrator [XX回]" (Chinese audiobook common pattern)
	// e.g. "B 贝姨 艾宝良 48回" → title="贝姨", narrator="艾宝良"
	// e.g. "G 鬼吹灯 艾宝良" → title="鬼吹灯", narrator="艾宝良"
	parts := strings.Fields(name)
	if len(parts) >= 3 {
		first := parts[0]
		if len(first) == 1 && ((first[0] >= 'A' && first[0] <= 'Z') || (first[0] >= 'a' && first[0] <= 'z')) {
			remaining := strings.TrimSpace(name[len(first):])
			parts2 := strings.Fields(remaining)
			if len(parts2) >= 2 {
				lastPart := parts2[len(parts2)-1]
				if strings.HasSuffix(lastPart, "回") || strings.HasSuffix(lastPart, "集") || strings.HasSuffix(lastPart, "集全") {
					if len(parts2) >= 3 {
						return strings.Join(parts2[len(parts2)-2:len(parts2)-1], " "), strings.Join(parts2[:len(parts2)-2], " ")
					}
					return "", strings.Join(parts2[:len(parts2)-1], " ")
				}
				if len(parts2) >= 3 {
					return strings.Join(parts2[len(parts2)-1:], " "), strings.Join(parts2[:len(parts2)-1], " ")
				}
			}
		}
	}
	return "", name
}

func detectGenreFromPath(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for i := 0; i < len(parts)-1; i++ {
		if genre, ok := genreKeywords[parts[i]]; ok {
			return genre
		}
	}
	return "有声读物"
}

// [LeChenMusic-END:audiobook]
