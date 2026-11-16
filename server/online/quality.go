package online

// DefaultQualityChain is the preferred order used when the user does not pin a
// specific quality: FLAC first, then MP3 320k.
//
// NOTE: mp3/128 is deliberately absent — upstream (hicn/kuwo) answers
// bitrate=128 with a generic 12-second trial-clip placeholder (same audio for
// every song), never the real track. A 320k request transparently downgrades
// to a real 128k full track when 320k is unavailable, so it covers that case.
var DefaultQualityChain = []Quality{
	{Format: "flac", Bitrate: 2000},
	{Format: "mp3", Bitrate: 320},
}

// PickQuality selects the best quality option for a song.
//
// If want is set and available it is returned as-is. Otherwise the
// DefaultQualityChain is walked in order and the first available option is
// picked. As a last resort the highest-bitrate available option is returned.
// The second return value reports whether any option matched.
func PickQuality(available []QualityOption, want Quality) (QualityOption, bool) {
	if len(available) == 0 {
		return QualityOption{}, false
	}
	if !want.IsZero() {
		for _, opt := range available {
			if opt.Format == want.Format && opt.Bitrate == want.Bitrate {
				return opt, true
			}
		}
		// Pinned quality unavailable: fall through to the default chain.
	}
	for _, q := range DefaultQualityChain {
		for _, opt := range available {
			if opt.Format == q.Format && opt.Bitrate == q.Bitrate {
				return opt, true
			}
		}
	}
	// Last resort: highest bitrate available.
	best := available[0]
	for _, opt := range available[1:] {
		if opt.Bitrate > best.Bitrate {
			best = opt
		}
	}
	return best, true
}
