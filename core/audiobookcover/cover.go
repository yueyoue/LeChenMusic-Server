// Package audiobookcover centralizes audiobook cover handling shared by the scanner
// (recognition of covers at ingestion) and the native API (upload / scrape / serve):
//
//   - CoverNames / AudioExts: the book-folder conventions (cover.jpg… and chapter extensions)
//   - SaveCover: stores cover bytes for a book and returns the value for book.CoverPath
//   - OverrideCover: resolves the local override cover of a book
//   - EmbeddedCover: extracts cover art embedded in a book's audio files (APIC/PICTURE/covr…)
//
// [LeChenMusic-START:audiobook]
package audiobookcover

import (
	"os"
	"path/filepath"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
)

// AudioExts are the file extensions that make a file an audiobook chapter.
var AudioExts = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".flac": true,
	".ogg": true, ".wav": true, ".opus": true, ".wma": true, ".aac": true,
}

// CoverNames are the external cover file names probed in a book folder, in priority order.
var CoverNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png",
	"folder.jpg", "folder.jpeg", "folder.png",
}

// OverrideNames are the file names probed in a book's local override cover directory.
var OverrideNames = []string{
	"cover.jpg", "cover.jpeg", "cover.png", "cover.webp",
}

// OverrideDir is the local override directory for covers of a single book.
func OverrideDir(bookID string) string {
	return filepath.Join(conf.Server.DataFolder.String(), "audiobook-covers", bookID)
}

// SaveCover stores cover bytes for a book and returns the value to persist in
// book.CoverPath. Local libraries keep writing into the library folder (unchanged
// behavior); anything else — cloud libraries, or an unwritable library folder — falls
// back to the local override directory.
func SaveCover(book *model.Audiobook, lib *model.Library, imageData []byte, ext string) (string, error) {
	if !storage.IsRemoteURI(lib.Path) {
		bookPath := filepath.Join(lib.Path, book.Path)
		for _, name := range CoverNames {
			_ = os.Remove(filepath.Join(bookPath, name))
		}
		coverPath := filepath.Join(bookPath, "cover"+ext)
		if err := os.WriteFile(coverPath, imageData, 0o644); err == nil {
			relCover, _ := filepath.Rel(lib.Path, coverPath)
			return relCover, nil
		}
		// Library folder not writable: fall through to the override directory.
	}

	dir := OverrideDir(book.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, name := range OverrideNames {
		_ = os.Remove(filepath.Join(dir, name))
	}
	coverPath := filepath.Join(dir, "cover"+ext)
	if err := os.WriteFile(coverPath, imageData, 0o644); err != nil {
		return "", err
	}
	// Stored as a library-relative-looking marker so consumers that join it with the
	// library path simply find nothing and skip it; the cover endpoint resolves the
	// override dir by convention, independent of this value.
	return filepath.Join("audiobook-covers", book.ID, "cover"+ext), nil
}

// OverrideCover returns the local override cover file for a book, if any.
func OverrideCover(bookID string) string {
	dir := OverrideDir(bookID)
	for _, name := range OverrideNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// [LeChenMusic-END:audiobook]
