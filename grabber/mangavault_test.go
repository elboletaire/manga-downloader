// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mangavaultSeriesHTML is a trimmed copy of a series page: the "Leer" button
// sits outside the chapter <ul>, and the "Cap. N" labels hold a comment node
// between the label and the number, which Text() drops.
const mangavaultSeriesHTML = `<html><head><title>Control Mental — MangaVault</title></head><body>
<h1 class="text-balance text-3xl font-bold">Control Mental</h1>
<a href="/manga/control-mental-f7d784/1" class="flex w-full">Leer desde el inicio</a>
<ul class="divide-y divide-border">
<li class="group flex items-center"><a href="/manga/control-mental-f7d784/2" class="flex flex-1"><span class="shrink-0 text-sm">Cap. <!-- -->2</span><span class="flex-1"></span><time class="shrink-0">09 oct 2026</time></a></li>
<li class="group flex items-center"><a href="/manga/control-mental-f7d784/1" class="flex flex-1"><span class="shrink-0 text-sm">Cap. <!-- -->1</span><span class="flex-1"></span><time class="shrink-0">01 oct 2026</time></a></li>
</ul>
</body></html>`

// mangavaultReaderHTML is a trimmed copy of a reader page: plain <img> tags on
// the media host, next to a site logo that must not be read as a page.
const mangavaultReaderHTML = `<html><head><title>Control Mental Cap. 20</title></head><body>
<img src="/logo.svg" alt="MangaVault">
<img src="https://media.mangavault.lat/manga/f7d78419e5e4438588f46eea101f500d/ch-20/0000-7dcb2b7f.jpg" alt="Página 1" loading="eager">
<img src="https://media.mangavault.lat/manga/f7d78419e5e4438588f46eea101f500d/ch-20/0001-7d19605d.jpg" alt="Página 2" loading="eager">
</body></html>`

func TestMangavault(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/control-mental-f7d784", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangavaultSeriesHTML))
	})
	mux.HandleFunc("/manga/control-mental-f7d784/2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangavaultReaderHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/control-mental-f7d784", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "ul.divide-y a" {
		t.Errorf("Test() matched with selector %q, want the mangavault one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Control Mental" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Control Mental")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("FetchChapters() returned %d chapters, want 2", len(chapters))
	}
	for i, want := range []float64{2, 1} {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}

	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() returned %d pages, want 2", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != "https://media.mangavault.lat/manga/f7d78419e5e4438588f46eea101f500d/ch-20/0000-7dcb2b7f.jpg" {
		t.Errorf("first page URL = %q", chapter.Pages[0].URL)
	}
}
