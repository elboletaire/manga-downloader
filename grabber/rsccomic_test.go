// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestParseRSCComicSeriesPage(t *testing.T) {
	// a trimmed-down series page payload: Next.js streams the RSC data as
	// several `self.__next_f.push([1,"..."])` chunks and splits it wherever
	// it likes, so the "chapters" key below straddles two of them on purpose
	html := `<script>self.__next_f.push([1,"3:[\"$\",\"$L6c\",null,{\"series\":{\"id\":\"abc\",\"title\":\"Some Series\",\"slug\":\"some-series\",\"chapterCount\":2},\"chap"])</script>` +
		`<script>self.__next_f.push([1,"ters\":[{\"id\":\"c1\",\"number\":1,\"title\":null},{\"id\":\"c2\",\"number\":2.5,\"title\":\"Named one\"}],\"currentPage\":1,\"totalPages\":2}]\n"])</script>`

	page, err := parseRSCComicSeriesPage(html)
	if err != nil {
		t.Fatalf("parseRSCComicSeriesPage() error = %v", err)
	}

	if page.Series.Title != "Some Series" || page.Series.Slug != "some-series" {
		t.Errorf("parseRSCComicSeriesPage() series = %+v, unexpected", page.Series)
	}
	if page.TotalPages != 2 {
		t.Errorf("parseRSCComicSeriesPage() totalPages = %d, want 2", page.TotalPages)
	}
	if len(page.Chapters) != 2 {
		t.Fatalf("parseRSCComicSeriesPage() got %d chapters, want 2", len(page.Chapters))
	}
	// most chapters carry a null title, which is what makes FetchChapters
	// fall back to "Chapter N"
	if page.Chapters[0].Number != 1 || page.Chapters[0].Title != "" {
		t.Errorf("parseRSCComicSeriesPage() chapters[0] = %+v, unexpected", page.Chapters[0])
	}
	if page.Chapters[1].Number != 2.5 || page.Chapters[1].Title != "Named one" {
		t.Errorf("parseRSCComicSeriesPage() chapters[1] = %+v, unexpected", page.Chapters[1])
	}

	if _, err := parseRSCComicSeriesPage("no payload here"); err == nil {
		t.Error("parseRSCComicSeriesPage() with no __next_f payload should error")
	}
}

// a series short enough to fit in a single chapter list page ships no
// totalPages at all, and must not be treated as having zero pages
func TestParseRSCComicSeriesPageDefaultsToOnePage(t *testing.T) {
	html := `<script>self.__next_f.push([1,"3:{\"series\":{\"title\":\"T\",\"slug\":\"t\"},\"chapters\":[{\"number\":1,\"title\":null}]}"])</script>`

	page, err := parseRSCComicSeriesPage(html)
	if err != nil {
		t.Fatalf("parseRSCComicSeriesPage() error = %v", err)
	}
	if page.TotalPages != 1 {
		t.Errorf("parseRSCComicSeriesPage() totalPages = %d, want 1", page.TotalPages)
	}
}

func TestRSCComicChapterPages(t *testing.T) {
	// reader payload: page URLs are relative and signed with a ?sig=&exp=
	// query string that must be kept verbatim for the download to work
	html := `<script>self.__next_f.push([1,"8:{\"number\":60,\"title\":null,\"content\":\"\",\"pages\":[{\"id\":\"p1\",\"pageNumber\":1,\"kind\":\"CONTENT\",\"imageUrl\":\"/uploads/comic-pages/some-series/60/page-001.webp?sig=abc&exp=123\",\"width\":800},{\"id\":\"p2\",\"pageNumber\":2,\"kind\":\"CONTENT\",\"imageUrl\":\"/uploads/comic-pages/some-series/60/page-002.webp?sig=def&exp=123\",\"width\":800}]}"])</script>`

	pages, err := rscComicChapterPages(html)
	if err != nil {
		t.Fatalf("rscComicChapterPages() error = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("rscComicChapterPages() got %d pages, want 2", len(pages))
	}
	if pages[0].ImageURL != "/uploads/comic-pages/some-series/60/page-001.webp?sig=abc&exp=123" {
		t.Errorf("rscComicChapterPages()[0] = %q, unexpected", pages[0].ImageURL)
	}
	if pages[1].ImageURL != "/uploads/comic-pages/some-series/60/page-002.webp?sig=def&exp=123" {
		t.Errorf("rscComicChapterPages()[1] = %q, unexpected", pages[1].ImageURL)
	}

	if _, err := rscComicChapterPages(`<script>self.__next_f.push([1,"8:{\"number\":60}"])</script>`); err == nil {
		t.Error("rscComicChapterPages() with no pages array should error")
	}
}

// totalPages is what drives pagination, but the site also reports the
// series' full chapterCount independently; if the two disagree (a page
// failed to load, or totalPages undercounts) FetchChapters must error
// instead of silently returning a truncated-but-successful list — the same
// failure class tanstackComic guards against for hivetoons/vortexscans
func TestRSCComicFetchChaptersGuardsIncompleteCount(t *testing.T) {
	page := &rscComicSeriesPage{TotalPages: 1}
	page.Series.Title = "T"
	page.Series.Slug = "t"
	page.Series.ChapterCount = 5
	if err := json.Unmarshal([]byte(`[{"number":1},{"number":2}]`), &page.Chapters); err != nil {
		t.Fatalf("unmarshal chapters: %v", err)
	}

	r := &RSCComic{
		Grabber: &Grabber{URL: "https://kaynscans.com/series/comic/t"},
		pages:   map[int]*rscComicSeriesPage{1: page},
	}

	chapters, errs := r.FetchChapters()
	if len(errs) == 0 {
		t.Fatalf("FetchChapters() returned %d chapters and no error, want an error", len(chapters))
	}
}

func TestRSCComicResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation/chapter/198",
			wanted: "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation",
		},
		{
			name:   "chapter URL with query string",
			input:  "https://witchtoons.net/series/comic/i-am-a-cultivation-big-shot/chapter/596?page=1",
			wanted: "https://witchtoons.net/series/comic/i-am-a-cultivation-big-shot",
		},
		{
			name:   "series URL unchanged",
			input:  "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation",
			wanted: "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation",
		},
		{
			name:   "series URL with trailing slash unchanged",
			input:  "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation/",
			wanted: "https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation/",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://kaynscans.com/",
			wanted: "https://kaynscans.com/",
		},
	}

	for _, c := range cases {
		r := &RSCComic{Grabber: &Grabber{URL: c.input}}
		r.resolveSeriesURL()
		if r.URL != c.wanted {
			t.Errorf("%s: got %q, want %q", c.name, r.URL, c.wanted)
		}
	}
}

func TestParseRSCComicSeriesPageLocked(t *testing.T) {
	html := `<script>self.__next_f.push([1,"3:[\"$\",\"$L6c\",null,{\"series\":{\"id\":\"abc\",\"title\":\"S\",\"slug\":\"s\",\"chapterCount\":2},\"chapters\":[` +
		`{\"id\":\"c146\",\"number\":146,\"title\":\"146\",\"isLocked\":true,\"coinPrice\":50,\"hasAccess\":false},` +
		`{\"id\":\"c145\",\"number\":145,\"title\":\"145\",\"isLocked\":false,\"coinPrice\":0,\"earlyAccessUntil\":\"2020-01-01T00:00:00.000Z\"}],\"currentPage\":1,\"totalPages\":1}]\n"])</script>`

	page, err := parseRSCComicSeriesPage(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Chapters) != 2 || !page.Chapters[0].IsLocked || page.Chapters[1].IsLocked {
		t.Errorf("chapters = %+v, want [locked free]", page.Chapters)
	}
}

func TestFormatChapterNumber(t *testing.T) {
	cases := []struct {
		number float64
		want   string
	}{
		{61, "61"},
		{61.5, "61.5"},
		{61.25, "61.25"},
		{0, "0"},
	}

	for _, c := range cases {
		if got := formatChapterNumber(c.number); got != c.want {
			t.Errorf("formatChapterNumber(%v) = %q, want %q", c.number, got, c.want)
		}
	}
}

// TestRSCComicTest covers the platform's hosts and every old host Test()
// rewrites, with the URL the grabber is left with
func TestRSCComicTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string // rewritten URL after Test(); empty means unchanged
	}{
		{"https://witchtoons.net/series/comic/some-series", true, ""},
		{"https://witchtoons.net/series/comic/some-series/chapter/12", true, ""},
		// the old domain 301s to witchtoons.net dropping the path, so its
		// URLs can't be mapped to the new /series/comic/{slug} shape
		{"https://witchscans.com/manga/some-series/", false, ""},
		{"https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation", true, ""},
		{"https://kaynscans.com/series/comic/colorist/chapter/45", true, ""},
		// kaynscan.org 301s to kaynscans.com remapping /series/{slug} to
		// /series/comic/{slug}; Test() applies the same rewrite in code
		{
			"https://kaynscan.org/series/heavenly-demon-cultivation-simulation",
			true,
			"https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation",
		},
		// an old URL already carrying the new path shape isn't doubled up
		{
			"http://kaynscan.org/series/comic/colorist",
			true,
			"https://kaynscans.com/series/comic/colorist",
		},
		{"https://kaynscan.org/", true, "https://kaynscans.com/"},
		{"https://drakecomic.net/series/comic/beast-evolution", true, ""},
		{"https://drakecomic.net/series/comic/beast-evolution/chapter/70", true, ""},
		// drakecomic.org 301s /manga/{slug}/ to drakecomic.net/series/comic/{slug}/
		{
			"https://drakecomic.org/manga/beast-evolution/",
			true,
			"https://drakecomic.net/series/comic/beast-evolution/",
		},
		{
			"http://drakecomic.org/manga/beast-evolution",
			true,
			"https://drakecomic.net/series/comic/beast-evolution",
		},
		{"https://drakecomic.org/", true, "https://drakecomic.net/"},
		{"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow", true, ""},
		{"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow/chapter/123", true, ""},
		// elftoon.com 301s /manga/{slug}/ on to elftoon.net/series/comic/{slug}
		{
			"https://elftoon.com/manga/god-level-assassin-im-the-shadow/",
			true,
			"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow/",
		},
		{
			"http://elftoon.com/manga/god-level-assassin-im-the-shadow",
			true,
			"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow",
		},
		{"https://elftoon.com/", true, "https://elftoon.net/"},
		{"https://example.com/series/comic/whatever", false, ""},
	}

	for _, c := range cases {
		r := NewRSCComic(&Grabber{URL: c.url})
		got, err := r.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
			continue
		}
		wantURL := c.wantURL
		if wantURL == "" {
			wantURL = c.url
		}
		if r.URL != wantURL {
			t.Errorf("Test(%q) rewrote URL to %q, want %q", c.url, r.URL, wantURL)
		}
	}
}
