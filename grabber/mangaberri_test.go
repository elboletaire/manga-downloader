// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mangaberriSeriesHTML is a trimmed copy of a series page: the ad-blocker <h1>
// of the access wall comes before the real title, and the Volumes and RAW
// panels must not be read as chapters.
const mangaberriSeriesHTML = `<html><head><title>Dai Dark · MangaBerri</title></head><body>
<h1 id="site-access-title">Please disable your ad blocker</h1>
<div class="detail"><img class="detail-cover" src="https://mangaberri.com/media/source/cover/1016"><div><div class="eyebrow pink">ongoing · 54 CHAPTERS</div><h1>Dai Dark</h1></div></div>
<section class="chapter-section" data-chapter-browser>
<div class="section-heading"><div><h2>Read <span class="muted">(54)</span></h2>
<div class="chapter-tabs" role="tablist" aria-label="Chapter type">
<button type="button" role="tab" id="chapters-tab" aria-controls="chapters-panel" aria-selected="true" data-chapter-tab="chapters">Chapters <span>3</span></button>
<button type="button" role="tab" id="volumes-tab" aria-controls="volumes-panel" aria-selected="false" data-chapter-tab="volumes">Volumes <span>1</span></button>
<button type="button" role="tab" id="raw-tab" aria-controls="raw-panel" aria-selected="false" data-chapter-tab="raw">RAW <span>1</span></button>
</div></div></div>
<div class="chapter-list" id="chapters-panel" role="tabpanel" aria-labelledby="chapters-tab" data-chapter-panel="chapters">
<a data-chapter-group="chapters" data-chapter-name="50 chapter 50" href="https://mangaberri.com/read/dai-dark/46939"><strong>Chapter 50</strong><span>3 views · Oct 09, 2026 <span class="pink">↗</span></span></a>
<a data-chapter-group="chapters" data-chapter-name="37.5 chapter 37.5" href="https://mangaberri.com/read/dai-dark/46226"><strong>Chapter 37.5</strong><span>2 views · Oct 01, 2026 <span class="pink">↗</span></span></a>
<a data-chapter-group="chapters" data-chapter-name="37 chapter 37" href="https://mangaberri.com/read/dai-dark/46225"><strong>Chapter 37</strong><span>2 views · Oct 01, 2026 <span class="pink">↗</span></span></a>
</div>
<div class="chapter-list" id="volumes-panel" role="tabpanel" aria-labelledby="volumes-tab" data-chapter-panel="volumes" hidden>
<a data-chapter-group="volumes" data-chapter-name="1 volume 1" href="https://mangaberri.com/read/dai-dark/90001"><strong>Volume 1</strong><span>1 view</span></a>
</div>
<div class="chapter-list" id="raw-panel" role="tabpanel" aria-labelledby="raw-tab" data-chapter-panel="raw" hidden>
<a data-chapter-group="raw" data-chapter-name="1 raw 1" href="https://mangaberri.com/read/dai-dark/90002"><strong>Chapter 1</strong><span>1 view</span></a>
</div>
</section>
</body></html>`

// mangaberriReaderHTML is a trimmed copy of a reader page: the pages are plain
// <img> tags inside .reader-pages
const mangaberriReaderHTML = `<html><head><title>Dai Dark · Chapter 1 · MangaBerri</title></head><body>
<div class="reader-heading"><a href="https://mangaberri.com/title/dai-dark">← Dai Dark</a><h1>Chapter 1</h1><p>3 pages</p></div>
<div class="reader-pages"><figure><img decoding="async" src="https://mangaberri.com/media/source/page/1388613" alt="Dai Dark, chapter 1, page 1" data-page-image><figcaption hidden>Page 1 could not load.</figcaption></figure><figure><img decoding="async" src="https://mangaberri.com/media/source/page/1388614" alt="Dai Dark, chapter 1, page 2" loading="lazy" data-page-image></figure><figure><img decoding="async" src="https://mangaberri.com/media/source/page/1388615" alt="Dai Dark, chapter 1, page 3" loading="lazy" data-page-image></figure></div>
<img class="site-logo" src="/media/library/branding/site-logo.webp" alt="MangaBerri">
</body></html>`

func TestMangaberri(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/title/dai-dark", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mangaberriSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/title/dai-dark", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "#chapters-panel a[data-chapter-group]" {
		t.Errorf("Test() matched with selector %q, want the mangaberri one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Dai Dark" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Dai Dark")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{50, 37.5, 37}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d (volumes and RAW must be skipped)", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; u != "https://mangaberri.com/read/dai-dark/46939" {
		t.Errorf("chapter 50 URL = %q", u)
	}
}

func TestMangaberriReaderPages(t *testing.T) {
	m := PlainHTML{
		site: SiteSelector{Image: ".reader-pages img"},
	}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 1, Title: "Chapter 1"}}, docFromHTML(t, mangaberriReaderHTML))

	if chapter.PagesCount != 3 {
		t.Fatalf("PagesCount = %d, want 3", chapter.PagesCount)
	}
	if len(chapter.Pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != "https://mangaberri.com/media/source/page/1388613" {
		t.Errorf("first page URL = %q", chapter.Pages[0].URL)
	}
}
