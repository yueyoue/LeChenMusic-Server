package scanner

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
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
		s.scanChapters(ctx, &book, library, repo)
		if err := repo.Put(&book); err != nil {
			log.Error(ctx, "Audiobook scanner: Error creating", "book", book.Title, err)
			return nil
		}
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
		Genre:     genre,
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
	for i, af := range audioFiles {
		chapterPath := filepath.Join(audiobookPath, af.name)
		chapterTitle := strings.TrimSuffix(af.name, filepath.Ext(af.name))
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(af.name)), ".")
		var fileSize int64
		if info, err := os.Stat(chapterPath); err == nil {
			fileSize = info.Size()
		}

		chapter := model.AudiobookChapter{
			ID:            id.NewRandom(),
			AudiobookID:   book.ID,
			Title:         chapterTitle,
			ChapterNumber: i + 1,
			Duration:      0,
			Format:        format,
			FileSize:      fileSize,
			Path:          af.path,
			CreatedAt:     time.Now(),
		}
		if err := repo.PutChapter(&chapter); err != nil {
			log.Error(ctx, "Audiobook scanner: Error saving chapter", "chapter", chapter.Title, err)
		}
		totalSize += fileSize
	}

	book.ChapterCount = len(audioFiles)
	book.TotalDuration = 0
	book.Size = totalSize
	if book.Title == "" {
		book.Title = filepath.Base(audiobookPath)
	}
}

func audiobookHash(path string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(path)))
}

func parseAudiobookDirName(name string) (author, title string) {
	if idx := strings.Index(name, " - "); idx > 0 {
		return strings.TrimSpace(name[:idx]), strings.TrimSpace(name[idx+3:])
	}
	if start := strings.LastIndex(name, "("); start > 0 {
		if end := strings.LastIndex(name, ")"); end > start {
			return strings.TrimSpace(name[start+1 : end]), strings.TrimSpace(name[:start])
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
