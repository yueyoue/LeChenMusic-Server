package online

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// 上游广告/占位音频防护（见 docs/在线搜歌下载功能-开发文档.md 第 18 节）。
//
// 实测：flac.music.hi.cn 的 getUrl 触发风控后会对**任意歌曲**返回同一段
// "酷我音乐·看广告免费畅听"语音广告（约 12.8s / 48kbps / 76979 字节固定文件），
// 响应特征 format=mp3、bitrate=1（即使请求的是 flac/2000）。这类响应绝不能
// 入库，也不应拿去试听。三道防线：
//  1. 解析结果可信度校验（ResolvePlausible）：bitrate / duration 异常即拒绝；
//  2. 下载完成后按音频真实时长复核（ValidateDownloadedAudio）；
//  3. 命中即清直链缓存并重试解析，持续异常则任务失败并给出明确原因。

// ErrPlaceholderMedia marks failures caused by upstream ad/trial placeholder
// audio. Such failures are worth retrying: a later resolve often returns the
// real track again.
var ErrPlaceholderMedia = errors.New("上游返回广告占位音频")

const (
	minPlausibleBitrate  = 32 // kbps：真实音频最低档也在 32kbps 以上，广告占位实测 bitrate=1
	minPlausibleDuration = 20 // 秒：正常歌曲远长于此，广告占位实测约 12.8s
)

// durationTolerance returns the allowed |actual-expected| deviation in seconds:
// metadata durations are rounded and encoders add encoder-delay, so allow a
// small absolute error that grows with song length.
func durationTolerance(expected int) int {
	t := expected * 15 / 100
	if t < 20 {
		t = 20
	}
	return t
}

// minPlausibleBytesPerSecond is the floor for the size/duration cross-check:
// no real audio stream averages below 4kbps, so a probe result implying less
// proves the probe (not the file) is wrong.
const minPlausibleBytesPerSecond = 4000 / 8 // 4 kbps

// durationFitsFileSize cross-checks a probed duration against the file size on
// disk. A song can't average below ~4kbps, so a duration that implies a lower
// rate means the duration probe is corrupt (e.g. a damaged FLAC STREAMINFO
// reporting 79 hours for a 4-minute file) rather than that the file is bad.
//
// It returns true when it cannot judge (unreadable/empty file), leaving the
// duration check itself in charge.
func durationFitsFileSize(path string, seconds int) bool {
	if seconds <= 0 {
		return false
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Size() <= 0 {
		return true
	}
	return stat.Size() >= int64(seconds)*minPlausibleBytesPerSecond
}

// ResolvePlausible reports whether a resolved media URL looks like the real
// track instead of an upstream ad/trial placeholder. expectedDuration may be 0
// when the caller does not know the song length.
//
// Failures wrap ErrPlaceholderMedia so callers can retry.
func ResolvePlausible(media *ResolvedMedia, expectedDuration int) error {
	if media == nil || media.URL == "" {
		return fmt.Errorf("%w: empty media url", ErrPlaceholderMedia)
	}
	if media.Bitrate > 0 && media.Bitrate < minPlausibleBitrate {
		return fmt.Errorf("%w: bitrate=%dk（正常音频 ≥%dk）", ErrPlaceholderMedia, media.Bitrate, minPlausibleBitrate)
	}
	if media.Duration > 0 {
		if expectedDuration > 0 {
			diff := media.Duration - expectedDuration
			if diff < 0 {
				diff = -diff
			}
			if diff > durationTolerance(expectedDuration) {
				return fmt.Errorf("%w: 时长 %ds 与歌曲实际 %ds 相差过大", ErrPlaceholderMedia, media.Duration, expectedDuration)
			}
		} else if media.Duration < minPlausibleDuration {
			return fmt.Errorf("%w: 时长仅 %ds", ErrPlaceholderMedia, media.Duration)
		}
	}
	return nil
}

// ValidateDownloadedAudio verifies a fully downloaded audio file before it is
// ingested: the probed audio duration must roughly match the expected song
// length (or at least look like a full track when the length is unknown).
//
// Files whose duration cannot be probed (unknown container) pass: the resolve
// stage already rejects the known placeholder signatures.
//
// Failures wrap ErrPlaceholderMedia so callers can retry.
func ValidateDownloadedAudio(path string, expectedDuration int) error {
	probed, err := AudioDuration(path)
	if err != nil || probed <= 0 {
		return nil
	}
	seconds := int(probed.Seconds() + 0.5)
	if !durationFitsFileSize(path, seconds) {
		// 探测出的时长与文件体积自相矛盾（例如 STREAMINFO 的总采样数损坏），
		// 说明是探测结果失真而不是“文件太短/太长”。放行，交给解析阶段的
		// bitrate/时长校验兜底，避免把正常歌曲误判成广告占位。
		return nil
	}
	if expectedDuration > 0 {
		diff := seconds - expectedDuration
		if diff < 0 {
			diff = -diff
		}
		if diff > durationTolerance(expectedDuration) {
			return fmt.Errorf("%w: 文件时长 %ds 与歌曲实际 %ds 相差过大", ErrPlaceholderMedia, seconds, expectedDuration)
		}
		return nil
	}
	if seconds < minPlausibleDuration {
		return fmt.Errorf("%w: 文件时长仅 %ds（疑似占位片段）", ErrPlaceholderMedia, seconds)
	}
	return nil
}

// URLInvalidator lets a Source drop cached direct URLs for a song/quality so a
// retry re-resolves instead of replaying the same (possibly ad) URL. Optional:
// sources that do not cache simply don't implement it.
type URLInvalidator interface {
	InvalidateURL(song SongRef, q Quality)
}

// InvalidateResolvedURL drops any cached resolve result for song/q when the
// source supports it. Best-effort: sources without a cache are unaffected.
func InvalidateResolvedURL(src Source, song SongRef, q Quality) {
	if inv, ok := src.(URLInvalidator); ok {
		inv.InvalidateURL(song, q)
	}
}

// ResolveValidated resolves a direct media URL and rejects ad/placeholder
// responses, re-resolving (cache invalidated) up to attempts times before
// giving up. The final error wraps ErrPlaceholderMedia when every attempt was
// rejected as placeholder audio.
func ResolveValidated(ctx context.Context, src Source, song SongRef, q Quality, expectedDuration, attempts int) (*ResolvedMedia, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			InvalidateResolvedURL(src, song, q)
		}
		media, err := src.ResolveURL(ctx, song, q)
		if err != nil {
			lastErr = err
			continue
		}
		if err := ResolvePlausible(media, expectedDuration); err != nil {
			lastErr = err
			continue
		}
		return media, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: no media", ErrPlaceholderMedia)
	}
	return nil, lastErr
}
