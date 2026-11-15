package online

import "testing"

func TestCleanSongTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"相思风雨中-《出位江湖》电视剧插曲", "相思风雨中"},
		{"海阔天空 (Live)", "海阔天空"},
		{"海阔天空（live）", "海阔天空"},
		{"吻别-电影《xx》主题曲", "吻别"},
		{"朋友-电视剧插曲", "朋友"},
		{"至少还有你（伴奏）", "至少还有你"},
		{"真的爱你", "真的爱你"},
		{"爱-情", "爱-情"}, // real title with dash, no annotation keywords
		{"Hello-World", "Hello-World"},
		{"", ""},
		{"  让我欢喜让我忧 - 现场版  ", "让我欢喜让我忧"},
	}
	for _, c := range cases {
		if got := CleanSongTitle(c.in); got != c.want {
			t.Errorf("CleanSongTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeArtist(t *testing.T) {
	cases := []struct{ in, want string }{
		{"张学友&汤宝如", "张学友 / 汤宝如"},
		{"刘若英 • 杨坤", "刘若英 / 杨坤"},
		{"凤凰传奇 • 林志炫 • 何炅", "凤凰传奇 / 林志炫 / 何炅"},
		{"黄家强&黄贯中&叶世荣", "黄家强 / 黄贯中 / 叶世荣"},
		{"BEYOND", "BEYOND"},
		{"张三,李四", "张三 / 李四"},
		{"张三、李四", "张三 / 李四"},
		{"A / B", "A / B"},
		{"张三 张三", "张三 张三"}, // single name containing space is untouched
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeArtist(c.in); got != c.want {
			t.Errorf("NormalizeArtist(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeArtistDedup(t *testing.T) {
	if got := NormalizeArtist("张三&李四&张三"); got != "张三 / 李四" {
		t.Errorf("expected dedup, got %q", got)
	}
}
