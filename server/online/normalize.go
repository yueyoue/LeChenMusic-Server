package online

import (
	"regexp"
	"strings"
)

// CleanSongTitle strips media-annotation suffixes from upstream song titles so
// the library stores clean titles, e.g.:
//
//	"相思风雨中-《出位江湖》电视剧插曲"  → "相思风雨中"
//	"海阔天空 (Live)"                   → "海阔天空"
//	"真的爱你"                          → "真的爱你" (unchanged)
//
// The cut is conservative: a separator suffix is only removed when it looks
// like an annotation (contains 《》/brackets or a media keyword).
var (
	titleSeparator = regexp.MustCompile(`[-—–－]`)
	titleAnnot     = regexp.MustCompile(`[（(【\[]`)
	annotKeywords  = regexp.MustCompile(`(?i)《|》|电视剧|插曲|主题曲|片尾曲|片头曲|电影|动画|网剧|微电影|综艺|现场|live|remix|版|伴奏|合唱|翻唱|铃声|铃音|主题歌|主打歌|ost|dj|剧情|片段|剪辑`)
)

// CleanSongTitle returns the title without trailing annotation suffixes.
func CleanSongTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return t
	}

	// Cut at the first separator/bracket whose suffix looks like an annotation.
	cut := len(t)
	for i, r := range t {
		if r == '-' || r == '—' || r == '–' || r == '－' || r == '（' || r == '(' || r == '【' || r == '[' {
			suffix := t[i+1:]
			// "-《xx》…" / "- (Live)" / "- 电视剧插曲" / "（伴奏）" are annotations;
			// "爱-情" or "Hello-World" style real titles are kept intact.
			if annotKeywords.MatchString(suffix) {
				cut = i
				break
			}
		}
	}
	t = strings.TrimSpace(t[:cut])
	t = strings.Trim(t, "-—–－ 　")
	if t == "" {
		return strings.TrimSpace(title)
	}
	return t
}

var artistSeparators = regexp.MustCompile(`\s*([&、,，;；/|×xX]| x )\s*`)

// NormalizeArtist splits a multi-artist string into individual artists and
// rejoins them with " / " (the separator the scanner splits on, which is then
// displayed with the library's " • " joiner). Examples:
//
//	"张学友&汤宝如"          → "张学友 / 汤宝如"      (displayed: 张学友 • 汤宝如)
//	"凤凰传奇 • 林志炫 • 何炅" → "凤凰传奇 / 林志炫 / 何炅"
//	"BEYOND"                → "BEYOND" (unchanged)
func NormalizeArtist(artist string) string {
	a := strings.TrimSpace(artist)
	if a == "" {
		return a
	}
	// The display form "A • B" is already split on the library joiner.
	a = strings.ReplaceAll(a, "•", "&")

	parts := artistSeparators.Split(a, -1)
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return strings.TrimSpace(artist)
	}
	return strings.Join(out, " / ")
}

// albumArtistSeparators splits album credits only on unambiguous separators:
// unlike artistSeparators it never splits on "x"/"X", which would mangle album
// artists such as "X JAPAN" or "Texas".
var albumArtistSeparators = regexp.MustCompile(`\s*([&、,，;；/|×])\s*`)

// NormalizeAlbumArtist cleans an album-level artist credit: "•" becomes the
// library joiner, multi-artist credits are rejoined with " / " (the separator
// the scanner splits on) and duplicates are dropped.
func NormalizeAlbumArtist(artist string) string {
	a := strings.TrimSpace(artist)
	if a == "" {
		return a
	}
	a = strings.ReplaceAll(a, "•", "&")

	parts := albumArtistSeparators.Split(a, -1)
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return strings.TrimSpace(artist)
	}
	return strings.Join(out, " / ")
}
