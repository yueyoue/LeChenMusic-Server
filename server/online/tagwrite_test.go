package online

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/go-flac"
)

// minimalFlac builds a syntactically valid FLAC file: header + STREAMINFO.
func minimalFlac(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte("fLaC"))
	// Metadata block header: last-flag(1) + type(7)=STREAMINFO(0), 24-bit big-endian length 34.
	buf.Write([]byte{0x80, 0x00, 0x00, 0x22})
	streamInfo := make([]byte, 34)
	binary.BigEndian.PutUint16(streamInfo[0:2], 16)   // min block size
	binary.BigEndian.PutUint16(streamInfo[2:4], 4096) // max block size
	buf.Write(streamInfo)
	return buf.Bytes()
}

// minimalMP3 builds a single silent-ish MPEG-1 Layer III frame.
func minimalMP3(t *testing.T) []byte {
	t.Helper()
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xFF, 0xFB, 0x90, 0x64
	return frame
}

// decodeVorbisComment parses the Vorbis Comment block into a map.
func decodeVorbisComment(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r := bytes.NewReader(data)
	var u32 uint32
	must := func(err error) {
		if err != nil {
			t.Fatalf("decode vorbis comment: %v", err)
		}
	}
	must(binary.Read(r, binary.LittleEndian, &u32))
	vendor := make([]byte, u32)
	_, mustErr := io.ReadFull(r, vendor)
	must(mustErr)
	must(binary.Read(r, binary.LittleEndian, &u32))
	res := map[string]string{}
	for i := uint32(0); i < u32; i++ {
		must(binary.Read(r, binary.LittleEndian, &u32))
		entry := make([]byte, u32)
		_, mustErr := io.ReadFull(r, entry)
		must(mustErr)
		k, v, _ := strings.Cut(string(entry), "=")
		res[strings.ToUpper(k)] = v
	}
	return res
}

func TestWriteFLACMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFlac(t), 0o644); err != nil {
		t.Fatal(err)
	}

	cover := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x01, 0x02} // fake jpeg
	lyrics := "[00:01.00] 今天我\n[00:05.00] 寒夜里看雪飘过"
	meta := EmbeddedMeta{
		Title:     "海阔天空",
		Artist:    "BEYOND",
		Album:     "乐与怒",
		Year:      "1993",
		Cover:     cover,
		CoverMIME: "image/jpeg",
		Lyrics:    lyrics,
	}
	if err := WriteEmbeddedMetadata(path, meta); err != nil {
		t.Fatalf("WriteEmbeddedMetadata: %v", err)
	}

	f, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("re-parse flac: %v", err)
	}
	var tags map[string]string
	var pic *flacpicture.MetadataBlockPicture
	for _, b := range f.Meta {
		switch b.Type {
		case flac.VorbisComment:
			tags = decodeVorbisComment(t, b.Data)
		case flac.Picture:
			p, err := flacpicture.ParseFromMetaDataBlock(*b)
			if err != nil {
				t.Fatalf("parse picture block: %v", err)
			}
			pic = p
		}
	}
	if tags == nil {
		t.Fatal("no vorbis comment block found")
	}
	if tags["TITLE"] != "海阔天空" || tags["ARTIST"] != "BEYOND" || tags["ALBUM"] != "乐与怒" || tags["DATE"] != "1993" {
		t.Errorf("unexpected tags: %v", tags)
	}
	if tags["LYRICS"] != lyrics {
		t.Errorf("lyrics not embedded correctly: %q", tags["LYRICS"])
	}
	if pic == nil {
		t.Fatal("no picture block found")
	}
	if !bytes.Equal(pic.ImageData, cover) {
		t.Errorf("cover bytes mismatch")
	}

	// Rewriting must replace, not duplicate.
	if err := WriteEmbeddedMetadata(path, EmbeddedMeta{Title: "新标题", Artist: "新歌手"}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	f, err = flac.ParseFile(path)
	if err != nil {
		t.Fatalf("re-parse flac: %v", err)
	}
	vorbisBlocks, picBlocks := 0, 0
	for _, b := range f.Meta {
		switch b.Type {
		case flac.VorbisComment:
			vorbisBlocks++
			tags = decodeVorbisComment(t, b.Data)
		case flac.Picture:
			picBlocks++
		}
	}
	if vorbisBlocks != 1 || picBlocks != 0 {
		t.Errorf("expected 1 vorbis block and 0 picture blocks after rewrite, got %d/%d", vorbisBlocks, picBlocks)
	}
	if tags["TITLE"] != "新标题" || tags["LYRICS"] != "" {
		t.Errorf("unexpected tags after rewrite: %v", tags)
	}
}

func TestWriteMP3Metadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, minimalMP3(t), 0o644); err != nil {
		t.Fatal(err)
	}

	cover := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x01, 0x02}
	meta := EmbeddedMeta{
		Title:     "海阔天空",
		Artist:    "BEYOND",
		Album:     "乐与怒",
		Cover:     cover,
		CoverMIME: "image/jpeg",
		Lyrics:    "[00:01.00] 今天我",
	}
	if err := WriteEmbeddedMetadata(path, meta); err != nil {
		t.Fatalf("WriteEmbeddedMetadata: %v", err)
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("re-open mp3: %v", err)
	}
	defer tag.Close()

	if tag.Title() != "海阔天空" || tag.Artist() != "BEYOND" {
		t.Errorf("unexpected title/artist: %q/%q", tag.Title(), tag.Artist())
	}
	lyricFrames := tag.GetFrames(tag.CommonID("Unsynchronised lyrics/text transcription"))
	if len(lyricFrames) != 1 {
		t.Fatalf("expected 1 USLT frame, got %d", len(lyricFrames))
	}
	uslt, ok := lyricFrames[0].(id3v2.UnsynchronisedLyricsFrame)
	if !ok || uslt.Lyrics != meta.Lyrics {
		t.Errorf("unexpected lyrics frame: %+v", lyricFrames[0])
	}
	picFrames := tag.GetFrames(tag.CommonID("Attached picture"))
	if len(picFrames) != 1 {
		t.Fatalf("expected 1 APIC frame, got %d", len(picFrames))
	}
	pf, ok := picFrames[0].(id3v2.PictureFrame)
	if !ok || !bytes.Equal(pf.Picture, cover) {
		t.Errorf("unexpected picture frame")
	}
}

func TestWriteMetadataUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(path, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteEmbeddedMetadata(path, EmbeddedMeta{Title: "x"}); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}
