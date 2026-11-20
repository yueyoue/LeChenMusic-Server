package online

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// AudioDuration probes the playback duration of a local audio file without
// decoding it. It supports the containers the download pipeline produces
// (FLAC, MP3); anything else returns an error and callers skip the check.
//
// It exists to catch upstream ad/trial placeholder audio (a ~12.8s clip served
// as any song): the probed duration simply does not match the real track.
func AudioDuration(path string) (d time.Duration, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var head [8]byte
	n, err := f.ReadAt(head[:], 0)
	if err != nil || n < 4 {
		return 0, fmt.Errorf("audio duration: unreadable header")
	}
	switch {
	case string(head[:4]) == "fLaC":
		return flacDuration(f)
	case string(head[:3]) == "ID3" || (head[0] == 0xFF && head[1]&0xE0 == 0xE0):
		return mp3Duration(f)
	default:
		return 0, errors.New("audio duration: unsupported container")
	}
}

// ---- FLAC ----

// flacDuration reads the STREAMINFO block: the total sample count divided by
// the sample rate is the exact stream duration.
func flacDuration(f *os.File) (time.Duration, error) {
	var header [8]byte
	if _, err := f.ReadAt(header[:], 0); err != nil {
		return 0, fmt.Errorf("flac duration: %w", err)
	}
	if string(header[:4]) != "fLaC" {
		return 0, errors.New("flac duration: bad magic")
	}
	// Walk metadata blocks until STREAMINFO (type 0). It must be first per spec,
	// but be lenient. Block header: 1 bit last-flag + 7 bit type + 24 bit length.
	offset := int64(4)
	buf := make([]byte, 4)
	for i := 0; i < 8; i++ {
		if _, err := f.ReadAt(buf, offset); err != nil {
			return 0, fmt.Errorf("flac duration: %w", err)
		}
		blockType := buf[0] & 0x7F
		length := int64(buf[1])<<16 | int64(buf[2])<<8 | int64(buf[3])
		if blockType == 0 {
			if length < 34 {
				return 0, errors.New("flac duration: short STREAMINFO")
			}
			info := make([]byte, 34)
			if _, err := f.ReadAt(info, offset+4); err != nil {
				return 0, fmt.Errorf("flac duration: %w", err)
			}
			return flacStreamInfoDuration(info)
		}
		if buf[0]&0x80 != 0 {
			break // last metadata block and it wasn't STREAMINFO
		}
		offset += 4 + length
	}
	return 0, errors.New("flac duration: STREAMINFO not found")
}

// flacStreamInfoDuration decodes the 34-byte STREAMINFO payload.
//
// Bit layout (FLAC format spec, after 8 bytes of block sizes):
//
//	info[0..1]   min block size
//	info[2..3]   max block size
//	info[4..6]   min frame size
//	info[7..9]   max frame size
//	info[10..12] 20 bit sample rate | 3 bit channels-1 | 1 bit bps-1 high bit
//	info[13]     4 bit bps-1 low bits | 4 bit total-samples high bits
//	info[14..17] remaining 32 bits of total samples
//	info[18..33] MD5
//
// The 36-bit total-samples field straddles info[13] (high nibble) and
// info[14..17]; reading its high nibble from info[12] instead shifts the value
// by 32 bits and turns a 4-minute track into a ~79-hour one (the 286089s
// false "ad placeholder" rejection seen in the field).
func flacStreamInfoDuration(info []byte) (time.Duration, error) {
	sampleRate := uint32(info[10])<<12 | uint32(info[11])<<4 | uint32(info[12])>>4
	total := uint64(info[13]&0x0F)<<32 | uint64(info[14])<<24 |
		uint64(info[15])<<16 | uint64(info[16])<<8 | uint64(info[17])
	if sampleRate == 0 || total == 0 {
		return 0, errors.New("flac duration: unknown sample rate/length")
	}
	return time.Duration(float64(total) / float64(sampleRate) * float64(time.Second)), nil
}

// ---- MP3 ----

// mp3Header describes one parsed MPEG audio frame header.
type mp3Header struct {
	version    int // 1, 2 or 25 (2.5)
	layer      int // 1, 2 or 3
	bitrate    int // bit/s
	sampleRate int // Hz
	padding    int
	frameLen   int // bytes
	samples    int // samples per frame
}

// mp3 bit-rate tables (bit/s), index 0 = free format (unsupported), 15 = invalid.
var (
	mp3BitrateV1L1  = [16]int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0}
	mp3BitrateV1L2  = [16]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0}
	mp3BitrateV1L3  = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	mp3BitrateV2L1  = [16]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0}
	mp3BitrateV2L23 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}

	mp3Rates = map[int][3]int{
		1:  {44100, 48000, 32000},
		2:  {22050, 24000, 16000},
		25: {11025, 12000, 8000},
	}
)

// parseMP3Header parses the 4 bytes at b into a frame header.
func parseMP3Header(b []byte) (mp3Header, bool) {
	var h mp3Header
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return h, false
	}
	versionBits := (b[1] >> 3) & 0x03
	layerBits := (b[1] >> 1) & 0x03
	switch versionBits {
	case 0:
		h.version = 25
	case 2:
		h.version = 2
	case 3:
		h.version = 1
	default:
		return h, false // reserved
	}
	switch layerBits {
	case 1:
		h.layer = 3
	case 2:
		h.layer = 2
	case 3:
		h.layer = 1
	default:
		return h, false // reserved
	}

	bitrateIdx := int(b[2] >> 4)
	rateIdx := int((b[2] >> 2) & 0x03)
	h.padding = int((b[2] >> 1) & 0x01)
	if bitrateIdx == 0 || bitrateIdx == 15 || rateIdx == 3 {
		return h, false // free format / invalid
	}

	var bitrate int
	switch {
	case h.version == 1 && h.layer == 1:
		bitrate = mp3BitrateV1L1[bitrateIdx] * 1000
	case h.version == 1 && h.layer == 2:
		bitrate = mp3BitrateV1L2[bitrateIdx] * 1000
	case h.version == 1 && h.layer == 3:
		bitrate = mp3BitrateV1L3[bitrateIdx] * 1000
	case h.layer == 1:
		bitrate = mp3BitrateV2L1[bitrateIdx] * 1000
	default:
		bitrate = mp3BitrateV2L23[bitrateIdx] * 1000
	}
	rates := mp3Rates[h.version]
	h.bitrate = bitrate
	h.sampleRate = rates[rateIdx]

	switch {
	case h.layer == 1:
		h.samples = 384
		h.frameLen = (12*bitrate/h.sampleRate + h.padding) * 4
	case h.version == 1:
		h.samples = 1152
		h.frameLen = 144*bitrate/h.sampleRate + h.padding
	default: // MPEG-2/2.5 Layer II & III
		h.samples = 1152
		if h.layer == 3 {
			h.samples = 576
		}
		h.frameLen = 72*bitrate/h.sampleRate + h.padding
	}
	if h.frameLen < 4 {
		return h, false
	}
	return h, true
}

// mp3Duration walks the frame chain and sums up the samples per frame. The
// file size cap bounds pathological inputs; 200k frames ≈ 87 min at 44.1kHz.
const mp3MaxFrames = 200000

func mp3Duration(f *os.File) (time.Duration, error) {
	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := stat.Size()

	// Skip an ID3v2 tag if present (syncsafe size, plus 10 byte header).
	var start int64
	var id3 [10]byte
	if n, _ := f.ReadAt(id3[:], 0); n == 10 && string(id3[:3]) == "ID3" {
		tagSize := int64(id3[6]&0x7F)<<21 | int64(id3[7]&0x7F)<<14 | int64(id3[8]&0x7F)<<7 | int64(id3[9]&0x7F)
		start = 10 + tagSize
	}
	if start >= size {
		return 0, errors.New("mp3 duration: no audio frames")
	}

	buf := make([]byte, 4)
	pos := start
	var frames, totalSamples int
	var rate int
	for pos+4 <= size && frames < mp3MaxFrames {
		if _, err := f.ReadAt(buf, pos); err != nil {
			break
		}
		h, ok := parseMP3Header(buf)
		if !ok {
			pos++ // resync: scan forward one byte
			continue
		}
		frames++
		totalSamples += h.samples
		rate = h.sampleRate
		pos += int64(h.frameLen)
	}
	if frames == 0 || rate == 0 {
		return 0, errors.New("mp3 duration: no audio frames")
	}
	return time.Duration(float64(totalSamples) / float64(rate) * float64(time.Second)), nil
}
