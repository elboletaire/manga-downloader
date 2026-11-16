// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// manhwaonlineSeriesHTML is a trimmed copy of a series page: the full list is
// one ul.chapter-list of plain links, newest first
const manhwaonlineSeriesHTML = `<html><head><title>A Wonderful New World Manhwa - ManhwaOnline.org</title></head><body>
<div class="main-info"><h1 class="main-info-title title-bigger">
                                A Wonderful New World                            </h1></div>
<ul class="chapter-list">
<li>
<a href="https://manhwaonline.org/manga/a-wonderful-new-world/chapter-2/" title="A Wonderful New World - Chapter 2">
<span class="chapter-name">Chapter 2</span>
<span class="ct-update">17 Jan 2026</span>
</a>
</li>
<li>
<a href="https://manhwaonline.org/manga/a-wonderful-new-world/chapter-1/" title="A Wonderful New World - Chapter 1">
<span class="chapter-name">Chapter 1</span>
<span class="ct-update">1 Jan 2026</span>
</a>
</li>
</ul>
</body></html>`

// manhwaonlineReaderHTML is a trimmed copy of a reader page: the pages are
// plain <img> tags inside .reading-content, the logo is outside of it
const manhwaonlineReaderHTML = `<html><head><title>A Wonderful New World - Chapter 1 - ManhwaOnline.org</title></head><body>
<img src="https://manhwaonline.org/wp-content/logo.png" alt="logo">
<div class="reading-img">
<div class="reading-content">
<p>
<img src="https://img11.manhwaonline.org/online/2405/1/1-1970f.jpg" alt="A Wonderful New World Chapter 1 - Page 1">
</p>
<p>
<img src="https://img11.manhwaonline.org/online/2405/1/2-eb136.jpg" alt="A Wonderful New World Chapter 1 - Page 2">
</p>
</div>
</div>
</body></html>`

func TestManhwaonline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/a-wonderful-new-world/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manhwaonlineSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/a-wonderful-new-world/", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "ul.chapter-list li a" {
		t.Errorf("Test() matched with selector %q, want the manhwaonline one", m.site.Rows)
	}
	if title, _ := m.FetchTitle(); title != "A Wonderful New World" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	first := chapters[0].(*PlainHTMLChapter)
	if first.GetNumber() != 2 || first.GetTitle() != "Chapter 2" {
		t.Errorf("unexpected first chapter: %v %q", first.GetNumber(), first.GetTitle())
	}
	if first.URL != "https://manhwaonline.org/manga/a-wonderful-new-world/chapter-2/" {
		t.Errorf("unexpected first chapter URL %q", first.URL)
	}
}

func TestManhwaonlineReaderImages(t *testing.T) {
	m := PlainHTML{site: SiteSelector{Image: ".reading-content img"}}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter{Number: 1}, ""}, docFromHTML(t, manhwaonlineReaderHTML))

	want := []string{
		"https://img11.manhwaonline.org/online/2405/1/1-1970f.jpg",
		"https://img11.manhwaonline.org/online/2405/1/2-eb136.jpg",
	}
	if len(chapter.Pages) != len(want) {
		t.Fatalf("expected %d pages, got %d", len(want), len(chapter.Pages))
	}
	for i, page := range chapter.Pages {
		if page.URL != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, page.URL, want[i])
		}
	}
}

// manhwaonlineesSeriesHTML is a trimmed copy of the Astro series page of
// manhwa-online.com: one div.chapter-row-full per chapter, its link holding the number
const manhwaonlineesSeriesHTML = `<html><head><title>Dear.00 manhwa - Manhwa-Online.com</title></head><body>
<h1 class="comic-title">Dear.00</h1>
<div class="chapter-list">
<div class="chapter-row-full" data-chapter-slug="capitulo-02" data-order="2"><a class="chapter-link" href="/manga/dear-00-4/capitulo-02/"><span class="ch-info"><span class="chapter-num">Capítulo 02</span></span></a></div>
<div class="chapter-row-full" data-chapter-slug="capitulo-01" data-order="1"><a class="chapter-link" href="/manga/dear-00-4/capitulo-01/"><span class="ch-info"><span class="chapter-num">Capítulo 01</span></span></a></div>
</div>
</body></html>`

// manhwaonlineesReaderHTML is a trimmed copy of the Astro reader: the pages are
// img.reader-page inside .reader-pages
const manhwaonlineesReaderHTML = `<html><body>
<img src="https://manhwa-online.com/media/covers/691/4ac6ed417d661e82/175x238.webp" class="mtile__img">
<div class="reader-pages" data-reading-content>
<img class="reader-page" src="https://stl.manhwa-online.com/dear-00/Cap%C3%ADtulo%2001/01.webp" id="image-0" alt="Dear.00 Capítulo 01 - página 1" data-position="1">
<img class="reader-page" src="https://stl.manhwa-online.com/dear-00/Cap%C3%ADtulo%2001/02.webp" id="image-1" alt="Dear.00 Capítulo 01 - página 2" data-position="2">
</div>
</body></html>`

func TestManhwaonlinees(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/dear-00-4/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manhwaonlineesSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/dear-00-4/", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "div.chapter-row-full a.chapter-link" {
		t.Errorf("Test() matched with selector %q, want the manhwa-online one", m.site.Rows)
	}
	if title, _ := m.FetchTitle(); title != "Dear.00" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	first := chapters[0].(*PlainHTMLChapter)
	if first.GetNumber() != 2 {
		t.Errorf("first chapter number = %v, want 2", first.GetNumber())
	}
}

func TestManhwaonlineesReaderImages(t *testing.T) {
	m := PlainHTML{site: SiteSelector{Image: "img.reader-page"}}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter{Number: 1}, ""}, docFromHTML(t, manhwaonlineesReaderHTML))

	want := []string{
		"https://stl.manhwa-online.com/dear-00/Cap%C3%ADtulo%2001/01.webp",
		"https://stl.manhwa-online.com/dear-00/Cap%C3%ADtulo%2001/02.webp",
	}
	if len(chapter.Pages) != len(want) {
		t.Fatalf("expected %d pages, got %d", len(want), len(chapter.Pages))
	}
	for i, page := range chapter.Pages {
		if page.URL != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, page.URL, want[i])
		}
	}
}
