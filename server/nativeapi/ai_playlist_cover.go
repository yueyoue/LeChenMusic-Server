package nativeapi

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"math/rand"
	"strings"

	"github.com/navidrome/navidrome/log"
)

// [LeChenMusic-START:ai-playlist-cover]

type coverTheme struct {
	Bg1    color.RGBA
	Bg2    color.RGBA
	Accent color.RGBA
}

var coverThemes = map[string]coverTheme{
	"default":  {Bg1: color.RGBA{99, 102, 241, 255}, Bg2: color.RGBA{139, 92, 246, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"民谣":     {Bg1: color.RGBA{180, 140, 80, 255}, Bg2: color.RGBA{120, 80, 40, 255}, Accent: color.RGBA{255, 240, 210, 255}},
	"摇滚":     {Bg1: color.RGBA{180, 30, 30, 255}, Bg2: color.RGBA{100, 10, 10, 255}, Accent: color.RGBA{255, 200, 100, 255}},
	"电子":     {Bg1: color.RGBA{0, 200, 200, 255}, Bg2: color.RGBA{0, 50, 150, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"古典":     {Bg1: color.RGBA{60, 60, 80, 255}, Bg2: color.RGBA{30, 20, 40, 255}, Accent: color.RGBA{212, 175, 55, 255}},
	"流行":     {Bg1: color.RGBA{236, 72, 153, 255}, Bg2: color.RGBA{168, 85, 247, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"爵士":     {Bg1: color.RGBA{50, 50, 70, 255}, Bg2: color.RGBA{20, 20, 35, 255}, Accent: color.RGBA{218, 165, 32, 255}},
	"嘻哈":     {Bg1: color.RGBA{255, 165, 0, 255}, Bg2: color.RGBA{200, 50, 0, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"R&B":      {Bg1: color.RGBA{100, 50, 150, 255}, Bg2: color.RGBA{40, 10, 60, 255}, Accent: color.RGBA{255, 200, 220, 255}},
	"轻音乐":   {Bg1: color.RGBA{100, 180, 200, 255}, Bg2: color.RGBA{60, 120, 160, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"国风":     {Bg1: color.RGBA{180, 50, 50, 255}, Bg2: color.RGBA{80, 20, 20, 255}, Accent: color.RGBA{255, 215, 0, 255}},
	"日语":     {Bg1: color.RGBA{255, 183, 197, 255}, Bg2: color.RGBA{255, 105, 140, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"韩语":     {Bg1: color.RGBA{135, 206, 235, 255}, Bg2: color.RGBA{70, 130, 180, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"粤语":     {Bg1: color.RGBA{200, 150, 50, 255}, Bg2: color.RGBA{120, 80, 20, 255}, Accent: color.RGBA{255, 255, 240, 255}},
	"深夜":     {Bg1: color.RGBA{30, 30, 60, 255}, Bg2: color.RGBA{10, 10, 30, 255}, Accent: color.RGBA{180, 180, 255, 255}},
	"早晨":     {Bg1: color.RGBA{255, 200, 100, 255}, Bg2: color.RGBA{255, 140, 50, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"运动":     {Bg1: color.RGBA{0, 200, 100, 255}, Bg2: color.RGBA{0, 100, 150, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"80后":     {Bg1: color.RGBA{180, 120, 60, 255}, Bg2: color.RGBA{100, 60, 30, 255}, Accent: color.RGBA{255, 230, 180, 255}},
	"90后":     {Bg1: color.RGBA{100, 150, 255, 255}, Bg2: color.RGBA{60, 80, 200, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"伤感":     {Bg1: color.RGBA{80, 80, 120, 255}, Bg2: color.RGBA{40, 40, 70, 255}, Accent: color.RGBA{180, 200, 255, 255}},
	"治愈":     {Bg1: color.RGBA{150, 220, 180, 255}, Bg2: color.RGBA{80, 160, 120, 255}, Accent: color.RGBA{255, 255, 255, 255}},
	"浪漫":     {Bg1: color.RGBA{255, 150, 180, 255}, Bg2: color.RGBA{200, 80, 120, 255}, Accent: color.RGBA{255, 255, 255, 255}},
}

func getAvailableThemes() []string {
	themes := make([]string, 0, len(coverThemes))
	for name := range coverThemes {
		themes = append(themes, name)
	}
	return themes
}

func getThemeForTitle(title string) coverTheme {
	titleLower := strings.ToLower(title)
	for keyword, theme := range coverThemes {
		if strings.Contains(titleLower, keyword) {
			return theme
		}
	}
	// Random theme
	keys := make([]string, 0, len(coverThemes))
	for k := range coverThemes {
		keys = append(keys, k)
	}
	return coverThemes[keys[rand.Intn(len(keys))]]
}

func lerpColor(c1, c2 color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c1.R)*(1-t) + float64(c2.R)*t),
		G: uint8(float64(c1.G)*(1-t) + float64(c2.G)*t),
		B: uint8(float64(c1.B)*(1-t) + float64(c2.B)*t),
		A: 255,
	}
}

func drawGradient(img *image.RGBA, c1, c2 color.RGBA) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		t := float64(y) / float64(bounds.Max.Y-bounds.Min.Y)
		c := lerpColor(c1, c2, t)
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func drawCircles(img *image.RGBA, c color.RGBA) {
	bounds := img.Bounds()
	dim := color.RGBA{
		R: uint8(float64(c.R) * 0.15),
		G: uint8(float64(c.G) * 0.15),
		B: uint8(float64(c.B) * 0.15),
		A: 255,
	}
	for i := 0; i < 15; i++ {
		cx := rand.Intn(bounds.Max.X)
		cy := rand.Intn(bounds.Max.Y)
		r := rand.Intn(80) + 20
		for angle := 0; angle < 360; angle += 2 {
			rad := float64(angle) * math.Pi / 180
			x := cx + int(float64(r)*math.Cos(rad))
			y := cy + int(float64(r)*math.Sin(rad))
			if x >= bounds.Min.X && x < bounds.Max.X && y >= bounds.Min.Y && y < bounds.Max.Y {
				img.Set(x, y, dim)
			}
		}
	}
}

func drawDots(img *image.RGBA, c color.RGBA) {
	dim := color.RGBA{
		R: uint8(float64(c.R) * 0.15),
		G: uint8(float64(c.G) * 0.15),
		B: uint8(float64(c.B) * 0.15),
		A: 255,
	}
	bounds := img.Bounds()
	for i := 0; i < 50; i++ {
		cx := rand.Intn(bounds.Max.X)
		cy := rand.Intn(bounds.Max.Y)
		r := rand.Intn(6) + 2
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dy*dy <= r*r {
					x, y := cx+dx, cy+dy
					if x >= bounds.Min.X && x < bounds.Max.X && y >= bounds.Min.Y && y < bounds.Max.Y {
						img.Set(x, y, dim)
					}
				}
			}
		}
	}
}

func drawTextCentered(img *image.RGBA, text string, c color.RGBA, yCenter int) {
	bounds := img.Bounds()
	// Simple bitmap font rendering - each character is 8x12 pixels
	// For Chinese characters, we'll use a simplified approach
	charWidth := 8
	if isChinese(text) {
		charWidth = 14
	}
	totalWidth := len([]rune(text)) * charWidth
	startX := (bounds.Max.X - totalWidth) / 2

	// Draw text as simple rectangles (placeholder - real implementation would use font rendering)
	for i, ch := range text {
		x := startX + i*charWidth
		drawChar(img, x, yCenter-6, ch, c, charWidth)
	}
}

func isChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func drawChar(img *image.RGBA, x, y int, ch rune, c color.RGBA, width int) {
	bounds := img.Bounds()
	// Simple pixel art for common characters
	// This is a simplified version - a real implementation would use font files
	pattern := getCharPattern(ch)
	if pattern == nil {
		// Default: draw a filled rectangle
		for dy := 0; dy < 12; dy++ {
			for dx := 0; dx < width; dx++ {
				px, py := x+dx, y+dy
				if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
					img.Set(px, py, c)
				}
			}
		}
		return
	}
	for dy, row := range pattern {
		for dx, pixel := range row {
			if pixel == 1 {
				px, py := x+dx, y+dy
				if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
					img.Set(px, py, c)
				}
			}
		}
	}
}

func getCharPattern(ch rune) [][]int {
	// Simplified 8x12 pixel patterns for common characters
	// 1 = filled, 0 = empty
	patterns := map[rune][][]int{
		'A': {
			{0, 0, 1, 1, 1, 1, 0, 0},
			{0, 1, 1, 0, 0, 1, 1, 0},
			{1, 1, 0, 0, 0, 0, 1, 1},
			{1, 1, 0, 0, 0, 0, 1, 1},
			{1, 1, 1, 1, 1, 1, 1, 1},
			{1, 1, 0, 0, 0, 0, 1, 1},
			{1, 1, 0, 0, 0, 0, 1, 1},
			{1, 1, 0, 0, 0, 0, 1, 1},
		},
		'I': {
			{1, 1, 1, 1, 1, 1, 1, 1},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{0, 0, 0, 1, 1, 0, 0, 0},
			{1, 1, 1, 1, 1, 1, 1, 1},
		},
	}
	if p, ok := patterns[ch]; ok {
		return p
	}
	return nil
}

func generatePlaylistCover(title string, songCount int, themeName string) ([]byte, error) {
	const size = 600
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Get theme
	var theme coverTheme
	if themeName != "" {
		if t, ok := coverThemes[themeName]; ok {
			theme = t
		} else {
			theme = getThemeForTitle(title)
		}
	} else {
		theme = getThemeForTitle(title)
	}

	// Draw gradient background
	drawGradient(img, theme.Bg1, theme.Bg2)

	// Draw decorative pattern
	patterns := []func(*image.RGBA, color.RGBA){drawCircles, drawDots}
	patterns[rand.Intn(len(patterns))](img, theme.Accent)

	// Draw title
	drawTextCentered(img, title, theme.Accent, size/2-20)

	// Draw subtitle
	if songCount > 0 {
		subtitle := fmt.Sprintf("%d 首歌曲 · AI 生成", songCount)
		subColor := theme.Accent
		subColor.A = 153 // 60% opacity
		drawTextCentered(img, subtitle, subColor, size/2+40)
	}

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		log.Error("AI Playlist: Cover encode failed", "error", err)
		return nil, err
	}
	return buf.Bytes(), nil
}

// [LeChenMusic-END:ai-playlist-cover]
