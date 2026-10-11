// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mangaworldSeriesHTML is a trimmed copy of a series page: unquoted attributes,
// one chapter row per <div class="chapter">, the Italian "Capitolo" label, a
// decimal chapter and a volume block that must not be read twice.
const mangaworldSeriesHTML = `<html><head><title>Kingdom Scan ITA - MangaWorld</title></head><body>
<h1 class="name bigger">Kingdom</h1>
<div class="volume-chapters pl-2" data-index=1>
<div class=chapter><a class=chap href=/manga/1683/kingdom/read/aaa111 title="Kingdom Capitolo 836 Scan ITA"><span class=d-inline-block>Capitolo 836</span><i class="text-right text-muted chap-date">11 Ottobre 2026</i><div class=clearfix></div></a></div>
<div class=chapter><a class=chap href=/manga/1683/kingdom/read/bbb222 title="Kingdom Capitolo 835.5 Scan ITA"><span class=d-inline-block>Capitolo 835.5</span><i class="text-right text-muted chap-date">10 Ottobre 2026</i><div class=clearfix></div></a></div>
<div class=chapter><a class=chap href=https://www.mangaworld.mx/manga/1683/kingdom/read/ccc333 title="Kingdom Capitolo 835 Scan ITA"><span class=d-inline-block>Capitolo 835</span><i class="text-right text-muted chap-date">9 Ottobre 2026</i><div class=clearfix></div></a></div>
</div>
<a class="btn btn-dark mr-1 mt-1" href=/manga/1683/kingdom/read/aaa111>Leggi</a>
</body></html>`

// mangaworldReaderHTML is a trimmed copy of a reader page: the pages are only
// rendered as img.page-image with ?style=list, the default view shows none.
const mangaworldReaderHTML = `<html><head><title>Kingdom Capitolo 836</title></head><body>
<div id=pages><img id=page-0 class="page-image img-fluid" src=https://cdn.mangaworld.mx/chapters/kingdom/capitolo-836/1.jpg>
<img id=page-1 class="page-image img-fluid" src=https://cdn.mangaworld.mx/chapters/kingdom/capitolo-836/2.jpg></div>
</body></html>`

func TestMangaworld(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/1683/kingdom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangaworldSeriesHTML))
	})
	mux.HandleFunc("/manga/1683/kingdom/read/aaa111", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("style") != "list" {
			_, _ = w.Write([]byte("<html><body></body></html>"))
			return
		}
		_, _ = w.Write([]byte(mangaworldReaderHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/1683/kingdom", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "div.chapter a.chap" {
		t.Errorf("Test() matched with selector %q, want the mangaworld one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Kingdom" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Kingdom")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{836, 835.5, 835}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if got := chapters[0].GetTitle(); got != "Capitolo 836" {
		t.Errorf("chapter 0 title = %q, want %q", got, "Capitolo 836")
	}

	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() returned %d pages, want 2 (?style=list not applied?)", len(chapter.Pages))
	}
	if chapter.PagesCount != 2 {
		t.Errorf("PagesCount = %d, want 2", chapter.PagesCount)
	}
}
