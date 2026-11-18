package online

import (
	"encoding/binary"
	"os"
	"testing"
	"time"
)

// flacWithDuration builds a minimal FLAC whose STREAMINFO reports the given
// sample rate and total sample count (i.e. a known duration).
func flacWithDuration(t *testing.T, sampleRate uint32, totalSamples uint64) []byte {
	t.Helper()
	buf := make([]byte, 0, 42)
	buf = append(buf, []byte("fLaC")...)
	buf = append(buf, 0x80, 0x00, 0x00, 0x22) // STREAMINFO, last block, len 34
	info := make([]byte, 34)
	binary.BigEndian.PutUint16(info[0:2], 16)   // min block size
	binary.BigEndian.PutUint16(info[2:4], 4096) // max block size
	info[10] = byte(sampleRate >> 12)
	info[11] = byte(sampleRate >> 4)
	info[12] = byte(sampleRate&0x0F)<<4 | byte(totalSamples>>32)&0x0F
	info[13] = byte(totalSamples >> 24)
	info[14] = byte(totalSamples >> 16)
	info[15] = byte(totalSamples >> 8)
	info[16] = byte(totalSamples)
	buf = append(buf, info...)
	buf = append(buf, 0xFF, 0xF8) // minimal frame sync so the stream looks alive
	return buf
}

// mp3Frames builds an MP3 of n silent MPEG-1 Layer III frames (44.1kHz,
// 1152 samples/frame each).
func mp3Frames(t *testing.T, n int) []byte {
	t.Helper()
	frame := minimalMP3(t) // 417 bytes, MPEG-1 L3, 44100 Hz
	buf := make([]byte, 0, n*len(frame))
	for i := 0; i < n; i++ {
		buf = append(buf, frame...)
	}
	return buf
}

func TestAudioDurationFLAC(t *testing.T) {
	path := t.TempDir() + "/song.flac"
	// 3 seconds at 44100 Hz
	writeTestFile(t, path, flacWithDuration(t, 44100, 3*44100))

	d, err := AudioDuration(path)
	if err != nil {
		t.Fatalf("AudioDuration: %v", err)
	}
	if diff := d - 3*time.Second; diff > 100*time.Millisecond || diff < -100*time.Millisecond {
		t.Errorf("expected ~3s, got %v", d)
	}
}

func TestAudioDurationMP3(t *testing.T) {
	path := t.TempDir() + "/song.mp3"
	// 80 frames × 1152 samples / 44100 Hz ≈ 2.09s
	writeTestFile(t, path, mp3Frames(t, 80))

	d, err := AudioDuration(path)
	if err != nil {
		t.Fatalf("AudioDuration: %v", err)
	}
	frames := 80
	want := time.Duration(float64(frames*1152) / 44100 * float64(time.Second))
	if diff := d - want; diff > 50*time.Millisecond || diff < -50*time.Millisecond {
		t.Errorf("expected %v, got %v", want, d)
	}
}

func TestAudioDurationMP3SkipsID3v2(t *testing.T) {
	audio := mp3Frames(t, 80)
	tag := make([]byte, 10)
	copy(tag, "ID3")
	tag[3], tag[4] = 3, 0 // version
	payload := 20         // tag body size (syncsafe below)
	tag[6] = byte(payload >> 21)
	tag[7] = byte(payload >> 14)
	tag[8] = byte(payload >> 7)
	tag[9] = byte(payload)
	body := append(tag, make([]byte, payload)...)
	body = append(body, audio...)

	path := t.TempDir() + "/tagged.mp3"
	writeTestFile(t, path, body)

	d, err := AudioDuration(path)
	if err != nil {
		t.Fatalf("AudioDuration: %v", err)
	}
	if d < 2*time.Second || d > 2200*time.Millisecond {
		t.Errorf("expected ~2.09s, got %v", d)
	}
}

func TestAudioDurationFLACUnknownLength(t *testing.T) {
	path := t.TempDir() + "/stream.flac"
	writeTestFile(t, path, flacWithDuration(t, 44100, 0))
	if _, err := AudioDuration(path); err == nil {
		t.Error("expected error for unknown stream length")
	}
}

func TestAudioDurationUnsupportedContainer(t *testing.T) {
	path := t.TempDir() + "/song.wav"
	writeTestFile(t, path, []byte("RIFF....WAVEfmt "))
	if _, err := AudioDuration(path); err == nil {
		t.Error("expected error for unsupported container")
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
