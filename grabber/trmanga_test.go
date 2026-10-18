// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// trmangaSeriesHTML is a trimmed copy of a series page (theme v3): the chapter
// list is #chapters too (so the entry must come before the meowing #chapters
// ones), each row's <a> is the chapter link, and the fansub and uploader links
// that follow it must not be read as chapters.
const trmangaSeriesHTML = `<html lang="tr" data-trm-theme="v3"><head><title>No Place · TRManga</title></head><body>
<h1 id="wt-title" class="text-3xl">No Place for the Fake Princess</h1>
<ol id="chapters" class="divide-y">
<li class="wt-ch" data-chapter="2" data-views="456">
<a href="/webtoon/no-place-for-the-fake-princess/2">
<span class="wt-ch-num tabular-nums">#2</span>
<span class="wt-ch-t">Bölüm</span>
<span class="badge badge-primary">Yeni</span>
</a>
<span class="wt-ch-m"><a href="/fansub/trwebtoon" title="Çeviri ekibi: TRwebtoon">TRwebtoon</a><a href="/@asoel" title="Yükleyen: Asoel">Asoel</a></span>
</li>
<li class="wt-ch" data-chapter="1" data-views="798">
<a href="/webtoon/no-place-for-the-fake-princess/1">
<span class="wt-ch-num tabular-nums">#1</span>
<span class="wt-ch-t">Bölüm</span>
</a>
<span class="wt-ch-m"><a href="/fansub/trwebtoon" title="Çeviri ekibi: TRwebtoon">TRwebtoon</a></span>
</li>
</ol>
</body></html>`

// trmangaReaderHTML is a trimmed copy of a reader page: only the first page
// carries a src, the others are lazy data-src images, and a comment-embedded
// preload tag must not be read.
const trmangaReaderHTML = `<html><head><title>No Place 1. Bölüm</title></head><body>
<!-- reader preload: <img src="https://trmanga.com/images/x/1/ignored.webp" data-src="https://trmanga.com/images/x/1/ignored.webp"> -->
<div class="rd-reader rd-section">
<div id="images">
<img class="reader-img" width="601" height="919" src="https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/first.webp" alt="No Place for the Fake Princess 1. Bölüm - Sayfa 1">
<img class="reader-img" width="940" height="4000" data-src="https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/second.webp" decoding="async" alt="No Place for the Fake Princess 1. Bölüm - Sayfa 2">
<img class="reader-img" width="940" height="4000" data-src="https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/third.webp" decoding="async" alt="No Place for the Fake Princess 1. Bölüm - Sayfa 3">
</div>
</div>
</body></html>`

func TestTrmanga(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/webtoon/no-place-for-the-fake-princess", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(trmangaSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/webtoon/no-place-for-the-fake-princess", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "ol#chapters > li.wt-ch[data-chapter] > a[href]" {
		t.Errorf("Test() matched with selector %q, want the trmanga one (an earlier entry shadows it)", m.site.Rows)
	}
	if title, _ := m.FetchTitle(); title != "No Place for the Fake Princess" {
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
	if first.GetNumber() != 2 || first.URL != srv.URL+"/webtoon/no-place-for-the-fake-princess/2" {
		t.Errorf("unexpected first chapter: %v %q", first.GetNumber(), first.URL)
	}
	if ChapterLabel(first) != "Chapter 2" {
		t.Errorf("unexpected title %q, want the bare number labelled", ChapterLabel(first))
	}
}

func TestTrmangaReaderImages(t *testing.T) {
	m := PlainHTML{site: SiteSelector{Image: "#images img.reader-img"}}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter{Number: 1}, ""}, docFromHTML(t, trmangaReaderHTML))

	want := []string{
		"https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/first.webp",
		"https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/second.webp",
		"https://cdn.trmanga.com/images/no-place-for-the-fake-princess/1/third.webp",
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
