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
