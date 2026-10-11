// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// maidSeriesHTML is a trimmed copy of a series page: each chapter row nests
// its date in a .date span inside the chapter link
const maidSeriesHTML = `<html><head><title>Watashi no Oshi wa Akuyaku Reijou. Bahasa Indonesia - Maid - Manga Indonesia</title></head><body>
<div class="series-info">
<div class="series-titlex"><h2>Watashi no Oshi wa Akuyaku Reijou.</h2><span></span></div>
</div>
<div class="series-title"><h2>Watashi no Oshi wa Akuyaku Reijou.</h2><span></span></div>
<div class="series-chapter"><h2><span>Chapter</span> List</h2><ul class="series-chapterlist"><li>
<div class="flexch">
<div class="flexch-book"><i class="icon ion-md-book"></i></div>
<div class="flexch-infoz">
<a href="https://www.maid.my.id/watashi-no-oshi-wa-akuyaku-reijou-chapter-63-bahasa-indonesia/" title="Watashi no Oshi wa Akuyaku Reijou. Chapter 63 Bahasa Indonesia"><span>Chapter 63<span class="date">September 9, 2026</span></span></a>
</div>
</div>
</li><li>
<div class="flexch">
<div class="flexch-infoz">
<a href="https://www.maid.my.id/watashi-no-oshi-wa-akuyaku-reijou-chapter-62-5-bahasa-indonesia/" title="Watashi no Oshi wa Akuyaku Reijou. Chapter 62.5 Bahasa Indonesia"><span>Chapter 62.5<span class="date">September 2, 2026</span></span></a>
</div>
</div>
</li><li>
<div class="flexch">
<div class="flexch-infoz">
<a href="https://www.maid.my.id/watashi-no-oshi-wa-akuyaku-reijou-chapter-01-bahasa-indonesia/" title="Watashi no Oshi wa Akuyaku Reijou. Chapter 01 Bahasa Indonesia"><span>Chapter 01<span class="date">July 1, 2026</span></span></a>
</div>
</div>
</li></ul></div>
</body></html>`

// maidReaderHTML is a trimmed copy of a reader page: the pages are lazy-loaded
// from imgbox.com, each one also repeated as a plain <img> without the
// data-lazy-src attribute, while the community banner is hosted on the site
const maidReaderHTML = `<html><head><title>Watashi no Oshi wa Akuyaku Reijou. Chapter 50 Bahasa Indonesia - Maid - Manga Indonesia</title></head><body>
<div class="reader-area">
<img src="data:image/svg+xml,%3Csvg%20xmlns='http://www.w3.org/2000/svg'%20viewBox='0%200%200%200'%3E%3C/svg%3E" alt="Informasi Komunitas" data-lazy-src="https://www.maid.my.id/wp-content/uploads/2026/06/fLvGlt4v_o.jpg"/>
<img decoding="async" src="data:image/svg+xml,%3Csvg%20xmlns='http://www.w3.org/2000/svg'%20viewBox='0%200%200%200'%3E%3C/svg%3E" alt="image host" data-lazy-src="https://images2.imgbox.com/8b/d5/AtxBu25B_o.jpg"/>
<img decoding="async" src="https://images2.imgbox.com/8b/d5/AtxBu25B_o.jpg" alt="image host"/>
<img decoding="async" src="data:image/svg+xml,%3Csvg%20xmlns='http://www.w3.org/2000/svg'%20viewBox='0%200%200%200'%3E%3C/svg%3E" alt="image host" data-lazy-src="https://images2.imgbox.com/f1/f4/aTuyCrvJ_o.jpg"/>
<img decoding="async" src="https://images2.imgbox.com/f1/f4/aTuyCrvJ_o.jpg" alt="image host"/>
</div>
</body></html>`

func TestMaid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/watashi-no-oshi-wa-akuyaku-reijou/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(maidSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/watashi-no-oshi-wa-akuyaku-reijou/", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != ".series-chapterlist li" {
		t.Errorf("Test() matched with selector %q, want the maid one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Watashi no Oshi wa Akuyaku Reijou." {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{63, 62.5, 1}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; u != "https://www.maid.my.id/watashi-no-oshi-wa-akuyaku-reijou-chapter-63-bahasa-indonesia/" {
		t.Errorf("chapter 63 URL = %q", u)
	}
	if title := chapters[0].GetTitle(); title != "Chapter 63" {
		t.Errorf("chapter 63 title = %q, want %q (the date must be dropped)", title, "Chapter 63")
	}
}

func TestMaidReaderPages(t *testing.T) {
	m := PlainHTML{
		site: SiteSelector{Image: `.reader-area img[data-lazy-src*="imgbox.com"]`},
	}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 50, Title: "Chapter 50"}}, docFromHTML(t, maidReaderHTML))

	if len(chapter.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (banner and duplicates must be skipped)", len(chapter.Pages))
	}
	if chapter.Pages[1].URL != "https://images2.imgbox.com/f1/f4/aTuyCrvJ_o.jpg" {
		t.Errorf("second page URL = %q", chapter.Pages[1].URL)
	}
}
