package nativeapi

import (
	"github.com/navidrome/navidrome/core/audiobookcover"
	"github.com/navidrome/navidrome/model"
)

// Cover storage for audiobooks (评审 §4 P1-2).
//
// The storage logic lives in core/audiobookcover so the audiobook scanner can save
// covers it recognizes during ingestion exactly the same way (audio-embedded covers
// included). These wrappers keep the historical names for the native API call sites.
//
// Local libraries keep the historical behavior: the cover is written next to the audio
// files, where other tools (and the scanner) expect it. Cloud libraries (openlist://…)
// have no writable local path, so uploaded/scraped covers go to a local override
// directory instead of failing with "Failed to save cover". cover() serves the override
// first, so the API keeps working for cloud books.

// saveAudiobookCover stores cover bytes for a book and returns the value to persist in
// book.CoverPath.
func saveAudiobookCover(book *model.Audiobook, lib *model.Library, imageData []byte, ext string) (string, error) {
	return audiobookcover.SaveCover(book, lib, imageData, ext)
}

// audiobookCoverOverride returns the local override cover file for a book, if any.
func audiobookCoverOverride(bookID string) string {
	return audiobookcover.OverrideCover(bookID)
}
