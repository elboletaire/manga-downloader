// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// uzaymangaSeriesHTML is a trimmed copy of a series page of the Turkish
// platform shared by uzaymanga.com, eldermanga.com and friends: the first and
// latest chapters are large cards without the number badge, and must not be
// read as well as their list rows.
const uzaymangaSeriesHTML = `<html><head><title>Kunlun - Türkçe Manga Oku</title></head><body>
<h1 class="text-3xl md:text-4xl font-black">Kunlun Dağının Şeytani Ustası</h1>
<h1 id="uzay-seo-title" class="uzay-seo__title">Kunlun Dağının Şeytani Ustası Manga Oku</h1>
<a href="/manga/kunlun-daginin-seytani-ustasi/1-bolum-oku" class="flex items-center justify-between p-6 rounded-2xl" title="Kunlun 1 Manga Oku"><div class="flex flex-col"><span class="text-xs">İlk Bölüm</span> <span class="text-xl font-black">Bölüm 1</span></div></a>
<a href="/manga/kunlun-daginin-seytani-ustasi/2-bolum-oku" class="flex items-center justify-between p-4 rounded-2xl" title="Kunlun 2 Manga Oku"><div class="flex items-center gap-4"><div class="w-10 h-10 rounded-lg">2</div> <div class="flex flex-col"><span class="font-bold text-base-content">Kunlun Dağının Şeytani Ustası
                        2. Bölüm Manga Oku</span> </div></div></a>
<a href="/manga/kunlun-daginin-seytani-ustasi/1-bolum-oku" class="flex items-center justify-between p-4 rounded-2xl" title="Kunlun 1 Manga Oku"><div class="flex items-center gap-4"><div class="w-10 h-10 rounded-lg">1</div> <div class="flex flex-col"><span class="font-bold text-base-content">Kunlun Dağının Şeytani Ustası
                        1. Bölüm Manga Oku</span> </div></div></a>
</body></html>`

// uzaymangaReaderHTML is a trimmed copy of a reader page: the page images are
// plain <img> tags with an "... Sayfa N" alt, next to the site logo
const uzaymangaReaderHTML = `<html><head><title>Kunlun 2. Bölüm - Türkçe Manga Oku</title></head><body>
<img src="https://eldermanga.com/upload/logo.avif?w=256" alt="Logo"/>
<img src="https://cdn-u.example.can.re/_manga/25/2/1.avif" alt="Kunlun Dağının Şeytani Ustası - Bölüm 2 sayfa 1" class="w-full h-auto" loading="eager"/>
<img src="https://cdn-u.example.can.re/_manga/25/2/2.avif" alt="Kunlun Dağının Şeytani Ustası - Bölüm 2 sayfa 2" class="w-full h-auto" loading="lazy"/>
</body></html>`

func TestUzaymanga(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/kunlun-daginin-seytani-ustasi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(uzaymangaSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/kunlun-daginin-seytani-ustasi", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if title, _ := m.FetchTitle(); title != "Kunlun Dağının Şeytani Ustası" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected the 2 list rows only, got %d", len(chapters))
	}
	first := chapters[0].(*PlainHTMLChapter)
	if ChapterLabel(first) != "Chapter 2" {
		t.Errorf("unexpected title %q, want the series title and the Bölüm label stripped", ChapterLabel(first))
	}
	if first.GetNumber() != 2 || first.URL != srv.URL+"/manga/kunlun-daginin-seytani-ustasi/2-bolum-oku" {
		t.Errorf("unexpected first chapter: %v %q", first.GetNumber(), first.URL)
	}
}

func TestUzaymangaReaderImages(t *testing.T) {
	m := PlainHTML{site: SiteSelector{Image: `img[alt*="sayfa"]`}}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter{Number: 2}, ""}, docFromHTML(t, uzaymangaReaderHTML))

	want := []string{
		"https://cdn-u.example.can.re/_manga/25/2/1.avif",
		"https://cdn-u.example.can.re/_manga/25/2/2.avif",
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
