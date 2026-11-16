// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// mhscansFixture reads a saved mhscans.com page from testdata/mhscans
func mhscansFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "mhscans", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mhscansServer is mhscansServerHeader sending the fragments' count as X-Pages
func mhscansServer(t *testing.T, series string, fragments map[string]string, reader string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	return mhscansServerHeader(t, series, fragments, reader, strconv.Itoa(len(fragments)))
}

// mhscansServerHeader serves the series page at /comics/<slug>, its fragments at
// /comics/<slug>/chapters?page=N, the last one repeated past the end (as the
// site does), and a reader at /leer/<id>. It sends xPages as the X-Pages
// header, none when empty, and counts the fragment requests.
func mhscansServerHeader(t *testing.T, series string, fragments map[string]string, reader, xPages string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var fragmentHits atomic.Int32
	pages := len(fragments)
	slug := slugOf(series)
	mux := http.NewServeMux()
	mux.HandleFunc("/comics/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/comics/"+slug {
			_, _ = w.Write([]byte(series))
			return
		}
		if r.URL.Path != "/comics/"+slug+"/chapters" {
			http.NotFound(w, r)
			return
		}
		fragmentHits.Add(1)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = min(max(page, 1), pages)
		if xPages != "" {
			w.Header().Set("X-Pages", xPages)
		}
		_, _ = w.Write([]byte(fragments[strconv.Itoa(page)]))
	})
	mux.HandleFunc("/leer/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(reader))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &fragmentHits
}

// slugOf is the series slug in a fixture's canonical URL
func slugOf(series string) string {
	_, rest, _ := strings.Cut(series, `<link rel="canonical" href="https://mhscans.com/comics/`)
	slug, _, _ := strings.Cut(rest, `"`)
	return slug
}

// TestMhscansChapterList checks that a series page's first 30 chapters are
// completed from the /chapters fragments up to their X-Pages count: all 90
// chapters of the series, each once, with no fetch past the last page.
func TestMhscansChapterList(t *testing.T) {
	series := mhscansFixture(t, "series.html")
	fragments := map[string]string{
		"1": series,
		"2": mhscansFixture(t, "chapters-2.html"),
		"3": mhscansFixture(t, "chapters-3.html"),
	}
	srv, hits := mhscansServer(t, series, fragments, "")
	// the series page is fragment 1: its rows are the page's own, so the first
	// fragment isn't fetched
	seriesURL := srv.URL + "/comics/" + slugOf(series)
	m := NewPlainHTML(&Grabber{URL: seriesURL, Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the mhscans selector to take the series", ok, err)
	}
	if m.site.ChaptersPath != "/chapters" {
		t.Fatalf("Test() matched a selector without ChaptersPath")
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 90 {
		t.Fatalf("FetchChapters() = %d chapters, want 90", len(chapters))
	}
	seen := map[float64]bool{}
	for _, c := range chapters {
		seen[c.GetNumber()] = true
	}
	if len(seen) != 90 {
		t.Errorf("FetchChapters() has %d distinct numbers, want 90", len(seen))
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("chapter fragments fetched %d times, want 2 (pages 2 and 3)", got)
	}
}

// TestMhscansRepeatedPageStops checks that without a usable X-Pages (none, 0 or
// junk) the walk still ends at the first repeated page, with no duplicate rows
func TestMhscansRepeatedPageStops(t *testing.T) {
	series := mhscansFixture(t, "series.html")
	fragments := map[string]string{
		"1": series,
		"2": mhscansFixture(t, "chapters-2.html"),
		"3": mhscansFixture(t, "chapters-3.html"),
	}
	for _, header := range []string{"", "0", "junk"} {
		srv, hits := mhscansServerHeader(t, series, fragments, "", header)
		m := NewPlainHTML(&Grabber{URL: srv.URL + "/comics/" + slugOf(series), Settings: &Settings{}})
		if ok, err := m.Test(); err != nil || !ok {
			t.Fatalf("X-Pages %q: Test() = %v, %v; want true", header, ok, err)
		}
		chapters, errs := m.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("X-Pages %q: FetchChapters() errors = %v", header, errs)
		}
		if len(chapters) != 90 {
			t.Errorf("X-Pages %q: FetchChapters() = %d chapters, want 90", header, len(chapters))
		}
		// pages 2 and 3, then page 4, which repeats page 3 and ends the walk
		if got := hits.Load(); got != 3 {
			t.Errorf("X-Pages %q: fragments fetched %d times, want 3", header, got)
		}
	}
}

// TestMhscansLockedChapter checks that a paid row (its link goes to the login
// page) is listed as Locked, and that its free neighbours are not
func TestMhscansLockedChapter(t *testing.T) {
	series := mhscansFixture(t, "cronicas.html")
	srv, _ := mhscansServer(t, series, map[string]string{"1": series}, "")
	m := NewPlainHTML(&Grabber{URL: srv.URL + "/comics/" + slugOf(series), Settings: &Settings{}})
	if ok, err := m.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want true", ok, err)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, want 3", len(chapters))
	}
	for _, f := range chapters {
		c := f.(*PlainHTMLChapter)
		if want := c.GetNumber() == 6; c.Locked != want {
			t.Errorf("chapter %v Locked = %v, want %v", c.GetNumber(), c.Locked, want)
		}
		if c.GetNumber() == 6 {
			if got := c.GetTitle(); got != "Entrando al Vacío (2)" {
				t.Errorf("chapter 6 title = %q, want %q (the label's chapter word is stripped)", got, "Entrando al Vacío (2)")
			}
		}
	}
}

// TestMhscansReaderResolvesSeries checks that a pasted /leer/<id> link resolves
// to its series, which lists every chapter
func TestMhscansReaderResolvesSeries(t *testing.T) {
	series := mhscansFixture(t, "cronicas.html")
	reader := mhscansFixture(t, "reader.html")
	srv, _ := mhscansServer(t, series, map[string]string{"1": series}, reader)
	readerURL := srv.URL + "/leer/GyFaKf4HeGpDIOydb7V2q"
	m := NewPlainHTML(&Grabber{URL: readerURL, Settings: &Settings{}})
	// a reader lists its chapters in <option>s, not rows, so Test() declines
	// and the series is found as IdentifySite's final fallback
	if ok, err := m.Test(); err != nil || ok {
		t.Fatalf("Test() = %v, %v; want false for a reader", ok, err)
	}
	if !m.resolveSeriesURL() {
		t.Fatal("resolveSeriesURL() = false; want the series")
	}
	if want := srv.URL + "/comics/" + slugOf(series); m.URL != want {
		t.Errorf("Test() resolved the reader to %q, want the series %q", m.URL, want)
	}
	chapters, _ := m.FetchChapters()
	if len(chapters) != 3 {
		t.Errorf("FetchChapters() after resolving = %d chapters, want 3", len(chapters))
	}
}

// TestMhscansReaderPages checks that a reader's pages are read from the
// <img> tags, lazy ones through data-src
func TestMhscansReaderPages(t *testing.T) {
	reader := mhscansFixture(t, "reader.html")
	cronicas := mhscansFixture(t, "cronicas.html")
	srv, _ := mhscansServer(t, cronicas, map[string]string{"1": cronicas}, reader)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(reader))
	if err != nil {
		t.Fatal(err)
	}
	wantPages := doc.Find("img.page-img").Length()
	m := NewPlainHTML(&Grabber{URL: srv.URL, Settings: &Settings{}})
	m.site = plainHTMLSelectors[len(plainHTMLSelectors)-1]
	f := &PlainHTMLChapter{Chapter: Chapter{Number: 1}, URL: srv.URL + "/leer/GyFaKf4HeGpDIOydb7V2q"}
	chapter, err := m.FetchChapter(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != wantPages {
		t.Fatalf("FetchChapter() = %d pages, want %d", len(chapter.Pages), wantPages)
	}
	const cdn = "https://mhscans.com/uploads/pages/"
	for i, p := range chapter.Pages {
		if !strings.HasPrefix(p.URL, cdn) {
			t.Errorf("page %d URL = %q, want a reader upload", i, p.URL)
		}
	}
}
