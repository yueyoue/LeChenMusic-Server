package scanner

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/core/audiobookcover"
	"github.com/navidrome/navidrome/core/cloudsource"
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

// 书目录/章节命名约定与封面落盘规则收敛在 core/audiobookcover，
// 与 server/nativeapi（上传/刮削/出图）同源。
var audiobookAudioExts = audiobookcover.AudioExts

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

// tagModeForLibrary resolves the gateway's tag-reading mode for a library path
// (评审 P2-11: "filename" = 云库快速模式，绝不读文件内容). Seam var so tests can stub it.
var tagModeForLibrary = cloudsource.TagModeForLibrary

func (s *AudiobookScanner) ScanLibrary(ctx context.Context, library model.Library) error {
	startTime := time.Now()
	source := "local"
	if storage.IsRemoteURI(library.Path) {
		source = "cloud"
	}
	// 快速模式：标题取文件名、时长留空、内嵌封面也不提取——零内容读取（风控友好）。
	skipTags := tagModeForLibrary(library.Path) == "filename"
	log.Info(ctx, "Audiobook scanner: Starting scan", "library", library.Name, "path", library.Path, "source", source)

	fsys, err := audiobookFS(ctx, library)
	if err != nil {
		log.Error(ctx, "Audiobook scanner: Cannot open library storage", "path", library.Path, err)
		return err
	}

	repo := s.ds.Audiobook(ctx)
	var scanned, created, updated int

	// Books seen during this walk. Anything in the library that is not in here is gone
	// from disk and gets purged below — otherwise moving a whole shelf to a cloud
	// library leaves the old rows behind and the media library keeps reporting
	// 专辑数/歌曲数 for content that no longer exists.
	seen := map[string]bool{}
	var walkErrors int
	if _, listErr := fs.ReadDir(fsys, "."); listErr != nil {
		// Unreadable root means unreachable storage (e.g. a cloud gateway that is down),
		// NOT an empty library: purging here would wipe the whole library from the DB.
		log.Warn(ctx, "Audiobook scanner: cannot list library root, skipping vanished-book cleanup",
			"library", library.Name, listErr)
		seen = nil
	}

	// fs.WalkDir walks the storage abstraction (io/fs), so every path below is
	// library-relative and always uses "/" as separator — for local and cloud alike.
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			walkErrors++
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
		if seen != nil {
			seen[relPath] = true
		}

		// Check if already exists
		existing, existErr := repo.GetAll(model.QueryOptions{
			Filters: Eq{"library_id": library.ID, "path": relPath},
		})
		if existErr == nil && len(existing) > 0 {
			book := existing[0]
			if book.Hash != bookHash || book.ChapterCount == 0 {
				book.Hash = bookHash
				// Re-read empty metadata from tags (narrator/author/description/series)
				if book.Narrator == "" || book.Author == "" || book.Description == "" || book.Series == "" {
					tags := readFirstAudioFileTags(fsys, book.Path)
					if book.Narrator == "" && tags.narrator != "" {
						book.Narrator = tags.narrator
					}
					if book.Author == "" && tags.author != "" {
						book.Author = tags.author
					}
					if book.Description == "" && tags.description != "" {
						book.Description = tags.description
					}
					if book.Series == "" && tags.series != "" {
						book.Series = tags.series
					}
				}
				s.scanChapters(ctx, fsys, &book, repo, skipTags)
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
		book := s.createAudiobookFromDir(ctx, fsys, library, relPath, bookHash, skipTags)
		// IMPORTANT: Save the book FIRST before scanning chapters,
		// because audiobook_chapter has a foreign key referencing audiobook(id).
		// If the book doesn't exist in DB yet, chapter inserts will fail.
		if err := repo.Put(&book); err != nil {
			log.Error(ctx, "Audiobook scanner: Error creating", "book", book.Title, err)
			return nil
		}
		s.scanChapters(ctx, fsys, &book, repo, skipTags)
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
		log.Error(ctx, "Audiobook scanner: Walk error", "library", library.Name, err)
		walkErrors++
	}

	// Clean up books whose folder is gone. Judgement is PER BOOK: a book is removed only when
	// its directory is definitively missing (ENOENT). Any other error (permissions, unreachable
	// gateway) keeps the book — a transient outage must never wipe the library. This no longer
	// needs a perfectly clean walk: a single unreadable entry used to skip cleanup entirely (the
	// old "walkErrors == 0" guard), so ghost books could never be removed.
	removed := s.purgeVanishedBooks(ctx, fsys, repo, library, seen)

	log.Info(ctx, "Audiobook scanner: Scan complete", "library", library.Name, "source", source,
		"scanned", scanned, "created", created, "updated", updated, "removed", removed, "walkErrors", walkErrors, "duration", time.Since(startTime))
	return nil
}

// bookDirMissing reports whether the book's folder is definitively gone (ENOENT). Any other
// error (permissions, unreachable gateway) keeps the book: a transient outage must never be
// mistaken for deletion.
func bookDirMissing(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	if err == nil {
		return false
	}
	return errors.Is(err, fs.ErrNotExist)
}

// purgeVanishedBooks deletes books (and everything that references them) that are still in the
// database but whose folder is gone. Without it, moving local audiobook folders to a cloud
// source leaves the media library counting books that are long gone.
func (s *AudiobookScanner) purgeVanishedBooks(ctx context.Context, fsys fs.FS, repo model.AudiobookRepository, library model.Library, seen map[string]bool) int {
	existing, err := repo.GetAll(model.QueryOptions{Filters: Eq{"library_id": library.ID}})
	if err != nil {
		log.Error(ctx, "Audiobook scanner: cannot list books for cleanup", "library", library.Name, err)
		return 0
	}
	removed := 0
	for _, book := range existing {
		if seen != nil && seen[book.Path] {
			continue // definitely present in this walk
		}
		if !bookDirMissing(fsys, book.Path) {
			continue // folder still there, or unreachable -> keep
		}
		if err := repo.DeleteWithRelations(book.ID); err != nil {
			log.Error(ctx, "Audiobook scanner: cannot delete vanished book", "book", book.Title, err)
			continue
		}
		removed++
		log.Info(ctx, "Audiobook scanner: removed vanished book",
			"book", book.Title, "path", book.Path, "library", library.Name)
	}
	return removed
}

func (s *AudiobookScanner) createAudiobookFromDir(ctx context.Context, fsys storage.MusicFS, library model.Library, relPath, bookHash string, skipTags bool) model.Audiobook {
	dirName := path.Base(relPath)
	author, title := parseAudiobookDirName(dirName)
	genre := detectGenreFromPath(relPath)

	// [LeChenMusic-START:audiobook-id3-tags]
	// Try to read metadata from the first audio file's ID3 tags
	tags := audiobookTags{}
	if !skipTags {
		tags = readFirstAudioFileTags(fsys, relPath)
	}
	if tags.title != "" {
		stripped := stripChapterSuffix(tags.title)
		if stripped != "" && !isNumericOnly(stripped) && len([]rune(stripped)) > 1 {
			if title == dirName {
				title = stripped
			}
		}
	}
	if tags.album != "" && (title == "" || title == dirName) {
		// Use ALBUM tag as fallback, but only if it's a meaningful title
		if !isNumericOnly(tags.album) && len([]rune(tags.album)) > 1 {
			title = tags.album
		}
	}
	if tags.genre != "" && genre == "有声读物" {
		genre = tags.genre
	}
	year := tags.year
	// 作者：目录名解析优先，解析不出作者再用显式作者标签（ARTIST 是演播者，不用作作者）
	if author == "" {
		author = tags.author
	}
	// In audiobook files, ARTIST tag typically contains the narrator (演播者), not the author (作者).
	// tags.narrator 已按 COMPOSER > CONDUCTOR > DIRECTOR > TXXX:NARRATOR > ARTIST/ALBUMARTIST解析好。
	narrator := tags.narrator
	// [LeChenMusic-END:audiobook-id3-tags]

	book := model.Audiobook{
		ID:          id.NewRandom(),
		LibraryID:   library.ID,
		Title:       title,
		Author:      author,
		Narrator:    narrator,
		Description: tags.description,
		Genre:       genre,
		Year:        year,
		Series:      tags.series,
		Path:        relPath,
		Hash:        bookHash,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 一次列目录同时回答两件事：书目录里有没有封面文件、哪些音频文件可能内嵌封面。
	// 以前是逐个 fs.Stat 六个候选封面名再列一次目录；网盘源上每个 Stat 都是一次被限速的
	// API 调用（约 2s/次），光找封面文件就能耗掉十几秒。
	coverPath := ""
	entries, listErr := fs.ReadDir(fsys, relPath)
	if listErr == nil {
		if name, ok := audiobookcover.CoverNameIn(entries); ok {
			coverPath = path.Join(relPath, name)
		}
	}
	// [LeChenMusic-START:audiobook-embedded-cover]
	// 书目录没有封面文件时，识别音频文件本身内嵌的封面（ID3v2 APIC/FLAC PICTURE/MP4 covr…）：
	// 提取后按上传/刮削同款规则落盘（本地可写库写进书目录，云库/只读库写本地覆盖目录），
	// 入库即把音频文件的封面图片信息记录进 book.CoverPath。
	// 快速模式（TagMode=filename）同样跳过：提取内嵌封面就是内容读取。
	if coverPath == "" && !skipTags && listErr == nil {
		if data, ext, err := audiobookcover.EmbeddedCoverFromEntries(fsys, relPath, entries); err == nil {
			if relCover, saveErr := audiobookcover.SaveCover(&book, &library, data, ext); saveErr == nil {
				coverPath = relCover
				log.Debug(ctx, "Audiobook scanner: recognized cover embedded in audio files", "book", title, "cover", relCover)
			} else {
				log.Warn(ctx, "Audiobook scanner: cannot save embedded cover", "book", title, saveErr)
			}
		}
	}
	// [LeChenMusic-END:audiobook-embedded-cover]
	book.CoverPath = coverPath
	return book
}

func (s *AudiobookScanner) scanChapters(ctx context.Context, fsys storage.MusicFS, book *model.Audiobook, repo model.AudiobookRepository, skipTags bool) {
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
		// Try to read chapter title and duration from ID3 tags (skipped in filename mode:
		// title falls back to the file name, duration stays 0 — zero content reads).
		var chapterDuration int
		if !skipTags {
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
// audiobookTags 是从首个音频文件标签里读出的书目级字段。
// 注意：有声书里 ARTIST/ALBUMARTIST 通常是演播者（narrator）而不是作者（author），
// 所以作者只认显式作者标签。
type audiobookTags struct {
	artist      string
	title       string
	album       string
	genre       string
	year        int
	narrator    string
	author      string
	description string
	series      string
}

// firstTagValue 返回给定标签键里第一个非空值（taglib 的 key 是大写，TXXX 帧为 "TXXX:<描述>"）。
func firstTagValue(tags map[string][]string, keys ...string) string {
	for _, key := range keys {
		if v, ok := tags[key]; ok && len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	return ""
}

// tagsFromMap 把 taglib 的原始标签映射到书目级字段。纯函数，便于单测标签→字段的映射规则。
func tagsFromMap(tags map[string][]string) audiobookTags {
	t := audiobookTags{
		artist:      firstTagValue(tags, "ARTIST"),
		title:       firstTagValue(tags, "TITLE"),
		album:       firstTagValue(tags, "ALBUM"),
		genre:       firstTagValue(tags, "GENRE"),
		description: firstTagValue(tags, "DESCRIPTION", "COMMENT", "TXXX:DESCRIPTION", "TXXX:COMMENT"),
		series:      firstTagValue(tags, "SERIES", "TXXX:SERIES"),
	}
	if v := firstTagValue(tags, "DATE"); v != "" {
		t.year = parseYear(v)
	}
	if t.artist == "" {
		t.artist = firstTagValue(tags, "ALBUMARTIST")
	}
	// 作者：只认显式作者标签（ARTIST 在有声书里是演播者，绝不能当作者）
	t.author = firstTagValue(tags, "TXXX:AUTHOR", "TXXX:BOOKAUTHOR", "TXXX:BOOK AUTHOR", "AUTHOR", "WRITER", "TXXX:WRITER")
	// 演播者：COMPOSER > CONDUCTOR > DIRECTOR > TXXX:NARRATOR，都没有再退回 ARTIST/ALBUMARTIST
	t.narrator = firstTagValue(tags, "COMPOSER", "CONDUCTOR", "DIRECTOR", "TXXX:NARRATOR")
	if t.narrator == "" {
		t.narrator = t.artist
	}
	return t
}

// parseYear 从 DATE 标签解析年份：兼容 "2014" 和 "2014-05-21" 这类带月日的写法。
func parseYear(v string) int {
	v = strings.TrimSpace(v)
	if y, err := strconv.Atoi(v); err == nil && y > 0 {
		return y
	}
	if len(v) >= 4 {
		if y, err := strconv.Atoi(v[:4]); err == nil && y > 0 {
			return y
		}
	}
	return 0
}

// readFirstAudioFileTags reads ID3/metadata tags from the first audio file in a directory.
// Empty strings/zeros if not found.
func readFirstAudioFileTags(fsys storage.MusicFS, dirPath string) audiobookTags {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return audiobookTags{}
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
		return tagsFromMap(allTags.Tags)
	}
	return audiobookTags{}
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
