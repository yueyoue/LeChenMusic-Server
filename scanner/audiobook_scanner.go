package scanner

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/core/storage"
	// The scanner resolves library paths through core/storage; the local backend registers
	// itself in init(). Importing it here keeps the "file" scheme available no matter which
	// binary pulls the scanner in.
	_ "github.com/navidrome/navidrome/core/storage/local"
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
	"有声书":  "有声读物",
	"有声读物": "有声读物",
	"小说":   "有声读物",
	"评书":   "评书",
	"相声":   "相声",
	"戏曲":   "戏曲",
	"儿童":   "儿童",
	"教育":   "教育",
}

type AudiobookScanner struct {
	ds model.DataStore
}

func NewAudiobookScanner(ds model.DataStore) *AudiobookScanner {
	return &AudiobookScanner{ds: ds}
}

// audiobookFS resolves the library's storage backend through the storage abstraction
// (core/storage), so a local folder and a cloud source (openlist://...) are scanned with
// exactly the same code. ContextualStorage (cloud) must be cancelled with the scan.
func audiobookFS(ctx context.Context, library model.Library) (storage.MusicFS, error) {
	st, err := storage.For(library.Path)
	if err != nil {
		return nil, err
	}
	if cs, ok := st.(storage.ContextualStorage); ok {
		return cs.FSWithContext(ctx)
	}
	return st.FS()
}

// openForTag opens a file through the library FS and hands back a seekable reader for
// taglib. On a cloud source the handle is a lazy Range reader, so tag parsing only pulls
// the byte ranges taglib actually asks for — never the whole file (design doc §15.1).
func openForTag(fsys storage.MusicFS, filePath string) (io.ReadSeeker, io.Closer, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		_ = f.Close()
		return nil, nil, fmt.Errorf("audiobook: %s is not seekable", filePath)
	}
	return rs, f, nil
}

func (s *AudiobookScanner) ScanLibrary(ctx context.Context, library model.Library) error {
	log.Info(ctx, "Audiobook scanner: Starting scan", "library", library.Name, "path", library.Path)

	fsys, err := audiobookFS(ctx, library)
	if err != nil {
		log.Error(ctx, "Audiobook scanner: Cannot open library storage", "path", library.Path, err)
		return err
	}

	repo := s.ds.Audiobook(ctx)
	var scanned, created, updated int

	// fs.WalkDir walks the storage abstraction (io/fs), so every path below is
	// library-relative and always uses "/" as separator — for local and cloud alike.
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}

		// Check if this directory contains audio files
		hasAudio := false
		entries, readErr := fs.ReadDir(fsys, p)
		if readErr != nil {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() && audiobookAudioExts[strings.ToLower(path.Ext(e.Name()))] {
				hasAudio = true
				break
			}
		}
		if !hasAudio {
			return nil
		}

		// This directory is an audiobook
		relPath := p
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
					_, _, _, _, _, tagNarr := readFirstAudioFileTags(fsys, book.Path)
					if tagNarr != "" {
						book.Narrator = tagNarr
					}
				}
				s.scanChapters(ctx, fsys, &book, repo)
				if err := repo.Put(&book); err != nil {
					log.Error(ctx, "Audiobook scanner: Error updating", "book", book.Title, err)
				} else {
					updated++
				}
			}
			scanned++
			return fs.SkipDir
		}

		// Create new audiobook
		book := s.createAudiobookFromDir(ctx, fsys, library, relPath, bookHash)
		// IMPORTANT: Save the book FIRST before scanning chapters,
		// because audiobook_chapter has a foreign key referencing audiobook(id).
		// If the book doesn't exist in DB yet, chapter inserts will fail.
		if err := repo.Put(&book); err != nil {
			log.Error(ctx, "Audiobook scanner: Error creating", "book", book.Title, err)
			return nil
		}
		s.scanChapters(ctx, fsys, &book, repo)
		// scanChapters fills ChapterCount/TotalDuration/Size in on the book struct. Persist
		// them here, or the DB keeps the zero values: the API then reports 0 chapters and the
		// "ChapterCount == 0" guard above re-reads every chapter tag on every single scan.
		if err := repo.Put(&book); err != nil {
			log.Error(ctx, "Audiobook scanner: Error saving chapter counters", "book", book.Title, err)
		}
		created++
		scanned++
		return fs.SkipDir
	})

	if err != nil {
		log.Error(ctx, "Audiobook scanner: Walk error", err)
	}

	log.Info(ctx, "Audiobook scanner: Scan complete", "scanned", scanned, "created", created, "updated", updated)
	return nil
}

func (s *AudiobookScanner) createAudiobookFromDir(ctx context.Context, fsys storage.MusicFS, library model.Library, relPath, bookHash string) model.Audiobook {
	dirName := path.Base(relPath)
	author, title := parseAudiobookDirName(dirName)
	genre := detectGenreFromPath(relPath)

	// [LeChenMusic-START:audiobook-id3-tags]
	// Try to read metadata from the first audio file's ID3 tags
	tagArtist, tagTitle, tagAlbum, tagGenre, tagYear, tagNarrator := readFirstAudioFileTags(fsys, relPath)
	if tagTitle != "" {
		stripped := stripChapterSuffix(tagTitle)
		if stripped != "" && !isNumericOnly(stripped) && len([]rune(stripped)) > 1 {
			if title == dirName {
				title = stripped
			}
		}
	}
	if tagAlbum != "" && (title == "" || title == dirName) {
		// Use ALBUM tag as fallback, but only if it's a meaningful title
		if !isNumericOnly(tagAlbum) && len([]rune(tagAlbum)) > 1 {
			title = tagAlbum
		}
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
		coverFile := path.Join(relPath, coverName)
		if _, err := fs.Stat(fsys, coverFile); err == nil {
			coverPath = coverFile
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

func (s *AudiobookScanner) scanChapters(ctx context.Context, fsys storage.MusicFS, book *model.Audiobook, repo model.AudiobookRepository) {
	_ = repo.DeleteChapters(book.ID)

	audiobookPath := book.Path
	entries, err := fs.ReadDir(fsys, audiobookPath)
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
		ext := strings.ToLower(path.Ext(e.Name()))
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
		chapterPath := path.Join(audiobookPath, af.name)
		chapterTitle := strings.TrimSuffix(af.name, path.Ext(af.name))
		format := strings.TrimPrefix(strings.ToLower(path.Ext(af.name)), ".")
		var fileSize int64
		if info, err := fs.Stat(fsys, chapterPath); err == nil {
			fileSize = info.Size()
		}
		// [LeChenMusic-START:audiobook-id3-tags]
		// Try to read chapter title and duration from ID3 tags
		var chapterDuration int
		if rs, closer, err := openForTag(fsys, chapterPath); err == nil {
			if f, err := taglib.OpenStream(rs, taglib.WithReadStyle(taglib.ReadStyleFast), taglib.WithFilename(chapterPath)); err == nil {
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
			closer.Close()
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
		book.Title = path.Base(audiobookPath)
	}
}

// [LeChenMusic-START:audiobook-id3-tags]
// readFirstAudioFileTags reads ID3/metadata tags from the first audio file in a directory.
// Returns (artist, title, album, genre, year, narrator). Empty strings/zeros if not found.
// Note: In audiobook files, ARTIST/ALBUMARTIST typically contains the narrator, not the book author.
func readFirstAudioFileTags(fsys storage.MusicFS, dirPath string) (artist, title, album, genre string, year int, narrator string) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(e.Name()))
		if !audiobookAudioExts[ext] {
			continue
		}
		filePath := path.Join(dirPath, e.Name())
		// Use taglib to read tags, through the storage abstraction (lazy Range reads on cloud)
		rs, closer, err := openForTag(fsys, filePath)
		if err != nil {
			continue
		}
		f, err := taglib.OpenStream(rs, taglib.WithReadStyle(taglib.ReadStyleFast), taglib.WithFilename(filePath))
		if err != nil {
			closer.Close()
			continue
		}
		allTags := f.AllTags()
		f.Close()
		closer.Close()
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

	// Pattern 4: "Title(01)" or "Title(01)" or "Title(48集)" or "Title[48回]"
	for _, pair := range []struct{ open, close string }{
		{"(", ")"}, {"（", "）"}, {"[", "]"}, {"【", "】"},
	} {
		closeIdx := strings.LastIndex(trimmed, pair.close)
		if closeIdx == len(trimmed)-len(pair.close) {
			openIdx := strings.LastIndex(trimmed[:closeIdx], pair.open)
			if openIdx > 0 {
				numStr := strings.TrimSpace(trimmed[openIdx+len(pair.open) : closeIdx])
				// Strip trailing 回/集/集全 suffix for chapter number detection
				cleanNum := numStr
				for _, suffix := range []string{"集全", "集", "回"} {
					cleanNum = strings.TrimSuffix(cleanNum, suffix)
				}
				cleanNum = strings.TrimSpace(cleanNum)
				if isChapterNum(cleanNum) {
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
	// Pattern 1: "Author - Title" (with spaces around dash)
	if idx := strings.Index(name, " - "); idx > 0 {
		return strings.TrimSpace(name[:idx]), strings.TrimSpace(name[idx+3:])
	}

	// Pattern 2: "Title (Author)" or "Title（Author）"
	for _, pair := range []struct{ open, close string }{
		{"(", ")"}, {"（", "）"},
	} {
		closeIdx := strings.LastIndex(name, pair.close)
		if closeIdx > 0 {
			openIdx := strings.LastIndex(name[:closeIdx], pair.open)
			if openIdx > 0 {
				authorPart := strings.TrimSpace(name[openIdx+len(pair.open) : closeIdx])
				titlePart := strings.TrimSpace(name[:openIdx])
				if authorPart != "" && titlePart != "" {
					return authorPart, titlePart
				}
			}
		}
	}

	// Pattern 3: "数字_演播者评书《书名》" e.g. "07_单田芳评书《楚汉争雄》"
	if idx := strings.Index(name, "_"); idx > 0 {
		prefix := name[:idx]
		rest := strings.TrimSpace(name[idx+1:])
		isNumPrefix := true
		for _, c := range prefix {
			if c < '0' || c > '9' {
				isNumPrefix = false
				break
			}
		}
		if isNumPrefix && rest != "" {
			// Try to extract title from 《》 brackets
			if start := strings.Index(rest, "《"); start >= 0 {
				if end := strings.Index(rest[start:], "》"); end > 0 {
					extractedTitle := strings.TrimSpace(rest[start+3 : start+end])
					authorPart := strings.TrimSpace(rest[:start])
					if extractedTitle != "" {
						return authorPart, extractedTitle
					}
				}
			}
			// No brackets: split by spaces, last word might be narrator
			parts := strings.Fields(rest)
			if len(parts) >= 2 {
				return strings.Join(parts[1:], " "), parts[0]
			}
			return "", rest
		}
	}

	// Pattern 4: "X Title Narrator" (Chinese audiobook common pattern)
	// e.g. "B 贝姨 艾宝良 48回" → narrator="艾宝良", title="贝姨"
	// e.g. "G 鬼吹灯 艾宝良" → narrator="艾宝良", title="鬼吹灯"
	// e.g. "Z 蜘蛛+十宗罪_艾宝良" → narrator="艾宝良", title="蜘蛛+十宗罪"
	// e.g. "G《古镜魂迷》17集 EBC5版" → title="古镜魂迷"
	parts := strings.Fields(name)
	if len(parts) >= 1 {
		first := parts[0]
		if len(first) == 1 && ((first[0] >= 'A' && first[0] <= 'Z') || (first[0] >= 'a' && first[0] <= 'z')) {
			remaining := strings.TrimSpace(name[len(first):])
			// Handle "X《书名》..." format (letter directly attached to brackets)
			if strings.HasPrefix(remaining, "《") {
				if endIdx := strings.Index(remaining, "》"); endIdx > 0 {
					extractedTitle := strings.TrimSpace(remaining[3:endIdx])
					if extractedTitle != "" {
						return "", extractedTitle
					}
				}
			}
			// Handle underscore-separated narrator: "蜘蛛+十宗罪_艾宝良"
			if usIdx := strings.LastIndex(remaining, "_"); usIdx > 0 {
				potentialNarrator := strings.TrimSpace(remaining[usIdx+1:])
				potentialTitle := strings.TrimSpace(remaining[:usIdx])
				if potentialNarrator != "" && potentialTitle != "" {
					return potentialNarrator, potentialTitle
				}
			}
			parts2 := strings.Fields(remaining)
			if len(parts2) >= 3 {
				// Has 回/集 suffix → last part before suffix is narrator
				lastPart := parts2[len(parts2)-1]
				if strings.HasSuffix(lastPart, "回") || strings.HasSuffix(lastPart, "集") || strings.HasSuffix(lastPart, "集全") {
					if len(parts2) >= 4 {
						return strings.Join(parts2[len(parts2)-2:len(parts2)-1], " "), strings.Join(parts2[:len(parts2)-2], " ")
					}
					return "", strings.Join(parts2[:len(parts2)-1], " ")
				}
				// No suffix: last part is narrator, rest is title
				return strings.Join(parts2[len(parts2)-1:], " "), strings.Join(parts2[:len(parts2)-1], " ")
			}
			if len(parts2) == 2 {
				// "X Title Narrator" → title=parts2[0], narrator=parts2[1]
				return parts2[1], parts2[0]
			}
		}
	}

	// Pattern 5: "书名_演播者" (underscore separator, no leading letter)
	if idx := strings.LastIndex(name, "_"); idx > 0 {
		potentialTitle := strings.TrimSpace(name[:idx])
		potentialNarrator := strings.TrimSpace(name[idx+1:])
		if potentialTitle != "" && potentialNarrator != "" && !isNumericOnly(potentialNarrator) {
			return potentialNarrator, potentialTitle
		}
	}

	// Default: entire name is title
	return "", name
}

func detectGenreFromPath(relPath string) string {
	parts := strings.Split(relPath, "/")
	for i := 0; i < len(parts)-1; i++ {
		if genre, ok := genreKeywords[parts[i]]; ok {
			return genre
		}
	}
	return "有声读物"
}

// [LeChenMusic-END:audiobook]
