package online

import (
	"errors"
	"testing"
)

func TestResolvePlausibleRejectsAdBitrate(t *testing.T) {
	// The observed upstream ad payload: format=mp3, bitrate=1.
	media := &ResolvedMedia{URL: "http://cdn/ad.mp3", Format: "mp3", Bitrate: 1, Duration: 245}
	err := ResolvePlausible(media, 245)
	if !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
}

func TestResolvePlausibleRejectsDurationMismatch(t *testing.T) {
	// 12s clip served for a 245s song, even if the bitrate looks fine.
	media := &ResolvedMedia{URL: "http://cdn/clip.mp3", Format: "mp3", Bitrate: 320, Duration: 12}
	if err := ResolvePlausible(media, 245); !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
}

func TestResolvePlausibleRejectsShortClipWithoutExpectation(t *testing.T) {
	media := &ResolvedMedia{URL: "http://cdn/clip.mp3", Format: "mp3", Bitrate: 320, Duration: 12}
	if err := ResolvePlausible(media, 0); !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
}

func TestResolvePlausibleAcceptsRealTrack(t *testing.T) {
	media := &ResolvedMedia{URL: "http://cdn/song.flac", Format: "flac", Bitrate: 2000, Duration: 245}
	if err := ResolvePlausible(media, 245); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Unknown expected duration: a full-length track still passes.
	if err := ResolvePlausible(media, 0); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Unknown payload duration (some sources omit it) also passes.
	media2 := &ResolvedMedia{URL: "http://cdn/song.mp3", Format: "mp3", Bitrate: 320}
	if err := ResolvePlausible(media2, 245); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestResolvePlausibleTolerance(t *testing.T) {
	// ±3s off the metadata duration is normal (rounding / encoder delay).
	media := &ResolvedMedia{URL: "http://cdn/song.mp3", Format: "mp3", Bitrate: 320, Duration: 242}
	if err := ResolvePlausible(media, 245); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// 40s off for a 245s song is not.
	media.Duration = 205
	if err := ResolvePlausible(media, 245); !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
}

func TestValidateDownloadedAudioRejectsAdFile(t *testing.T) {
	// The real ad file: ~12.8s of audio downloaded as a 4-minute song.
	path := t.TempDir() + "/ad.mp3"
	writeTestFile(t, path, mp3Frames(t, 493)) // 493 frames ≈ 12.8s
	if err := ValidateDownloadedAudio(path, 245); !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
	// Without an expected duration the short-clip heuristic still catches it.
	if err := ValidateDownloadedAudio(path, 0); !errors.Is(err, ErrPlaceholderMedia) {
		t.Fatalf("expected ErrPlaceholderMedia, got %v", err)
	}
}

func TestValidateDownloadedAudioAcceptsRealFile(t *testing.T) {
	path := t.TempDir() + "/song.flac"
	writeTestFile(t, path, flacWithDuration(t, 44100, 245*44100))
	if err := ValidateDownloadedAudio(path, 245); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateDownloadedAudioIgnoresCorruptDurationProbe(t *testing.T) {
	// Field case: a 274s song whose STREAMINFO total-samples is damaged, so the
	// probe reports 286089s (≈79h). A 42-byte file cannot hold 79 hours of audio,
	// so the probe—not the file—is wrong; the download must be accepted.
	path := t.TempDir() + "/corrupt.flac"
	writeTestFile(t, path, flacWithDuration(t, 44100, 286089*44100))
	if err := ValidateDownloadedAudio(path, 274); err != nil {
		t.Fatalf("corrupt duration probe must not reject a valid download: %v", err)
	}
}

func TestValidateDownloadedAudioSkipsUnknownContainer(t *testing.T) {
	path := t.TempDir() + "/song.bin"
	writeTestFile(t, path, []byte("not audio"))
	if err := ValidateDownloadedAudio(path, 245); err != nil {
		t.Fatalf("unknown containers must pass (resolve stage already screened): %v", err)
	}
}
