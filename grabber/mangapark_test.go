// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestMangaparkLang(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "EN"},
		{"en", "EN"},
		{"EN", "EN"},
		{"ar", "AR"},
		{"pt-br", "PT-BR"},
	}
	for _, c := range cases {
		if got := mangaparkLang(c.in); got != c.want {
			t.Errorf("mangaparkLang(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMangaparkChapterTitle(t *testing.T) {
	cases := []struct {
		title  string
		number float64
		want   string
	}{
		// Prefix with subtitle: strip "Chapter N: "
		{"Chapter 267: Finale: Goodbye, Shonan", 267, "Finale: Goodbye, Shonan"},
		// Decimal number with subtitle
		{"Chapter 267.5: Extra", 267.5, "Extra"},
		// Number only: labelled "Chapter N"
		{"Chapter 267", 267, "Chapter 267"},
		{"Chapter 267.5", 267.5, "Chapter 267.5"},
		// No "Chapter N" prefix: keep as-is
		{"One-Shot", 1, "One-Shot"},
		// Bare "Chapter" (no number): keep as-is (one-shot without subtitle)
		{"Chapter", 1, "Chapter"},
	}
	// want is the label, as the title used to read before labels were composed
	for _, c := range cases {
		ch := Chapter{Title: mangaparkChapterTitle(c.title), Number: c.number, Label: Label{Style: LabelNumbered}}
		if got := ChapterLabel(ch); got != c.want {
			t.Errorf("label of %q (%v) = %q, want %q", c.title, c.number, got, c.want)
		}
	}
}

func TestMangaparkSeriesURL(t *testing.T) {
	const ccSeries = "https://mangapark.cc/manga/kokoro-ni-fuku-arashi-o-kanadete"
	const oldSeries = "https://mangapark.cc/manga/rowdy-reunion"

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"series page on .cc", "https://mangapark.cc/manga/kokoro-ni-fuku-arashi-o-kanadete", ccSeries, false},
		{"series page with query", "https://mangapark.cc/manga/kokoro-ni-fuku-arashi-o-kanadete?tab=chapters#top", ccSeries, false},
		{"reader page resolves to series", "https://mangapark.cc/read/kokoro-ni-fuku-arashi-o-kanadete/en/chapter-1", ccSeries, false},
		{"old host .page rewritten", "https://mangapark.page/series/rowdy-reunion", oldSeries, false},
		{"old host bare slug rewritten", "https://mangapark.page/rowdy-reunion", oldSeries, false},
		{"old host .to rewritten", "https://mangapark.to/manga/rowdy-reunion", oldSeries, false},
		{"old host 1.com rewritten", "https://mangapark1.com/manga/rowdy-reunion", oldSeries, false},
		{"bare path on .cc fails", "https://mangapark.cc/home", "", true},
		{"no slug fails", "https://mangapark.cc/", "", true},
	}

	for _, c := range cases {
		got, err := mangaparkSeriesURL(c.input)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%s: url = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMangaparkTest(t *testing.T) {
	cases := map[string]bool{
		"https://mangapark.cc/manga/kokoro-ni-fuku-arashi-o-kanadete":             true,
		"https://mangapark.cc/read/kokoro-ni-fuku-arashi-o-kanadete/en/chapter-1": true,
		"https://mangapark.page/series/rowdy-reunion":                             true,
		"https://mangapark.to/series/rowdy-reunion":                               true,
		"https://mangapark1.com/manga/rowdy-reunion":                              true,
		"https://example.com/manga/rowdy-reunion":                                 false,
	}

	for url, want := range cases {
		m := &Mangapark{Grabber: &Grabber{URL: url}}
		got, err := m.Test()
		if err != nil || got != want {
			t.Errorf("%s: Test() = %v, %v, want %v", url, got, err, want)
		}
	}
}

func TestParseMangaparkChaptersBatch(t *testing.T) {
	// HTML fixture: real structure with flag icons, subtitles, decimal numbers,
	// and an unnumbered (one-shot) entry.
	html := `<span class="chapter-batch-meta" hidden
    data-has-more="true"
    data-next-offset="200"
    data-total="268"></span>
<li class="item" data-number="268">
    <a href="/read/shonan-junai-gumi/en/chapter-268" title="Chapter 268">
        <span><i class="flag EN"></i> Chapter 268: The End</span>
        <span>Jan 1, 2018</span>
    </a>
</li>
<li class="item" data-number="267.5">
    <a href="/read/shonan-junai-gumi/en/chapter-267.5" title="Chapter 267.5">
        <span><i class="flag EN"></i> Chapter 267.5</span>
        <span>Jan 1, 2018</span>
    </a>
</li>
<li class="item" data-number="">
    <a href="/read/oneshot/en/chapter-0" title="One-Shot">
        <span><i class="flag EN"></i> One-Shot</span>
        <span>Jan 1, 2018</span>
    </a>
</li>`

	chapters, nextOffset, hasMore, err := parseMangaparkChaptersBatch(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(chapters))
	}
	if id := chapters[0].(*MangaparkChapter).ID; id != "/read/shonan-junai-gumi/en/chapter-268" {
		t.Errorf("chapters[0] ID = %q, want its reader path", id)
	}
	if !hasMore {
		t.Error("hasMore should be true")
	}
	if nextOffset != 200 {
		t.Errorf("nextOffset = %d, want 200", nextOffset)
	}

	tests := []struct {
		idx        int
		wantNumber float64
		wantTitle  string
		wantLink   string
		wantLang   string
	}{
		// "Chapter 268: The End" → prefix stripped → "The End"
		{0, 268, "The End", "/read/shonan-junai-gumi/en/chapter-268", "EN"},
		// "Chapter 267.5" → no subtitle → fallback "Chapter 267.5"
		{1, 267.5, "Chapter 267.5", "/read/shonan-junai-gumi/en/chapter-267.5", "EN"},
		// One-shot (data-number=""): seqNum after 267.5+1=268.5; title kept as-is
		{2, 268.5, "One-Shot", "/read/oneshot/en/chapter-0", "EN"},
	}

	for _, test := range tests {
		c := chapters[test.idx].(*MangaparkChapter)
		if c.Number != test.wantNumber {
			t.Errorf("chapter %d: number = %v, want %v", test.idx, c.Number, test.wantNumber)
		}
		if ChapterLabel(c) != test.wantTitle {
			t.Errorf("chapter %d: title = %q, want %q", test.idx, ChapterLabel(c), test.wantTitle)
		}
		if c.Link != test.wantLink {
			t.Errorf("chapter %d: link = %q, want %q", test.idx, c.Link, test.wantLink)
		}
		if c.Language != test.wantLang {
			t.Errorf("chapter %d: language = %q, want %q", test.idx, c.Language, test.wantLang)
		}
	}
}

func TestParseMangaparkLanguages(t *testing.T) {
	// HTML fixture: series page language tabs
	html := `<html><body>
<a class="dropdown-item active" href="#" data-code="EN" data-flag="EN">English</a>
<a class="dropdown-item " href="#" data-code="IT" data-flag="IT">Italian</a>
<a class="dropdown-item " href="#" data-code="PT-BR" data-flag="PT-BR">Portuguese (Br)</a>
</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	var langs []string
	doc.Find("a.dropdown-item[data-code]").Each(func(_ int, s *goquery.Selection) {
		if code := s.AttrOr("data-code", ""); code != "" {
			langs = append(langs, code)
		}
	})

	want := []string{"EN", "IT", "PT-BR"}
	if len(langs) != len(want) {
		t.Fatalf("got %d languages, want %d: %v", len(langs), len(want), langs)
	}
	for i, w := range want {
		if langs[i] != w {
			t.Errorf("lang[%d] = %q, want %q", i, langs[i], w)
		}
	}
}

func TestParseMangaparkReaderImages(t *testing.T) {
	html := `<section class="chapter-reader">
<img src="https://c.imgeu2.lol/chapter/abc/1-def.jpg" alt="Page 1">
<img src="https://c.imgeu2.lol/chapter/abc/2-ghi.png" alt="Page 2">
<img src="not-a-url" alt="Invalid">
</section>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	var pages []string
	doc.Find("section.chapter-reader img").Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if !strings.HasPrefix(src, "http") {
			return
		}
		pages = append(pages, src)
	})

	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0] != "https://c.imgeu2.lol/chapter/abc/1-def.jpg" {
		t.Errorf("page 0 = %q", pages[0])
	}
}
