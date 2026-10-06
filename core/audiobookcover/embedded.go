// Package audiobookcover: extraction of cover art embedded in audiobook audio files.
//
// [LeChenMusic-START:audiobook]
package audiobookcover

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	taglib "go.senan.xyz/taglib"
)

// ErrNoEmbeddedCover marks a confident "this book's audio carries no embedded cover art" result,
// as opposed to a transient read / network failure. Callers use it to cache the negative result
// instead of re-probing the audio files on every request (a cloud probe can take seconds).
var ErrNoEmbeddedCover = errors.New("audiobook cover: no embedded image")

// ErrNoAudio marks "the folder has no audio files", so no embedded art is possible at all.
var ErrNoAudio = errors.New("audiobook cover: no audio files")

// errNoImageInFile: the file was read fine but carries no picture (definitive for that file).
var errNoImageInFile = errors.New("no embedded image in file")

// EmbeddedCover extracts the cover art embedded in a book's audio files (ID3v2 APIC,
// FLAC PICTURE, MP4 covr, …). It probes the folder's first audio files — the same
// files the scanner reads the book's tags from — and prefers the front-cover picture.
// Returns the image bytes and the extension the image should be stored under.
//
// The FS is the library storage abstraction, so local folders and cloud sources
// (openlist://…) behave the same: a cloud source only pulls the byte ranges taglib
// actually needs (lazy Range reads, never the whole file).
func EmbeddedCover(fsys fs.FS, dirPath string) ([]byte, string, error) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return nil, "", fmt.Errorf("audiobook cover: cannot read dir %s: %w", dirPath, err)
	}
	return EmbeddedCoverFromEntries(fsys, dirPath, entries)
}

// EmbeddedCoverFromEntries is EmbeddedCover with the book folder already listed. Callers that just
// did a ReadDir of the same folder (to look for a cover file, say) hand the entries over instead of
// paying for a second listing.
//
// This matters a lot on a cloud source: the gateway client paces its API calls (roughly one per two
// seconds), so every listing is real wall-clock time. A cover resolve used to cost a listing plus a
// handful of Stats plus the audio probe — comfortably more than the resolve timeout, which meant the
// answer never arrived, nothing got cached, and the same slow walk was repeated for every request.
func EmbeddedCoverFromEntries(fsys fs.FS, dirPath string, entries []fs.DirEntry) ([]byte, string, error) {
	files, err := audioFilesFromEntries(dirPath, entries)
	if err != nil {
		if errors.Is(err, ErrNoAudio) {
			// No audio files at all → no embedded art is possible. Confident negative.
			return nil, "", fmt.Errorf("%w in %s", ErrNoEmbeddedCover, dirPath)
		}
		return nil, "", err
	}
	if len(files) > maxCoverProbe {
		files = files[:maxCoverProbe]
	}
	var transient error
	for _, filePath := range files {
		data, ext, err := embeddedCoverFromFile(fsys, filePath)
		if err == nil {
			return data, ext, nil
		}
		if !errors.Is(err, errNoImageInFile) {
			// Could not even read this file's tags (network / partial read): remember it so the
			// caller knows this is NOT a confident "no cover" and should not cache the negative.
			transient = err
		}
	}
	if transient != nil {
		return nil, "", transient
	}
	return nil, "", fmt.Errorf("%w in %s", ErrNoEmbeddedCover, dirPath)
}

// maxCoverProbe bounds how many audio files are probed for embedded art. Covers are
// almost always embedded in every chapter of a book or in none of them, so a small
// probe window is enough and keeps the scan cost bounded for huge books without any.
const maxCoverProbe = 3

func embeddedCoverFromFile(fsys fs.FS, filePath string) ([]byte, string, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return nil, "", fmt.Errorf("audiobook cover: %s is not seekable", filePath)
	}
	tf, err := taglib.OpenStream(rs,
		taglib.WithReadStyle(taglib.ReadStyleFast),
		taglib.WithFilename(filePath),
	)
	if err != nil {
		return nil, "", fmt.Errorf("audiobook cover: cannot read tags from %s: %w", filePath, err)
	}
	// Close in LIFO order: tf first (it holds rs internally), then f.
	defer tf.Close()

	images := tf.Properties().Images
	if len(images) == 0 {
		return nil, "", fmt.Errorf("%s: %w", filePath, errNoImageInFile)
	}
	data, err := tf.Image(bestImageIndex(images))
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf("audiobook cover: could not load embedded image from %s", filePath)
	}
	return data, imageExt(data), nil
}

// CoverNameIn returns the cover file present in an already-listed book folder, honouring the
// CoverNames priority order (cover.jpg wins over folder.png). The listing is reused so probing for a
// cover file costs zero extra round trips against a cloud gateway.
func CoverNameIn(entries []fs.DirEntry) (string, bool) {
	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			present[e.Name()] = true
		}
	}
	for _, name := range CoverNames {
		if present[name] {
			return name, true
		}
	}
	return "", false
}

// audioFilesFromEntries filters an already-read directory listing down to the chapter files.
func audioFilesFromEntries(dirPath string, entries []fs.DirEntry) ([]string, error) {
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if AudioExts[strings.ToLower(path.Ext(e.Name()))] {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: %w", dirPath, ErrNoAudio)
	}
	sort.Strings(names)
	for i := range names {
		names[i] = path.Join(dirPath, names[i])
	}
	return names, nil
}

// picTypeRegexes match the picture type embedded in the file, in the order they are
// listed. Same preference as the music artwork pipeline (core/artwork/sources.go).
var picTypeRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i).*cover.*front.*|.*front.*cover.*`),
	regexp.MustCompile(`(?i).*front.*`),
	regexp.MustCompile(`(?i).*cover.*`),
}

func bestImageIndex(images []taglib.ImageDesc) int {
	for _, regex := range picTypeRegexes {
		for i, img := range images {
			if regex.MatchString(img.Type) {
				return i
			}
		}
	}
	return 0
}

// imageExt maps image bytes to the extension used for the saved cover file,
// mirroring the scrape/downloadCover content-type handling.
func imageExt(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// [LeChenMusic-END:audiobook]
