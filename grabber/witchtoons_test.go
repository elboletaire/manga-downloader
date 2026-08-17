// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestWitchtoonsSeriesPage(t *testing.T) {
	// a trimmed-down series page payload: Next.js streams the RSC data as
	// several `self.__next_f.push([1,"..."])` chunks and splits it wherever
	// it likes, so the "chapters" key below straddles two of them on purpose
	html := `<script>self.__next_f.push([1,"3:[\"$\",\"$L6c\",null,{\"series\":{\"id\":\"abc\",\"title\":\"Some Series\",\"slug\":\"some-series\",\"chapterCount\":2},\"chap"])</script>` +
		`<script>self.__next_f.push([1,"ters\":[{\"id\":\"c1\",\"number\":1,\"title\":null},{\"id\":\"c2\",\"number\":2.5,\"title\":\"Named one\"}],\"currentPage\":1,\"totalPages\":2}]\n"])</script>`

	page, err := parseWitchtoonsSeriesPage(html)
	if err != nil {
		t.Fatalf("parseWitchtoonsSeriesPage() error = %v", err)
	}

	if page.Series.Title != "Some Series" || page.Series.Slug != "some-series" {
		t.Errorf("parseWitchtoonsSeriesPage() series = %+v, unexpected", page.Series)
	}
	if page.TotalPages != 2 {
		t.Errorf("parseWitchtoonsSeriesPage() totalPages = %d, want 2", page.TotalPages)
	}
	if len(page.Chapters) != 2 {
		t.Fatalf("parseWitchtoonsSeriesPage() got %d chapters, want 2", len(page.Chapters))
	}
	// most chapters carry a null title, which is what makes FetchChapters
	// fall back to "Chapter N"
	if page.Chapters[0].Number != 1 || page.Chapters[0].Title != "" {
		t.Errorf("parseWitchtoonsSeriesPage() chapters[0] = %+v, unexpected", page.Chapters[0])
	}
	if page.Chapters[1].Number != 2.5 || page.Chapters[1].Title != "Named one" {
		t.Errorf("parseWitchtoonsSeriesPage() chapters[1] = %+v, unexpected", page.Chapters[1])
	}

	if _, err := parseWitchtoonsSeriesPage("no payload here"); err == nil {
		t.Error("parseWitchtoonsSeriesPage() with no __next_f payload should error")
	}
}

// a series short enough to fit in a single chapter list page ships no
// totalPages at all, and must not be treated as having zero pages
func TestWitchtoonsSeriesPageDefaultsToOnePage(t *testing.T) {
	html := `<script>self.__next_f.push([1,"3:{\"series\":{\"title\":\"T\",\"slug\":\"t\"},\"chapters\":[{\"number\":1,\"title\":null}]}"])</script>`

	page, err := parseWitchtoonsSeriesPage(html)
	if err != nil {
		t.Fatalf("parseWitchtoonsSeriesPage() error = %v", err)
	}
	if page.TotalPages != 1 {
		t.Errorf("parseWitchtoonsSeriesPage() totalPages = %d, want 1", page.TotalPages)
	}
}

func TestWitchtoonsChapterPages(t *testing.T) {
	// reader payload: page URLs are relative and signed with a ?sig=&exp=
	// query string that must be kept verbatim for the download to work
	html := `<script>self.__next_f.push([1,"8:{\"number\":60,\"title\":null,\"content\":\"\",\"pages\":[{\"id\":\"p1\",\"pageNumber\":1,\"kind\":\"CONTENT\",\"imageUrl\":\"/uploads/comic-pages/some-series/60/page-001.webp?sig=abc&exp=123\",\"width\":800},{\"id\":\"p2\",\"pageNumber\":2,\"kind\":\"CONTENT\",\"imageUrl\":\"/uploads/comic-pages/some-series/60/page-002.webp?sig=def&exp=123\",\"width\":800}]}"])</script>`

	pages, err := witchtoonsChapterPages(html)
	if err != nil {
		t.Fatalf("witchtoonsChapterPages() error = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("witchtoonsChapterPages() got %d pages, want 2", len(pages))
	}
	if pages[0].ImageURL != "/uploads/comic-pages/some-series/60/page-001.webp?sig=abc&exp=123" {
		t.Errorf("witchtoonsChapterPages()[0] = %q, unexpected", pages[0].ImageURL)
	}
	if pages[1].ImageURL != "/uploads/comic-pages/some-series/60/page-002.webp?sig=def&exp=123" {
		t.Errorf("witchtoonsChapterPages()[1] = %q, unexpected", pages[1].ImageURL)
	}

	if _, err := witchtoonsChapterPages(`<script>self.__next_f.push([1,"8:{\"number\":60}"])</script>`); err == nil {
		t.Error("witchtoonsChapterPages() with no pages array should error")
	}
}

func TestWitchtoonsTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://witchtoons.net/series/comic/some-series", true},
		{"https://witchtoons.net/series/comic/some-series/chapter/12", true},
		// the old domain 301s to witchtoons.net dropping the path, so its
		// URLs can't be mapped to the new /series/comic/{slug} shape
		{"https://witchscans.com/manga/some-series/", false},
		{"https://example.com/series/comic/some-series", false},
	}

	for _, c := range cases {
		w := Witchtoons{Grabber: &Grabber{URL: c.url}}
		got, err := w.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
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
