package online

import "testing"

func TestHiResCoverURL(t *testing.T) {
	cases := map[string]string{
		"https://img1.kuwo.cn/star/albumcover/120/28/42/1776247091.jpg":  "https://img1.kuwo.cn/star/albumcover/1000/28/42/1776247091.jpg",
		"https://img1.kuwo.cn/star/starheads/120/s4s5/17/3477575663.jpg": "https://img1.kuwo.cn/star/starheads/1000/s4s5/17/3477575663.jpg",
		"https://img2.kuwo.cn/star/albumcover/300/1/2/3.jpg":             "https://img2.kuwo.cn/star/albumcover/1000/1/2/3.jpg",
		// non-kuwo / unknown patterns pass through untouched
		"https://example.com/cover.jpg": "https://example.com/cover.jpg",
		"":                              "",
	}
	for in, want := range cases {
		if got := hiResCoverURL(in); got != want {
			t.Errorf("hiResCoverURL(%q) = %q, want %q", in, got, want)
		}
	}
}
