package online

import "testing"

func TestPickQualityPrefersRequested(t *testing.T) {
	available := []QualityOption{
		{Format: "flac", Bitrate: 2000},
		{Format: "mp3", Bitrate: 320},
		{Format: "mp3", Bitrate: 128},
	}
	got, ok := PickQuality(available, Quality{Format: "mp3", Bitrate: 128})
	if !ok || got.Format != "mp3" || got.Bitrate != 128 {
		t.Fatalf("unexpected pick: %+v ok=%v", got, ok)
	}
}

func TestPickQualityFallsBackToFlac(t *testing.T) {
	available := []QualityOption{
		{Format: "flac", Bitrate: 2000},
		{Format: "mp3", Bitrate: 320},
	}
	// User pinned 128k mp3 which is unavailable -> default chain prefers flac.
	got, ok := PickQuality(available, Quality{Format: "mp3", Bitrate: 128})
	if !ok || got.Format != "flac" || got.Bitrate != 2000 {
		t.Fatalf("unexpected pick: %+v ok=%v", got, ok)
	}
}

func TestPickQualityDefaultChain(t *testing.T) {
	available := []QualityOption{
		{Format: "mp3", Bitrate: 128},
		{Format: "mp3", Bitrate: 320},
	}
	got, ok := PickQuality(available, Quality{})
	if !ok || got.Format != "mp3" || got.Bitrate != 320 {
		t.Fatalf("unexpected pick: %+v ok=%v", got, ok)
	}
}

func TestPickQualityLastResortHighestBitrate(t *testing.T) {
	available := []QualityOption{
		{Format: "aac", Bitrate: 96},
		{Format: "ogg", Bitrate: 500},
	}
	got, ok := PickQuality(available, Quality{})
	if !ok || got.Format != "ogg" || got.Bitrate != 500 {
		t.Fatalf("unexpected pick: %+v ok=%v", got, ok)
	}
}

func TestPickQualityEmpty(t *testing.T) {
	if _, ok := PickQuality(nil, Quality{}); ok {
		t.Fatal("expected no match for empty availability")
	}
}
