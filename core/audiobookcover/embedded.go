// Package audiobookcover: extraction of cover art embedded in audiobook audio files.
//
// [LeChenMusic-START:audiobook]
package audiobookcover

import (
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

// EmbeddedCover extracts the cover art embedded in a book's audio files (ID3v2 APIC,
// FLAC PICTURE, MP4 covr, …). It probes the folder's first audio files — the same
// files the scanner reads the book's tags from — and prefers the front-cover picture.
// Returns the image bytes and the extension the image should be stored under.
//
// The FS is the library storage abstraction, so local folders and cloud sources
// (openlist://…) behave the same: a cloud source only pulls the byte ranges taglib
// actually needs (lazy Range reads, never the whole file).
func EmbeddedCover(fsys fs.FS, dirPath string) ([]byte, string, error) {
	files, err := audioFiles(fsys, dirPath)
	if err != nil {
		return nil, "", err
	}
	if len(files) > maxCoverProbe {
		files = files[:maxCoverProbe]
	}
	lastErr := fmt.Errorf("audiobook cover: no embedded image in %s", dirPath)
	for _, filePath := range files {
		data, ext, err := embeddedCoverFromFile(fsys, filePath)
		if err == nil {
			return data, ext, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
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
		return nil, "", fmt.Errorf("audiobook cover: no embedded image in %s", filePath)
	}
	data, err := tf.Image(bestImageIndex(images))
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf("audiobook cover: could not load embedded image from %s", filePath)
	}
	return data, imageExt(data), nil
}

// audioFiles returns the library-relative paths of the audio files in dirPath,
// in name order — the same order chapters are numbered in.
func audioFiles(fsys fs.FS, dirPath string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return nil, fmt.Errorf("audiobook cover: cannot read dir %s: %w", dirPath, err)
	}
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
		return nil, fmt.Errorf("audiobook cover: no audio files in %s", dirPath)
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
