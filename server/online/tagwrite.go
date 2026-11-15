package online

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/go-flac"
	"github.com/navidrome/navidrome/log"
)

// EmbeddedMeta is the metadata written INTO the audio file itself.
// No sidecar files (cover.jpg / .lrc) are ever generated.
type EmbeddedMeta struct {
	Title       string
	Artist      string
	AlbumArtist string // album artist: keeps multi-artist albums in one album
	Album       string
	Year        string // year or full release date (YYYY-MM-DD)
	TrackNumber int
	Cover       []byte
	CoverMIME   string
	Lyrics      string
}

// WriteEmbeddedMetadata writes song info, cover art and lyrics directly into the
// audio file at path (MP3: ID3v2.4, FLAC: Vorbis Comment + PICTURE block).
func WriteEmbeddedMetadata(path string, meta EmbeddedMeta) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return writeMP3Metadata(path, meta)
	case ".flac":
		return writeFLACMetadata(path, meta)
	default:
		return fmt.Errorf("online: unsupported format for embedded tags: %s", filepath.Ext(path))
	}
}

// writeMP3Metadata writes ID3v2.4 tags: text frames + APIC cover + USLT lyrics.
func writeMP3Metadata(path string, meta EmbeddedMeta) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("online: open mp3: %w", err)
	}
	defer tag.Close()

	// Drop whatever the upstream file carried so our tags are authoritative.
	tag.DeleteAllFrames()

	tag.AddTextFrame(tag.CommonID("Title"), id3v2.EncodingUTF8, meta.Title)
	tag.AddTextFrame(tag.CommonID("Artist"), id3v2.EncodingUTF8, meta.Artist)
	// TPE2 = album artist; TPE1 above stays the track artist.
	if meta.AlbumArtist != "" {
		tag.AddTextFrame(tag.CommonID("Band/Orchestra/Accompaniment"), id3v2.EncodingUTF8, meta.AlbumArtist)
	}
	if meta.Album != "" {
		tag.AddTextFrame(tag.CommonID("Album/Movie/Show title"), id3v2.EncodingUTF8, meta.Album)
	}
	if meta.Year != "" {
		tag.AddTextFrame(tag.CommonID("Year"), id3v2.EncodingUTF8, meta.Year)
	}
	if meta.TrackNumber > 0 {
		tag.AddTextFrame(tag.CommonID("Track number/Position in set"), id3v2.EncodingUTF8, fmt.Sprintf("%d", meta.TrackNumber))
	}
	if len(meta.Cover) > 0 {
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    meta.CoverMIME,
			PictureType: id3v2.PTFrontCover,
			Description: "Cover",
			Picture:     meta.Cover,
		})
	}
	if meta.Lyrics != "" {
		tag.AddUnsynchronisedLyricsFrame(id3v2.UnsynchronisedLyricsFrame{
			Encoding:          id3v2.EncodingUTF8,
			Language:          "chi",
			ContentDescriptor: "",
			Lyrics:            meta.Lyrics,
		})
	}
	return tag.Save()
}

// writeFLACMetadata writes Vorbis Comment tags + PICTURE cover + LYRICS into the FLAC file.
func writeFLACMetadata(path string, meta EmbeddedMeta) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("online: parse flac: %w", err)
	}

	// Rebuild metadata: keep everything except old tag/picture blocks.
	blocks := make([]*flac.MetaDataBlock, 0, len(f.Meta)+2)
	for _, b := range f.Meta {
		if b.Type == flac.VorbisComment || b.Type == flac.Picture {
			continue
		}
		blocks = append(blocks, b)
	}

	tags := [][2]string{
		{"TITLE", meta.Title},
		{"ARTIST", meta.Artist},
	}
	if meta.AlbumArtist != "" {
		tags = append(tags, [2]string{"ALBUMARTIST", meta.AlbumArtist})
	}
	if meta.Album != "" {
		tags = append(tags, [2]string{"ALBUM", meta.Album})
	}
	if meta.Year != "" {
		tags = append(tags, [2]string{"DATE", meta.Year})
	}
	if meta.TrackNumber > 0 {
		tags = append(tags, [2]string{"TRACKNUMBER", fmt.Sprintf("%d", meta.TrackNumber)})
	}
	if meta.Lyrics != "" {
		tags = append(tags, [2]string{"LYRICS", meta.Lyrics})
	}
	blocks = append(blocks, &flac.MetaDataBlock{Type: flac.VorbisComment, Data: encodeVorbisComment(tags)})

	if len(meta.Cover) > 0 {
		pic, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Cover", meta.Cover, meta.CoverMIME)
		if err != nil {
			// FLAC picture blocks require a decodable jpeg/png. Skip the cover
			// instead of failing the whole ingest.
			log.Warn(context.Background(), "online: skipping undecodable FLAC cover", "mime", meta.CoverMIME, err)
		} else {
			picBlock := pic.Marshal()
			blocks = append(blocks, &picBlock)
		}
	}

	f.Meta = blocks
	if err := f.Save(path); err != nil {
		return fmt.Errorf("online: save flac: %w", err)
	}
	return nil
}

// encodeVorbisComment serializes Vorbis Comment pairs per the spec:
// vendor_length + vendor + comment_count + (length + "K=V")*.
func encodeVorbisComment(tags [][2]string) []byte {
	var buf bytes.Buffer
	vendor := []byte("lechenmusic")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(vendor)))
	buf.Write(vendor)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(tags)))
	for _, kv := range tags {
		entry := []byte(kv[0] + "=" + kv[1])
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(entry)))
		buf.Write(entry)
	}
	return buf.Bytes()
}
