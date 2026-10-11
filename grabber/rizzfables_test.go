// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// rizzSeriesHTML is a trimmed copy of a series page: the chapter list is
// plain markup in #chapterlist, the same as en-thunderscans, so the rizz
// entry must be the one that matches.
const rizzSeriesHTML = `<html><head><title>An S-Rank Hunter's Healing Journey with Monsters - Rizz Fables</title></head><body>
<div class="search-chapter"><input id="searchchapter" type="text" autocomplete="off"></div>
<div class="eplister" id="chapterlist"><ul>
<li data-num="8"><div class="chbox"><div class="eph-num"><a href="https://rizzfables.com/chapter/r2311170-an-s-rank-hunters-healing-journey-with-monsters-chapter-8"><span class="chapternum">Chapter
	8 <i class="epn-name">
	</i></span><span class="chapterdate">August 6, 2025</span></a></div></div></li>
<li data-num="7"><div class="chbox"><div class="eph-num"><a href="https://rizzfables.com/chapter/r2311170-an-s-rank-hunters-healing-journey-with-monsters-chapter-7"><span class="chapternum">Chapter 7</span><span class="chapterdate">July 31, 2025</span></a></div></div></li>
<li data-num="0"><div class="chbox"><div class="eph-num"><a href="https://rizzfables.com/chapter/r2311170-an-s-rank-hunters-healing-journey-with-monsters-chapter-0"><span class="chapternum">Chapter 0</span><span class="chapterdate">July 21, 2025</span></a></div></div></li>
</ul></div>
<div class="readercontent"><h1 class="entry-title" itemprop="name">
	An S-Rank Hunter's Healing Journey with Monsters	</h1></div>
</body></html>`

// rizzReaderHTML is a trimmed copy of a reader page: the pages are direct
// children of #readerarea, while the Discord banner (inside an <a>) and the
// avatar sit outside of them
const rizzReaderHTML = `<html><head><title>An S-Rank Hunter's Healing Journey with Monsters Chapter 8</title></head><body>
<img src="/assets/images/default_pfp_mewo.webp">
<div id="readerarea" class="rdminimal">
<img src="https://cdn.55779955.xyz/file/roraor/wp-content/uploads/2025/08/06/f103-01-d1cb.webp" loading="eager">
<img src="https://cdn.55779955.xyz/file/roraor/wp-content/uploads/2025/08/06/c502-02-6c17.webp" loading="eager">
<a href="https://discord.com/invite/Yxhj5FbATU"> <img src="https://cdn.rizzfables.com/wp-content/uploads/2024/08/20/66c47bc2c15dd.webp" loading="lazy"></a>
</div>
</body></html>`

func TestRizzfables(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/series/r2311170-an-s-rank-hunters-healing-journey-with-monsters", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rizzSeriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/series/r2311170-an-s-rank-hunters-healing-journey-with-monsters", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != `#chapterlist li[data-num] a[href*="rizzfables.com/chapter/"]` {
		t.Errorf("Test() matched with selector %q, want the rizzfables one (an earlier entry shadows it)", m.site.Rows)
	}

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %v", err)
	}
	if title != "An S-Rank Hunter's Healing Journey with Monsters" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	wantNumbers := []float64{8, 7, 0}
	if len(chapters) != len(wantNumbers) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; u != "https://rizzfables.com/chapter/r2311170-an-s-rank-hunters-healing-journey-with-monsters-chapter-8" {
		t.Errorf("chapter 8 URL = %q", u)
	}
	if title := chapters[0].GetTitle(); title != "Chapter 8" {
		t.Errorf("chapter 8 title = %q, want %q", title, "Chapter 8")
	}
}

func TestRizzfablesReaderPages(t *testing.T) {
	m := PlainHTML{
		site: SiteSelector{Image: "#readerarea > img"},
	}
	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 8, Title: "Chapter 8"}}, docFromHTML(t, rizzReaderHTML))

	if len(chapter.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 (avatar and banner must be skipped)", len(chapter.Pages))
	}
	if chapter.Pages[1].URL != "https://cdn.55779955.xyz/file/roraor/wp-content/uploads/2025/08/06/c502-02-6c17.webp" {
		t.Errorf("second page URL = %q", chapter.Pages[1].URL)
	}
}
