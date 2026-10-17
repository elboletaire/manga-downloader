// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestComiciTest(t *testing.T) {
	c := &Comici{Grabber: &Grabber{URL: "https://comic-growl.com/series/842677db37980"}}
	if ok, _ := c.Test(); !ok {
		t.Error("expected a Comici host to match")
	}
	c.URL = "https://example.com/episodes/c1ffef58c01d8"
	if ok, _ := c.Test(); ok {
		t.Error("expected a foreign host not to match")
	}
}

func TestComiciTitle(t *testing.T) {
	tests := []struct {
		raw, title, part string
		number           float64
	}{
		{"第1話　血まみれのカオリス", "血まみれのカオリス", "", 1},
		{"第34話①", "", "1", 34},
		{"第36話前編①", "", "前編1", 36},
		{"第36話前編", "", "前編", 36},
		{"第35話　黒の衝撃", "黒の衝撃", "", 35},
		{"0話", "", "", 0},
		{"1", "", "", 1},
		{"2-①", "", "1", 2},
		{"19-②", "", "2", 19},
		{"#9.5", "", "", 9.5},
		{"#1", "", "", 1},
		{"番外編", "番外編", "", 0},
		{"#特別版", "#特別版", "", 0},
	}
	for _, tt := range tests {
		title, number, part := comiciTitle(tt.raw)
		if title != tt.title || number != tt.number || part != tt.part {
			t.Errorf("comiciTitle(%q) = (%q, %v, %q), want (%q, %v, %q)",
				tt.raw, title, number, part, tt.title, tt.number, tt.part)
		}
	}
}

// TestComiciIsFree reads the badge that marks an episode free: the one with
// data-e2e="eliFreeBadge". A second "今なら無料" span without it is no free badge.
func TestComiciIsFree(t *testing.T) {
	const listing = `<div data-e2e="eli"><span class="series-eplist-item-access-text mode-free" data-e2e="eliFreeBadge">無料</span></div>
<div data-e2e="eli"><span class="series-eplist-item-access-text mode-free">今なら無料</span></div>
<div data-e2e="eli"><span class="series-eplist-item-access-icon"></span></div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(listing))
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, false}
	doc.Find(`[data-e2e="eli"]`).Each(func(i int, item *goquery.Selection) {
		if got := comiciIsFree(item); got != want[i] {
			t.Errorf("item %d: comiciIsFree = %v, want %v", i, got, want[i])
		}
	})
}

func TestComiciSeries(t *testing.T) {
	tests := map[string]string{
		"機動絶記ガンダムSEQUEL - 井上敏樹,千明太郎 | HERO&#x27;S Web": "機動絶記ガンダムSEQUEL",
		"転生競走馬 H-MAN エッチマン - 城谷間間,akinaka | ライコミ":      "転生競走馬 H-MAN エッチマン",
		"最強異端の植物使い - かん奈,福寿草真 | コミックグロウル":              "最強異端の植物使い",
	}
	for raw, want := range tests {
		if got := comiciSeries(raw); got != want {
			t.Errorf("comiciSeries(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestComiciPermutation(t *testing.T) {
	if _, err := comiciPermutation("[1, 2, 3]"); err == nil {
		t.Error("expected a short scramble to be refused")
	}
	perm, err := comiciPermutation("[12, 0, 3, 2, 7, 5, 11, 4, 14, 10, 8, 13, 9, 1, 6, 15]")
	if err != nil || len(perm) != 16 || perm[0] != 12 {
		t.Errorf("comiciPermutation = %v, %v", perm, err)
	}
}

// TestComiciUnscramble swaps two tiles of a synthetic image: tile 0 (top-left)
// must come from tile 4 (column 1, row 0), so the colours trade places
func TestComiciUnscramble(t *testing.T) {
	const size = 64
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	tile := func(col, row int) color.RGBA {
		return color.RGBA{R: uint8(col * 60), G: uint8(row * 60), B: 200, A: 255}
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			src.Set(x, y, tile(x/16, y/16))
		}
	}
	perm := []int{4, 1, 2, 3, 0, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, src); err != nil {
		t.Fatal(err)
	}

	out, err := comiciUnscramble(perm)(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	// tile 0 (top-left) now shows the source's tile 4 (column 1, row 0)
	r, g, _, _ := got.At(4, 4).RGBA()
	want := tile(1, 0)
	if uint8(r>>8) != want.R || uint8(g>>8) != want.G {
		t.Errorf("tile 0 colour = (%d,%d), want (%d,%d)", r>>8, g>>8, want.R, want.G)
	}
}
