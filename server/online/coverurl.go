package online

import "regexp"

// Kuwo cover CDNs size the image by a path segment
// (e.g. /star/albumcover/120/…jpg serves a 120px thumbnail). When embedding
// covers into audio files we want full-size art, so bump the size to 1000px.
var kuwoCoverPath = regexp.MustCompile(`(img\d*\.kuwo\.cn/star/(?:albumcover|starheads)/)\d+/`)

// hiResCoverURL rewrites known thumbnail URL patterns to their high-res form.
// Unknown URLs are returned unchanged. (Search results keep the small URL —
// UI thumbnails don't need more — only fetchCover upgrades before embedding.)
func hiResCoverURL(url string) string {
	return kuwoCoverPath.ReplaceAllString(url, "${1}1000/")
}
