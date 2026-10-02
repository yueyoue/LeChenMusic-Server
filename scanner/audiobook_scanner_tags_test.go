package scanner

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

// 音频资源自带的封面之外的其他信息（作者/演播者/简介/系列/年份…）入库识别的映射规则：
// 标签 → 书目字段。纯函数单测，不依赖二进制 fixture。
func TestTagsFromMap(t *testing.T) {
	cases := []struct {
		name string
		tags map[string][]string
		want audiobookTags
	}{
		{
			name: "基础字段：TITLE/ARTIST/ALBUM/GENRE/DATE，演播者退回 ARTIST",
			tags: map[string][]string{
				"TITLE":  {"贝姨"},
				"ARTIST": {"艾宝良"},
				"ALBUM":  {"贝姨"},
				"GENRE":  {"有声读物"},
				"DATE":   {"2019"},
			},
			want: audiobookTags{title: "贝姨", artist: "艾宝良", album: "贝姨", genre: "有声读物", year: 2019, narrator: "艾宝良"},
		},
		{
			name: "演播者优先级：COMPOSER > TXXX:NARRATOR > ARTIST",
			tags: map[string][]string{
				"COMPOSER":      {"单田芳"},
				"TXXX:NARRATOR": {"艾宝良"},
				"ARTIST":        {"不该被选中"},
			},
			want: audiobookTags{artist: "不该被选中", narrator: "单田芳"},
		},
		{
			name: "没有 COMPOSER 时演播者用 TXXX:NARRATOR",
			tags: map[string][]string{
				"TXXX:NARRATOR": {"艾宝良"},
				"ARTIST":        {"不该被选中"},
			},
			want: audiobookTags{artist: "不该被选中", narrator: "艾宝良"},
		},
		{
			name: "没有 ARTIST 时演播者退回 ALBUMARTIST",
			tags: map[string][]string{
				"ALBUMARTIST": {"艾宝良"},
			},
			want: audiobookTags{artist: "艾宝良", narrator: "艾宝良"},
		},
		{
			name: "作者只认显式作者标签，ARTIST（演播者）不能当作者",
			tags: map[string][]string{
				"TXXX:AUTHOR": {"巴尔扎克"},
				"ARTIST":      {"艾宝良"},
			},
			want: audiobookTags{artist: "艾宝良", narrator: "艾宝良", author: "巴尔扎克"},
		},
		{
			name: "没有显式作者标签时作者留空（宁缺毋滥，不把演播者标成作者）",
			tags: map[string][]string{
				"ARTIST": {"艾宝良"},
			},
			want: audiobookTags{artist: "艾宝良", narrator: "艾宝良"},
		},
		{
			name: "简介：DESCRIPTION 优先于 COMMENT",
			tags: map[string][]string{
				"DESCRIPTION": {"长篇小说"},
				"COMMENT":     {"备用注释"},
			},
			want: audiobookTags{description: "长篇小说"},
		},
		{
			name: "简介：COMMENT 兜底，TXXX 变体也认",
			tags: map[string][]string{
				"COMMENT":          {"备用注释"},
				"TXXX:DESCRIPTION": {"不该被选中"},
			},
			want: audiobookTags{description: "备用注释"},
		},
		{
			name: "系列：SERIES 与 TXXX:SERIES",
			tags: map[string][]string{
				"TXXX:SERIES": {"人间喜剧"},
			},
			want: audiobookTags{series: "人间喜剧"},
		},
		{
			name: "DATE 带月日时也能解析出年份",
			tags: map[string][]string{
				"DATE": {"2014-05-21"},
			},
			want: audiobookTags{year: 2014},
		},
		{
			name: "DATE 解析失败年份为 0",
			tags: map[string][]string{
				"DATE": {"不明"},
			},
			want: audiobookTags{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tagsFromMap(tc.tags)
			if got != tc.want {
				t.Errorf("tagsFromMap(%v)\n = %+v,\nwant %+v", tc.tags, got, tc.want)
			}
		})
	}
}

// 入库时音频文件自带的其他信息（演播者/简介/流派/年份…）必须落库可显示。
// tests/fixtures/test.mp3 自带 COMPOSER/COMMENT/GENRE/DATE 等标签。
func TestAudiobookScanIngestsEmbeddedTags(t *testing.T) {
	tests.Init(t, false)

	libDir := path.Join("tests", "fixtures", "audiobooks-embedded-tags")
	t.Cleanup(func() { _ = os.RemoveAll(libDir) })
	bookDir := path.Join(libDir, "某书")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mp3, err := os.ReadFile("tests/fixtures/test.mp3")
	if err != nil {
		t.Fatalf("missing audio fixture: %v", err)
	}
	if err := os.WriteFile(path.Join(bookDir, "01.mp3"), mp3, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ds := &tests.MockDataStore{MockedAudiobook: tests.CreateMockAudiobookRepo()}
	if err := NewAudiobookScanner(ds).ScanLibrary(ctx, model.Library{ID: 987658, Name: "本地标签", Path: libDir}); err != nil {
		t.Fatal(err)
	}
	books, err := ds.Audiobook(ctx).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	book := books[0]
	// fixture 自带的标签必须入库：COMPOSER=演播者、COMMENT=简介、GENRE、DATE(2014-05-21)
	if book.Narrator != "coma a" {
		t.Errorf("Narrator = %q, want coma a（COMPOSER 标签）", book.Narrator)
	}
	if book.Description != "Comment1\nComment2" {
		t.Errorf("Description = %q, want COMMENT 标签内容", book.Description)
	}
	if book.Genre != "Rock" {
		t.Errorf("Genre = %q, want Rock", book.Genre)
	}
	if book.Year != 2014 {
		t.Errorf("Year = %d, want 2014（DATE=2014-05-21）", book.Year)
	}
}
