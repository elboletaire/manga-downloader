// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"
)

func TestPornhwareaderResolveSeriesURL(t *testing.T) {
	tests := map[string]string{
		"https://pornhwareader.com/series/345/perfect-half":                 "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/chapter-150":     "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half/chapter-148.1":   "https://pornhwareader.com/series/345/perfect-half",
		"https://pornhwareader.com/series/345/perfect-half?page=2#chapters": "https://pornhwareader.com/series/345/perfect-half?page=2#chapters",
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
