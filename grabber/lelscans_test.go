// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lelScansReaderPage is a reader page showing page number page of a chapter
// with pages pages, its image and the pager
func lelScansReaderPage(chapter string, page, pages int) string {
	var pager strings.Builder
	for n := 1; n <= pages; n++ {
		fmt.Fprintf(&pager, `<a href="https://lelscans.net/scan-one-piece/%s/%d">%d</a>`, chapter, n, n)
	}
	return fmt.Sprintf(`<html><body><div id="navigation">%s</div>
		<div id="image"><a href="/scan-one-piece/%s/%d"><img src="/mangas/one-piece/%s/%02d.jpg?v=1"></a></div>
		</body></html>`, pager.String(), chapter, page+1, chapter, page-1)
}

func TestLelScansTest(t *testing.T) {
	cases := map[string]bool{
		"https://lelscans.net/lecture-en-ligne-one-piece": true,
		"https://www.lelscans.net/scan-one-piece/1100":    true,
		"https://lelscanfr.com/manga/one-piece":           false,
		"https://example.com/lecture-en-ligne-one-piece":  false,
	}
	for url, want := range cases {
		got, err := NewLelScans(&Grabber{URL: url}).Test()
		if err != nil {
			t.Fatalf("Test(%q) error = %v", url, err)
		}
		if got != want {
			t.Errorf("Test(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestLelScansFetchChapters(t *testing.T) {
	series := `<html><body><h1>Lecture en ligne One Piece scan</h1>
		<select><option value="https://lelscans.net/scan-one-piece/1195" selected>1195</option>
		<option value="https://lelscans.net/scan-one-piece/686.5">686.5</option>
		<option value="https://lelscans.net/scan-one-piece/680">680</option></select>
		<select></select></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lecture-en-ligne-one-piece" {
			_, _ = w.Write([]byte(series))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := NewLelScans(&Grabber{URL: srv.URL + "/lecture-en-ligne-one-piece"})
	if title, err := m.FetchTitle(); err != nil || title != "One Piece" {
		t.Errorf("FetchTitle() = %q, %v, want One Piece", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, want 3", len(chapters))
	}
	if c := chapters[1].(*LelScansChapter); c.GetNumber() != 686.5 || c.ID != "/scan-one-piece/686.5" {
		t.Errorf("chapter 686.5 = {number %v, ID %q}", c.GetNumber(), c.ID)
	}
}

func TestLelScansFetchChapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/scan-one-piece/1100":
			_, _ = w.Write([]byte(lelScansReaderPage("1100", 1, 3)))
		case "/scan-one-piece/1100/2":
			_, _ = w.Write([]byte(lelScansReaderPage("1100", 2, 3)))
		case "/scan-one-piece/1100/3":
			_, _ = w.Write([]byte(lelScansReaderPage("1100", 3, 3)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	listed := &LelScansChapter{
		Chapter: Chapter{Number: 1100, Title: "Chapter 1100", ID: "/scan-one-piece/1100"},
		URL:     srv.URL + "/scan-one-piece/1100",
	}
	chapter, err := NewLelScans(&Grabber{URL: srv.URL + "/lecture-en-ligne-one-piece"}).FetchChapter(listed)
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if len(chapter.Pages) != 3 {
		t.Fatalf("FetchChapter() pages = %d, want 3", len(chapter.Pages))
	}
	if got := chapter.Pages[2].URL; got != srv.URL+"/mangas/one-piece/1100/02.jpg?v=1" {
		t.Errorf("page 3 URL = %q", got)
	}
}

func TestLelScansResolveSeriesURL(t *testing.T) {
	reader := `<html><body><div itemscope>
		<a href="https://lelscans.net/index.php" itemprop="url"><span>Lelscan</span></a>
		<a href="https://lelscans.net/lecture-en-ligne-one-piece" itemprop="url"><span>One Piece</span></a>
		</div></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(reader))
	}))
	t.Cleanup(srv.Close)

	m := NewLelScans(&Grabber{URL: srv.URL + "/scan-one-piece/1100"})
	if err := m.resolveSeriesURL(); err != nil {
		t.Fatalf("resolveSeriesURL() error = %v", err)
	}
	if want := "https://lelscans.net/lecture-en-ligne-one-piece"; m.URL != want {
		t.Errorf("resolveSeriesURL() URL = %q, want %q", m.URL, want)
	}
}
