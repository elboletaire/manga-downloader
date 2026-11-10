// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestPornhwareaderResolveSeriesURL(t *testing.T) {
	tests := map[string]string{
		"https://pornhwareader.com/series/345/perfect-half":                                      "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/chapter-150":                          "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/chapter-148.1":                        "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/4f07da7e-f882-4775-9ab2-732135471747": "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/chapter-150/extra/segment":            "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half?page=2#chapters":                      "https://pornhwareader.com/series/345/perfect-half?page=2#chapters",
	}

	for in, want := range tests {
		p := &Pornhwareader{Grabber: &Grabber{URL: in}}
		p.resolveSeriesURL()
		if p.URL != want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", in, p.URL, want)
		}
	}
}

func TestPornhwareaderSeriesURL(t *testing.T) {
	p := &Pornhwareader{Grabber: &Grabber{URL: "https://pornhwareader.com/series/345/perfect-half/chapter-150"}}
	p.resolveSeriesURL()

	if got := p.seriesURL(1); got != "https://pornhwareader.com/series/345/perfect-half" {
		t.Errorf("seriesURL(1) = %q", got)
	}
	if got := p.seriesURL(3); got != "https://pornhwareader.com/series/345/perfect-half?page=3" {
		t.Errorf("seriesURL(3) = %q", got)
	}
}

func TestPornhwareaderChapterRe(t *testing.T) {
	tests := map[string]string{
		"/series/345/perfect-half/chapter-217":   "217",
		"/series/345/perfect-half/chapter-148.1": "148.1",
	}
	for in, want := range tests {
		m := pornhwareaderChapterRe.FindStringSubmatch(in)
		if m == nil || m[2] != want {
			t.Errorf("pornhwareaderChapterRe(%q) = %v, want number %q", in, m, want)
		}
	}
	for _, in := range []string{"/series/345/perfect-half", "/series/345/perfect-half#chapters", "/series/345/perfect-half/chapter-1/extra"} {
		if pornhwareaderChapterRe.MatchString(in) {
			t.Errorf("pornhwareaderChapterRe(%q) matched, want no match", in)
		}
	}
}

func TestPornhwareaderPages(t *testing.T) {
	// the chapter list's "pages":19 counts come before the chapter's own
	// "pages" array in some payloads, so the array must be anchored on
	html := `<script>self.__next_f.push([1,"5:[{\"number\":1,\"pages\":19},{\"number\":2,\"pages\":12}]\n"])</script>` +
		`<script>self.__next_f.push([1,"6:{\"chapter\":{\"pages\":[{\"ordinal\":2,\"url\":\"https://media.example/2.webp\",\"width\":1440,\"height\":10000},{\"ordinal\":1,\"url\":\"https://media.example/1.webp\",\"width\":1080,\"height\":2700}]}}\n"])</script>`

	pages, err := pornhwareaderPages(html)
	if err != nil {
		t.Fatalf("pornhwareaderPages() error = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pornhwareaderPages() got %d pages, want 2", len(pages))
	}
	if pages[0].URL != "https://media.example/1.webp" || pages[1].URL != "https://media.example/2.webp" {
		t.Errorf("pornhwareaderPages() = %+v, want pages sorted by ordinal", pages)
	}

	if _, err := pornhwareaderPages("<html>no payload</html>"); err == nil {
		t.Error("pornhwareaderPages() without a payload should error")
	}
}

func TestPornhwareaderListedChapters(t *testing.T) {
	// the "Start reading" button links a UUID chapter too, but it's outside
	// the chapter rows, so it must not add a chapter
	html := `<div class="actions"><a class="button primary resume" href="/series/102/like-father-like-son/4f07da7e-f882-4775-9ab2-732135471747">Start reading</a></div>` +
		`<ol><li data-number="8"><div class="chapter-row"><a href="/series/345/perfect-half/chapter-8"><span class="chapter-title">Chapter 8</span><time>Dec 19, 2024</time></a></div></li>` +
		`<li><div class="chapter-row"><a href="/series/345/perfect-half/4f07da7e-f882-4775-9ab2-732135471747"><span class="chapter-title">Chapter 8 - Special Forces</span><time>Dec 19, 2024</time></a></div></li>` +
		`<li><div class="chapter-row"><a href="/series/345/perfect-half/chapter-148.1"><span class="chapter-title">Chapter 148.1</span></a></div></li>` +
		`<li><div class="chapter-row"><a href="/series/345/perfect-half/chapter-8"><span class="chapter-title">Chapter 8</span></a></div></li>` +
		`<li><div class="chapter-row"><a href="/series/999/other-series/chapter-1"><span class="chapter-title">Other</span></a></div></li></ol>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	p := &Pornhwareader{Grabber: &Grabber{URL: "https://pornhwareader.com/series/345/perfect-half"}}
	listed := p.listedChapters(doc, map[string]bool{})
	if len(listed) != 3 {
		t.Fatalf("listedChapters() got %d chapters, want 3 (numbered 8 and 148.1, and the UUID one)", len(listed))
	}

	numbered := listed[0].(*PornhwareaderChapter)
	if numbered.Number != 8 || numbered.Label.Number != "8" || numbered.URL != "https://pornhwareader.com/series/345/perfect-half/chapter-8" {
		t.Errorf("listedChapters()[0] = %+v, unexpected", numbered)
	}
	uuid := listed[1].(*PornhwareaderChapter)
	if !Unnumbered(uuid) || uuid.Title != "Chapter 8 - Special Forces" {
		t.Errorf("listedChapters()[1] = %+v, want an unnumbered chapter titled from its first line", uuid)
	}

	if fraction := listed[2].(*PornhwareaderChapter); fraction.Label.Number != "148.1" || fraction.Number != 148.1 {
		t.Errorf("listedChapters()[2] = %+v, unexpected", fraction)
	}

	// a page that repeats known URLs adds nothing, which ends the pagination
	if again := p.listedChapters(doc, map[string]bool{
		"https://pornhwareader.com/series/345/perfect-half/chapter-8":                            true,
		"https://pornhwareader.com/series/345/perfect-half/chapter-148.1":                        true,
		"https://pornhwareader.com/series/345/perfect-half/4f07da7e-f882-4775-9ab2-732135471747": true,
	}); len(again) != 0 {
		t.Errorf("listedChapters() with known URLs got %d chapters, want none", len(again))
	}
}
