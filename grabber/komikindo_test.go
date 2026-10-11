// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// komikindoSeriesHTML is a trimmed copy of a series page: the "Chapter Awal"
// box comes before the full list in #chapter_list, and each row carries a
// relative-date link next to the chapter link
const komikindoSeriesHTML = `<html><head><title>Komik Nano Machine - KomikIndo</title></head><body>
<div class="epsbr chapter-awal"><a href="https://komikindo.ch/nano-machine-chapter-1/"><div class="epsbr-left"><span>Chapter Awal </span><span class="barunew">Chapter
                        1                    </span></div></a></div>
<h1 class="entry-title" itemprop="name">
            Komik
              Nano Machine            </h1>
<div class="bxcl scrolling" id="chapter_list">
<ul>
  <li>
    <span class="lchx">
        <a href="https://komikindo.ch/nano-machine-chapter-333/" itemprop="url" rel="bookmark" title="Komik Nano Machine Chapter 333">
            Chapter <chapter>333</chapter></a></span>
            <span class="dt"><a href="https://komikindo.ch/nano-machine-chapter-333/">3 hari yang lalu</a></span>
    </li><li>
    <span class="lchx">
        <a href="https://komikindo.ch/nano-machine-chapter-98.5/" itemprop="url" rel="bookmark" title="Komik Nano Machine Chapter 98.5">
            Chapter <chapter>98.5</chapter></a></span>
            <span class="dt"><a href="https://komikindo.ch/nano-machine-chapter-98.5/">5 hari yang lalu</a></span>
    </li><li>
    <span class="lchx">
        <a href="https://komikindo.ch/nano-machine-chapter-98/" itemprop="url" rel="bookmark" title="Komik Nano Machine Chapter 98">
            Chapter <chapter>98</chapter></a></span>
            <span class="dt"><a href="https://komikindo.ch/nano-machine-chapter-98/">2 tahun yang lalu</a></span>
    </li>
</ul>
</div>
</body></html>`

// komikindoReaderHTML is a trimmed copy of a reader page: the pages are in
// #chimg-auh, after a blogger banner and ahead of an embedded video, both of
// which must be skipped
const komikindoReaderHTML = `<html><head><title>Komik Nano Machine Chapter 98 Bahasa Indonesia - KomikIndo</title></head><body>
<div class="chapter-image eastengine bc" id="Baca_Komik">
    <div class="gulai_asam_manis setdah"><a href="https://orangarab.fun/777">
<img src="https://blogger.googleusercontent.com/img/b/R29vZ2xl/AVvXsEjObdXKe3Db1vCZcvajoU7qAxQCS12l3xR7Ojww6gu9Kgu5wrMm89u_DVC8D.jpg">
</a></div>
    <div class="img-landmine">
        <div id="chimg-auh">
            <img src="https://imageainewgeneration.lol/data/588266/98/2086f132ec6ac18a36781c3ea422bad7/U140atAsC6uNs32DQiqiKPca8n9Ey4qjuvxKkgHv.jpg" alt="Nano Machine Chapter 98" onError="this.onerror=null;this.src='https://gaimgame.pics/data/588266/98/2086f132ec6ac18a36781c3ea422bad7/wKJZb78eSHX0rchSAfChjrxuZPs68TCbhLbf7kX1.jpg';"/><img src="https://indocontentaising.lol/data/588266/98/2086f132ec6ac18a36781c3ea422bad7/teJTgpO3PKbT4dNy4c43fOhmGzBGvoL0l0fQczfA.jpg" alt="Nano Machine Chapter 98"/>
        </div>
    </div>
<iframe src="https://www.youtube-nocookie.com/embed/iSfDoHOzFJo?autoplay=0&rel=0" title="Karrapo no Isu"></iframe>
</div>
</body></html>`

func TestKomikindo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/komik/155895-nano-machine/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(komikindoSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/komik/155895-nano-machine/", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != "#chapter_list li:has(.lchx)" {
		t.Errorf("Test() matched with selector %q, want the komikindo one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "Komik Nano Machine" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Komik Nano Machine")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{333, 98.5, 98}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if u := chapters[2].(*PlainHTMLChapter).URL; u != "https://komikindo.ch/nano-machine-chapter-98/" {
		t.Errorf("chapter 98 URL = %q", u)
	}
	if title := chapters[2].GetTitle(); title != "Chapter 98" {
		t.Errorf("chapter 98 title = %q, want %q", title, "Chapter 98")
	}
}

func TestKomikindoReaderPages(t *testing.T) {
	m := PlainHTML{
		site: SiteSelector{Image: "#chimg-auh img"},
	}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 98, Title: "Chapter 98"}}, docFromHTML(t, komikindoReaderHTML))

	if len(chapter.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (banner must be skipped)", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != "https://imageainewgeneration.lol/data/588266/98/2086f132ec6ac18a36781c3ea422bad7/U140atAsC6uNs32DQiqiKPca8n9Ey4qjuvxKkgHv.jpg" {
		t.Errorf("first page URL = %q", chapter.Pages[0].URL)
	}
}
