// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// scanvfSeriesHTML is a trimmed copy of a series page: the navbar has its own
// <h1>, the series title is the first of two widget <h2>s (the second one is
// the chapters heading), and the chapter names carry no "Chapter" keyword.
const scanvfSeriesHTML = `<html><head><title>Lecture en ligne - Scans Jujutsu Kaisen Modulo VF</title></head><body>
<h1 class="" style="margin:0;"><a class="navbar-brand" href="https://www.scan-vf.net">ScanOnePiece</a></h1>
<h2 class="widget-title" style="display: inline-block;">Jujutsu Kaisen Modulo</h2>
<h2 class="widget-title">Jujutsu Kaisen Modulo Chapitres</h2>
<ul>
<li style="padding: 3px 0;" class="volume-0">
<h5 class="chapter-title-rtl">
<a href="/jujutsu-kaisen-modulo/chapitre-25">Jujutsu Kaisen Modulo 25</a> :
<em>Un future radieux</em>
</h5>
</li>
<li style="padding: 3px 0;" class="volume-0">
<h5 class="chapter-title-rtl">
<a href="/jujutsu-kaisen-modulo/chapitre-24.5">Jujutsu Kaisen Modulo 24.5</a> :
<em>Hors série</em>
</h5>
</li>
</ul>
</body></html>`

// scanvfReaderHTML is a trimmed copy of a reader page: every page has a
// placeholder data:image src and the real URL, padded with spaces, in data-src.
const scanvfReaderHTML = `<html><head><title>Jujutsu Kaisen Modulo</title></head><body>
<script>var pages = [{"page_image":"01.webp","page_slug":1,"external":0},{"page_image":"02.webp","page_slug":2,"external":0}]; var next_chapter = false;</script>
<img class="img-responsive" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" data-src=' https://www.scan-vf.net/uploads/manga/jujutsu-kaisen-modulo/chapters/chapitre-25/01.webp ' alt='Jujutsu Kaisen Modulo: Chapter chapitre-25 - Page 1'/>
<img class="img-responsive" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" data-src=' https://www.scan-vf.net/uploads/manga/jujutsu-kaisen-modulo/chapters/chapitre-25/02.webp ' alt='Jujutsu Kaisen Modulo: Chapter chapitre-25 - Page 2'/>
</body></html>`

func TestScanVF(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jujutsu-kaisen-modulo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(scanvfSeriesHTML))
	})
	mux.HandleFunc("/jujutsu-kaisen-modulo/chapitre-25", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(scanvfReaderHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/jujutsu-kaisen-modulo", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "h5.chapter-title-rtl" {
		t.Errorf("Test() matched with selector %q, want the scan-vf one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Jujutsu Kaisen Modulo" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Jujutsu Kaisen Modulo")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{25, 24.5}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if got := chapters[0].GetTitle(); got != "Un future radieux" {
		t.Errorf("chapter 0 title = %q, want %q", got, "Un future radieux")
	}

	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() returned %d pages, want 2", len(chapter.Pages))
	}
	if want := "https://www.scan-vf.net/uploads/manga/jujutsu-kaisen-modulo/chapters/chapitre-25/01.webp"; chapter.Pages[0].URL != want {
		t.Errorf("first page URL = %q, want %q", chapter.Pages[0].URL, want)
	}
}
