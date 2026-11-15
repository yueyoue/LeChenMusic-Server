package online

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/go-flac"
)

// EmbeddedInfo is the metadata currently embedded in an audio file,
// read back for scrape verification (M5 admin download management).
type EmbeddedInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"albumArtist,omitempty"`
	Album       string `json:"album"`
	Year        string `json:"year"`
	TrackNumber int    `json:"trackNumber"`
	HasCover    bool   `json:"hasCover"`
	CoverMIME   string `json:"coverMime,omitempty"`
	Cover       []byte `json:"-"`
	Lyrics      string `json:"lyrics"`
}

// HasTags reports whether basic identifying tags are present.
func (info EmbeddedInfo) HasTags() bool {
	return info.Title != "" || info.Artist != ""
}

// ReadEmbeddedMetadata reads the embedded tags (info/cover/lyrics) back from an
// audio file. Only MP3 and FLAC are supported, matching WriteEmbeddedMetadata.
func ReadEmbeddedMetadata(path string) (*EmbeddedInfo, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return readMP3Metadata(path)
	case ".flac":
		return readFLACMetadata(path)
	default:
		return nil, fmt.Errorf("online: unsupported format for embedded tags: %s", filepath.Ext(path))
	}
}

// readMP3Metadata reads ID3v2 tags: text frames + APIC cover + USLT lyrics.
func readMP3Metadata(path string) (*EmbeddedInfo, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, fmt.Errorf("online: open mp3: %w", err)
	}
	defer tag.Close()

	info := &EmbeddedInfo{
		Title:  tag.Title(),
		Artist: tag.Artist(),
	}
	if f := firstTextFrame(tag, tag.CommonID("Album/Movie/Show title")); f != "" {
		info.Album = f
	}
	if f := firstTextFrame(tag, tag.CommonID("Band/Orchestra/Accompaniment")); f != "" {
		info.AlbumArtist = f
	}
	// Year may live in the version-specific frame (TYER in v2.3, TDRC in v2.4)
	// or in a release/original time frame written by another tagger.
	info.Year = firstTextFrame(tag, tag.CommonID("Year"))
	if info.Year == "" {
		info.Year = firstAnyTextFrame(tag, "TDRL", "TYER", "TDRC", "TDOR")
	}
	if f := firstTextFrame(tag, tag.CommonID("Track number/Position in set")); f != "" {
		var n int
		if _, err := fmt.Sscanf(f, "%d", &n); err == nil {
			info.TrackNumber = n
		}
	}
	for _, fr := range tag.GetFrames(tag.CommonID("Attached picture")) {
		if pf, ok := fr.(id3v2.PictureFrame); ok {
			info.HasCover = true
			info.CoverMIME = pf.MimeType
			info.Cover = pf.Picture
			break
		}
	}
	for _, fr := range tag.GetFrames(tag.CommonID("Unsynchronised lyrics/text transcription")) {
		if uslt, ok := fr.(id3v2.UnsynchronisedLyricsFrame); ok {
			info.Lyrics = uslt.Lyrics
			break
		}
	}
	return info, nil
}

func firstTextFrame(tag *id3v2.Tag, id string) string {
	if id == "" {
		return ""
	}
	for _, fr := range tag.GetFrames(id) {
		if tf, ok := fr.(id3v2.TextFrame); ok {
			return strings.TrimSpace(tf.Text)
		}
	}
	return ""
}

// firstAnyTextFrame returns the first non-empty text frame among the given ids.
// Taggers disagree on where the year lives (v2.4 TDRC, v2.3 TYER, MusicBrainz
// TDRL/TDOR), so read-back checks them all.
func firstAnyTextFrame(tag *id3v2.Tag, ids ...string) string {
	for _, id := range ids {
		if v := firstTextFrame(tag, id); v != "" {
			return v
		}
	}
	return ""
}

// readFLACMetadata reads Vorbis Comment tags + PICTURE cover + LYRICS.
func readFLACMetadata(path string) (*EmbeddedInfo, error) {
	f, err := flac.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("online: parse flac: %w", err)
	}
	info := &EmbeddedInfo{}
	for _, b := range f.Meta {
		switch b.Type {
		case flac.VorbisComment:
			tags := decodeVorbisComments(b.Data)
			info.Title = tags["TITLE"]
			info.Artist = tags["ARTIST"]
			info.AlbumArtist = firstNonEmpty(tags["ALBUMARTIST"], tags["ALBUM ARTIST"], tags["ALBUM_ARTIST"])
			info.Album = tags["ALBUM"]
			info.Year = firstNonEmpty(tags["DATE"], tags["YEAR"], tags["ORIGINALDATE"])
			if t := tags["TRACKNUMBER"]; t != "" {
				var n int
				if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
					info.TrackNumber = n
				}
			}
			// LYRICS is our field; fall back to common alternatives found in the wild.
			info.Lyrics = tags["LYRICS"]
			if info.Lyrics == "" {
				info.Lyrics = tags["UNSYNCEDLYRICS"]
			}
		case flac.Picture:
			pic, err := flacpicture.ParseFromMetaDataBlock(*b)
			if err != nil {
				continue // tolerate undecodable picture blocks
			}
			info.HasCover = true
			info.CoverMIME = pic.MIME
			info.Cover = pic.ImageData
		}
	}
	return info, nil
}

// decodeVorbisComments parses the Vorbis Comment block into a map (keys upper-cased).
func decodeVorbisComments(data []byte) map[string]string {
	r := bytes.NewReader(data)
	var u32 uint32
	if err := binary.Read(r, binary.LittleEndian, &u32); err != nil {
		return nil
	}
	if _, err := r.Seek(int64(u32), io.SeekCurrent); err != nil { // skip vendor string
		return nil
	}
	if err := binary.Read(r, binary.LittleEndian, &u32); err != nil {
		return nil
	}
	res := make(map[string]string, u32)
	for i := uint32(0); i < u32; i++ {
		var l uint32
		if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
			break
		}
		entry := make([]byte, l)
		if _, err := io.ReadFull(r, entry); err != nil {
			break
		}
		k, v, _ := strings.Cut(string(entry), "=")
		res[strings.ToUpper(k)] = v
	}
	return res
}
