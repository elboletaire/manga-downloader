// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

const rawkumaSeriesPath = "/manga/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/"

// rawkumaSeriesHTML is a trimmed copy of a series page: a "Last Updates"
// sidebar <h1> comes before the real title, and the chapter rows carry the
// number on their wrapper, with decimal chapters in the URL as
// chapter-29.6.<postid>
const rawkumaSeriesHTML = `<html><head><title>Romelia Senki - Rawkuma</title></head><body>
<h1 class="font-semibold text-base text-gray-100 flex items-center justify-start"><span data-lucide="calendar" class="mx-1 inline-block w-6 h-6"></span><span class="font-semibold">Last Updates</span></h1>
<h1 class="text-2xl font-bold leading-[1.5rem] text-gray-100" itemprop="name"> Romelia Senki ~Maou wo Taoshita Ato mo Jinrui Yabasou Dakara Guntai Soshiki Shita~ </h1>
<div id="chapter-list" class="mt-4 space-y-2 overflow-auto max-h-[512px]">
<div data-chapter-number="29.6" class="flex flex-1 relative rounded-md cursor-pointer"><a href="https://rawkuma.net/manga/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/chapter-29.6.391587/" class="w-full h-full flex items-center justify-between"><div class="flex w-full flex-col pr-3.5"><div class="flex flex-row gap-1 font-medium text-base"><span>Chapter 29.6</span></div><div class="text-sm"><time datetime="2026-09-04T12:53:20Z">1 month ago</time></div></div></a><div class="last flex flex-row"><p class="font-bold">3.5K</p></div></div>
<div data-chapter-number="1" class="flex flex-1 relative rounded-md cursor-pointer"><a href="https://rawkuma.net/manga/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/chapter-1.123925/" class="w-full h-full flex items-center justify-between"><div class="flex w-full flex-col pr-3.5"><div class="flex flex-row gap-1 font-medium text-base"><span>Chapter 1</span></div><div class="text-sm"><time datetime="2025-10-01T00:00:00Z">1 year ago</time></div></div></a><div class="last flex flex-row"><a href="https://drive.google.com/uc?id=1abc&#038;export=download">Download</a></div></div>
</div>
</body></html>`

// rawkumaReaderHTML is a trimmed copy of a reader page: the pages are plain
// <img>s, all server-rendered inside the data-image-data section
const rawkumaReaderHTML = `<html><head><title>ROMELIA SENKI Chapter 29.6 - Rawkuma</title></head><body>
<div class="relative"><section class="mx-auto w-full md:w-[800px]"><div class="mx-4 md:mx-0"></div><section class="w-full flex flex-col" data-image-data="1"><img src="https://kuma.kyut.dev/wp-content/scr/r/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/29.6/1.jpg" alt="" /><img src="https://kuma.kyut.dev/wp-content/scr/r/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/29.6/2.jpg" alt="" /><img src="https://kuma.kyut.dev/wp-content/scr/r/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/29.6/3.jpg" alt="" /><img src="https://kuma.kyut.dev/wp-content/scr/r/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/29.6/4.jpg" alt="" /><img src="https://kuma.kyut.dev/wp-content/scr/r/romelia-senki-maou-wo-taoshita-ato-mo-jinrui-yabasou-dakara-guntai-soshiki-shita/29.6/5.jpg" alt="" /></section></section></div>
</body></html>`

func TestRawkuma(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(rawkumaSeriesPath, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rawkumaSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + rawkumaSeriesPath, Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "#chapter-list [data-chapter-number]" {
		t.Errorf("Test() matched with selector %q, want the rawkuma one", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if want := "Romelia Senki ~Maou wo Taoshita Ato mo Jinrui Yabasou Dakara Guntai Soshiki Shita~"; title != want {
		t.Errorf("FetchTitle() = %q, want %q (the sidebar <h1> must not leak in)", title, want)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("FetchChapters() returned %d chapters, want 2", len(chapters))
	}
	if n := chapters[0].GetNumber(); n != 29.6 {
		t.Errorf("first chapter number = %v, want 29.6 (not the postid)", n)
	}
	if n := chapters[1].GetNumber(); n != 1 {
		t.Errorf("second chapter number = %v, want 1", n)
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; !strings.HasSuffix(u, "/chapter-29.6.391587/") {
		t.Errorf("first chapter URL = %q, want the chapter-29.6.391587 reader", u)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawkumaReaderHTML))
	if err != nil {
		t.Fatalf("parsing reader fixture: %v", err)
	}
	chapter := m.chapterFromDoc(chapters[0], doc)
	if chapter.PagesCount != 5 || len(chapter.Pages) != 5 {
		t.Fatalf("chapterFromDoc() has %d pages (count %d), want 5", len(chapter.Pages), chapter.PagesCount)
	}
	if u := chapter.Pages[4].URL; !strings.HasSuffix(u, "/29.6/5.jpg") {
		t.Errorf("last page URL = %q, want .../29.6/5.jpg", u)
	}
}
