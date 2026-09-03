// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

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
