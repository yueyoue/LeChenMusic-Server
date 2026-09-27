package nativeapi

import (
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
)

// Cover storage for audiobooks (评审 §4 P1-2).
//
// Local libraries keep the historical behavior: the cover is written next to the audio
// files, where other tools (and the scanner) expect it. Cloud libraries (openlist://…)
// have no writable local path, so uploaded/scraped covers go to a local override
// directory instead of failing with "Failed to save cover". cover() serves the override
// first, so the API keeps working for cloud books.

// audiobookCoverOverrideDir is the local override directory for covers of a single book.
func audiobookCoverOverrideDir(bookID string) string {
	return filepath.Join(conf.Server.DataFolder.String(), "audiobook-covers", bookID)
}

// audiobookCoverOverrideNames are the file names probed in the override directory.
var audiobookCoverOverrideNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png", "cover.webp",
}

// saveAudiobookCover stores cover bytes for a book and returns the value to persist in
// book.CoverPath. Local libraries keep writing into the library folder (unchanged
// behavior); anything else — cloud libraries, or an unwritable library folder — falls
// back to the local override directory.
func saveAudiobookCover(book *model.Audiobook, lib *model.Library, imageData []byte, ext string) (string, error) {
	if !storage.IsRemoteURI(lib.Path) {
		bookPath := filepath.Join(lib.Path, book.Path)
		for _, name := range audiobookCoverNames {
			_ = os.Remove(filepath.Join(bookPath, name))
		}
		coverPath := filepath.Join(bookPath, "cover"+ext)
		if err := os.WriteFile(coverPath, imageData, 0o644); err == nil {
			relCover, _ := filepath.Rel(lib.Path, coverPath)
			return relCover, nil
		}
		// Library folder not writable: fall through to the override directory.
	}

	dir := audiobookCoverOverrideDir(book.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, name := range audiobookCoverOverrideNames {
		_ = os.Remove(filepath.Join(dir, name))
	}
	coverPath := filepath.Join(dir, "cover"+ext)
	if err := os.WriteFile(coverPath, imageData, 0o644); err != nil {
		return "", err
	}
	// Stored as a library-relative-looking marker so consumers that join it with the
	// library path simply find nothing and skip it; cover() resolves the override dir
	// by convention, independent of this value.
	return filepath.Join("audiobook-covers", book.ID, "cover"+ext), nil
}

// audiobookCoverOverride returns the local override cover file for a book, if any.
func audiobookCoverOverride(bookID string) string {
	dir := audiobookCoverOverrideDir(bookID)
	for _, name := range audiobookCoverOverrideNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
