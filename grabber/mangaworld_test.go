// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mangaworldSeriesHTML is a trimmed copy of a series page: one <li class="ch-row">
// per chapter in its volume's list, the Italian "Capitolo" label, a decimal
// chapter and the volume header that gives the rows their volume.
const mangaworldSeriesHTML = `<html><head><title>Kingdom Scan ITA - MangaWorld</title></head><body>
<h1 class="manga-title display">Kingdom</h1>
<div class="volume">
<div class="volume-row"><button class="volume-head" type="button"><span class="volume-name">Volume 77</span></button></div>
<ul class="ch-list volume-chapters" id="volume-0">
<li class="ch-row"><a class="ch" href="/manga/1683/kingdom/read/aaa111" title="Kingdom Capitolo 836 Scan ITA"><span class="ch-label"><span class="ch-name">Capitolo 836</span></span><span class="ch-date">20 Ottobre 2026</span></a></li>
<li class="ch-row"><a class="ch" href="https://www.mangaworld.me/manga/1683/kingdom/read/bbb222" title="Kingdom Capitolo 835.5 Scan ITA"><span class="ch-label"><span class="ch-name">Capitolo 835.5</span></span><span class="ch-date">15 Ottobre 2026</span></a></li>
<li class="ch-row"><a class="ch" href="/manga/1683/kingdom/read/ccc333" title="Kingdom Capitolo 835 Scan ITA"><span class="ch-label"><span class="ch-name">Capitolo 835</span></span><span class="ch-date">11 Ottobre 2026</span></a></li>
</ul>
</div>
</body></html>`

// mangaworldReaderHTML is a trimmed copy of a reader page: every page is an
// img.reader-page, with or without a query.
const mangaworldReaderHTML = `<html><head><title>Kingdom Capitolo 836</title></head><body>
<img class="reader-page" src="https://cdn.mangaworld.me/chapters/kingdom/capitolo-836/1.png" alt="Kingdom Capitolo 836 - Pagina 1" data-reader-image>
<img class="reader-page" src="https://cdn.mangaworld.me/chapters/kingdom/capitolo-836/2.png" alt="Kingdom Capitolo 836 - Pagina 2" loading="lazy" data-reader-image>
</body></html>`

func TestMangaworld(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/1683/kingdom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangaworldSeriesHTML))
	})
	mux.HandleFunc("/manga/1683/kingdom/read/aaa111", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangaworldReaderHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/1683/kingdom", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "li.ch-row a.ch" {
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
