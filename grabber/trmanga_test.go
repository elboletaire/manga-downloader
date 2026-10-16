// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// trmangaSeriesHTML is a trimmed copy of a series page: the chapter table is
// #chapters too (so the entry must come before the meowing #chapters ones),
// each row's first column is the chapter link, and the other links of the row
// (fansub, uploader) must not be read as chapters.
const trmangaSeriesHTML = `<html><head><title>No Place · TRManga</title></head><body>
<h1 class="movie__title h2">No Place for the Fake Princess</h1>
<table class="table" id="chapters">
<thead><tr><th>Bölüm</th><th>Fansub & Yükleyen</th><th>Tarih</th></tr></thead>
<tbody>
<tr class="fw-boldest font-size-lg wt-ch-row" data-chapter="2">
<td class="ps-0"><div><a href="/webtoon/no-place-for-the-fake-princess/2">#2                               .Bölüm</a></div></td>
<td class="text-end"><a class="text-black-50" href="/fansub/trwebtoon">TRwebtoon</a><a href="/@asoel">Asoel</a></td>
</tr>
<tr class="fw-boldest font-size-lg wt-ch-row" data-chapter="1">
<td class="ps-0"><div><a href="/webtoon/no-place-for-the-fake-princess/1">#1                               .Bölüm</a></div></td>
<td class="text-end"><a class="text-black-50" href="/fansub/trwebtoon">TRwebtoon</a></td>
</tr>
</tbody>
</table>
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
	if m.site.Rows != "tr.wt-ch-row td.ps-0 a" {
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
