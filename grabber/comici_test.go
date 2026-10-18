// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
		numbered         bool
	}{
		{"第1話　血まみれのカオリス", "血まみれのカオリス", "", 1, true},
		{"第34話①", "", "1", 34, true},
		{"第36話前編①", "", "前編1", 36, true},
		{"第36話前編", "", "前編", 36, true},
		{"第35話　黒の衝撃", "黒の衝撃", "", 35, true},
		{"0話", "", "", 0, true},
		{"第0話 はじめに", "はじめに", "", 0, true},
		{"1", "", "", 1, true},
		{"2-①", "", "1", 2, true},
		{"19-②", "", "2", 19, true},
		{"#9.5", "", "", 9.5, true},
		{"#1", "", "", 1, true},
		{"＃1 お楽しみ", "お楽しみ", "", 1, true},
		{"第七十一話 ぬい", "ぬい", "", 71, true},
		{"番外編", "番外編", "", 0, false},
		{"#特別版", "#特別版", "", 0, false},
		{"第４集 単行本告知", "第４集 単行本告知", "", 0, false},
		// a trailing part: 前編/後編, or a circled number after the title
		{"第一服「魔女と再会」", "「魔女と再会」", "", 1, true},
		{"第二十一服「魔女と背信」前編", "「魔女と背信」", "前編", 21, true},
		{"EPISODE.01　復活", "復活", "", 1, true},
		{"第2話 特定熊事象(2)前編", "特定熊事象(2)", "前編", 2, true},
		{"第2話 特定熊事象(2)後編", "特定熊事象(2)", "後編", 2, true},
		{"第2話　したい事(働いてみたい)①", "したい事(働いてみたい)", "1", 2, true},
		{"第4話 したい事(セックスがしたい①)③", "したい事(セックスがしたい①)", "3", 4, true},
		{"第17話　2024年　由美子　17歳③", "2024年　由美子　17歳", "3", 17, true},
		// not parts: a bracketed number, a numbered subtitle, a marker alone
		{"第2話 巨獣禍の後に(1)", "巨獣禍の後に(1)", "", 2, true},
		{"第62話 石川と田中とフジイ(2)", "石川と田中とフジイ(2)", "", 62, true},
		{"第2話 文正の政変 その1", "文正の政変 その1", "", 2, true},
		{"第3話 前編", "前編", "", 3, true},
		{"特別イラスト②", "特別イラスト②", "", 0, false},
	}
	for _, tt := range tests {
		title, number, part, numbered := comiciTitle(tt.raw, true)
		if title != tt.title || number != tt.number || part != tt.part || numbered != tt.numbered {
			t.Errorf("comiciTitle(%q) = (%q, %v, %q, %v), want (%q, %v, %q, %v)",
				tt.raw, title, number, part, numbered, tt.title, tt.number, tt.part, tt.numbered)
		}
	}
}

// TestComiciTitleLoneTrailingPart keeps a trailing marker in the title when no
// other episode shares the number: "25話 サマーキャンプ①" is an arc, not a part
func TestComiciTitleLoneTrailingPart(t *testing.T) {
	for _, tt := range []struct {
		raw, title, part string
		number           float64
	}{
		{"25話 サマーキャンプ①", "サマーキャンプ①", "", 25},
		{"第4話 したい事(セックスがしたい①)③", "したい事(セックスがしたい①)③", "", 4},
		{"第2話 特定熊事象(2)前編", "特定熊事象(2)前編", "", 2},
	} {
		title, number, part, _ := comiciTitle(tt.raw, false)
		if title != tt.title || number != tt.number || part != tt.part {
			t.Errorf("comiciTitle(%q, false) = (%q, %v, %q), want (%q, %v, %q)",
				tt.raw, title, number, part, tt.title, tt.number, tt.part)
		}
	}
}

// TestComiciFetchChaptersTrailingParts splits a trailing part only where the
// listing shares the number
func TestComiciFetchChaptersTrailingParts(t *testing.T) {
	const listing = `<title>テスト - 作者 | サイト</title>
<div data-e2e="eli"><a href="/episodes/aaaaaaaaaaaaa"></a><span data-e2e="eliTitle">25話 サマーキャンプ①</span></div>
<div data-e2e="eli"><a href="/episodes/bbbbbbbbbbbbb"></a><span data-e2e="eliTitle">26話 サマーキャンプ②</span></div>
<div data-e2e="eli"><a href="/episodes/ccccccccccccc"></a><span data-e2e="eliTitle">2話 特定熊事象(2)後編</span></div>
<div data-e2e="eli"><a href="/episodes/ddddddddddddd"></a><span data-e2e="eliTitle">2話 特定熊事象(2)前編</span></div>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/series/eeeeeeeeeeeee/1" {
			fmt.Fprint(w, listing)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewComici(&Grabber{URL: srv.URL + "/series/eeeeeeeeeeeee", Settings: &Settings{}})
	chapters, errs := c.FetchChapters()
	if len(errs) != 0 || len(chapters) != 4 {
		t.Fatalf("FetchChapters() = %d chapters, errs %v; want 4", len(chapters), errs)
	}
	// listed as the site lists them: the 後編 first
	want := []string{"サマーキャンプ①", "サマーキャンプ②", "後編 特定熊事象(2)", "前編 特定熊事象(2)"}
	for i, w := range want {
		if got := ChapterLabel(chapters[i]); got != w {
			t.Errorf("chapter %d: label %q, want %q", i, got, w)
		}
	}
}

// TestComiciPartsTitle checks that the 前編 and 後編 of one episode merge under
// its title, as the merged file is named
func TestComiciPartsTitle(t *testing.T) {
	var parts Filterables
	for _, raw := range []string{"第2話 特定熊事象(2)後編", "第2話 特定熊事象(2)前編"} {
		title, number, part, _ := comiciTitle(raw, true)
		parts = append(parts, &ComiciChapter{Chapter{Title: title, Number: number, Label: Label{Style: LabelNumbered, Part: part}}})
	}
	if got := PartsTitle(parts); got != "特定熊事象(2)" {
		t.Errorf("PartsTitle = %q, want %q", got, "特定熊事象(2)")
	}
}

// TestComiciFetchChaptersUnnumbered lists episodes with and without a number:
// the unnumbered ones are LabelUnnumbered, and 第0話 stays a numbered chapter
func TestComiciFetchChaptersUnnumbered(t *testing.T) {
	const listing = `<title>テスト - 作者 | サイト</title>
<div data-e2e="eli"><a href="/episodes/aaaaaaaaaaaaa"></a><span data-e2e="eliTitle">第0話 はじめに</span></div>
<div data-e2e="eli"><a href="/episodes/bbbbbbbbbbbbb"></a><span data-e2e="eliTitle">特別イラスト②</span></div>
<div data-e2e="eli"><a href="/episodes/ccccccccccccc"></a><span data-e2e="eliTitle">第４集 単行本告知</span></div>
<div data-e2e="eli"><a href="/episodes/ddddddddddddd"></a><span data-e2e="eliTitle">第七十一話 ぬい</span></div>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/series/eeeeeeeeeeeee/1" {
			fmt.Fprint(w, listing)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewComici(&Grabber{URL: srv.URL + "/series/eeeeeeeeeeeee", Settings: &Settings{}})
	chapters, errs := c.FetchChapters()
	if len(errs) != 0 || len(chapters) != 4 {
		t.Fatalf("FetchChapters() = %d chapters, errs %v; want 4", len(chapters), errs)
	}
	want := []struct {
		number float64
		style  LabelStyle
	}{{0, LabelNumbered}, {0, LabelUnnumbered}, {0, LabelUnnumbered}, {71, LabelNumbered}}
	for i, w := range want {
		if got := labelOf(chapters[i]).Style; got != w.style || chapters[i].GetNumber() != w.number {
			t.Errorf("chapter %d: number %v style %v, want %v %v", i, chapters[i].GetNumber(), got, w.number, w.style)
		}
	}
	if !Unnumbered(chapters[1]) || Unnumbered(chapters[0]) {
		t.Error("Unnumbered tells the numbered 第0話 from the unnumbered 特別イラスト②")
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

// comiciMarkupPage is a series page with the vendor link and an episode item
const comiciMarkupPage = `<html><body><a href="https://comici.co.jp/terms">terms</a><ul><li data-e2e="eli"><a href="/episodes/c1ffef58c01d8">1</a></li></ul></body></html>`

// TestComiciTestByMarkup checks that a host outside the list is matched by its
// page's markup, with one fetch, and that a page with only the vendor link (a
// footer) or another URL shape is refused
func TestComiciTestByMarkup(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/plain/"):
			w.Write([]byte(`<html><body><a href="/">no platform here</a></body></html>`))
		case strings.HasPrefix(r.URL.Path, "/footer/"):
			w.Write([]byte(`<html><body><a href="https://comici.co.jp/terms">terms</a></body></html>`))
		default:
			w.Write([]byte(comiciMarkupPage))
		}
	}))
	defer srv.Close()

	check := func(path string, want bool, wantHits int32) {
		t.Helper()
		before := hits.Load()
		got, err := NewComiciByMarkup(&Grabber{URL: srv.URL + path, Settings: &Settings{}}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v; want %v, nil", path, got, err, want)
		}
		if n := hits.Load() - before; n != wantHits {
			t.Errorf("Test(%q) made %d requests, want %d", path, n, wantHits)
		}
	}

	check("/series/842677db37980/1", true, 1)
	check("/episodes/c1ffef58c01d8", true, 1)
	check("/plain/series/842677db37980/1", false, 1)
	check("/footer/series/842677db37980/1", false, 1)
	check("/comics/842677db37980", false, 0)

	// the listed-host instance never fetches an unlisted host
	before := hits.Load()
	if got, _ := NewComici(&Grabber{URL: srv.URL + "/episodes/c1ffef58c01d8", Settings: &Settings{}}).Test(); got || hits.Load() != before {
		t.Errorf("listed-host Test on an unlisted host = %v with %d requests; want false and none", got, hits.Load()-before)
	}
}

// TestComiciTestSharesPageCache checks that the markup check reads the page
// PlainHTML already fetched in the same identification, so it makes no request
// of its own
func TestComiciTestSharesPageCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(comiciMarkupPage))
	}))
	defer srv.Close()

	g := &Grabber{URL: srv.URL + "/series/842677db37980/1", Settings: &Settings{}}
	g.identifyPages = newPageCache()
	plain := NewPlainHTML(g)
	plain.Test()
	before := hits.Load()

	got, err := NewComiciByMarkup(g).Test()
	if err != nil || !got {
		t.Errorf("Test = %v, %v; want true, nil", got, err)
	}
	if n := hits.Load() - before; n != 0 {
		t.Errorf("markup check made %d requests after PlainHTML fetched the page, want 0", n)
	}
}
