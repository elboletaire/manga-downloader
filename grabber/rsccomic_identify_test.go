// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// rscComicSeriesPath is a series page of the platform, the shape the URL gate accepts
const rscComicSeriesPath = "/series/comic/i-am-a-cultivation-big-shot"

// fixtureHandler answers every request with an identify fixture
func fixtureHandler(t *testing.T, file string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(identifyFixtureDir, file))
	if err != nil {
		t.Fatal(err)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	}
}

// TestRSCComicByMarkupAcceptsUnlistedHost checks a host outside the domain list
// is identified as RSCComic from a single GET of its series and reader pages
func TestRSCComicByMarkupAcceptsUnlistedHost(t *testing.T) {
	cases := []struct {
		file, path string
	}{
		{"rsc_series.html", rscComicSeriesPath},
		{"rsc_chapter.html", rscComicSeriesPath + "/chapter/1"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			srv, counter := newCountingServer(t, fixtureHandler(t, c.file))

			site, _ := NewSite(srv.URL+c.path, &Settings{})
			if _, ok := site.(*RSCComic); !ok {
				t.Fatalf("IdentifySite() = %T, want *RSCComic", site)
			}
			if got := counter.count(c.path); got != 1 {
				t.Errorf("page requests = %d, want 1", got)
			}
		})
	}
}

// TestRSCComicByMarkupRejectsOtherPages checks pages of other platforms, and
// the chapter-list markup of a themesia or Madara site, don't match the markers
func TestRSCComicByMarkupRejectsOtherPages(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"madara series", fixtureHandler(t, "madara_series.html")},
		{"themesia series", fixtureHandler(t, "themesia_series.html")},
		{"unrelated page", fixtureHandler(t, "unrelated_page.html")},
		{"tanstack series", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html><script>window.d={initialChapters:$R[1],totalChapterCount:12}</script></html>`))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := newCountingServer(t, c.handler)

			r := NewRSCComicByMarkup(&Grabber{URL: srv.URL + rscComicSeriesPath})
			got, err := r.Test()
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			if got {
				t.Error("Test() = true, want false")
			}
		})
	}
}

// TestRSCComicSkipsRequests checks URLs outside the URL gate, and the
// domain-only instance on an unlisted host, never fetch the page
func TestRSCComicSkipsRequests(t *testing.T) {
	srv, counter := newCountingServer(t, fixtureHandler(t, "rsc_series.html"))

	paths := []string{"/", "/manga/some-series/", "/series/some-series", "/series/comic/", "/series/comicfoo/x"}
	for _, path := range paths {
		r := NewRSCComicByMarkup(&Grabber{URL: srv.URL + path})
		got, err := r.Test()
		if err != nil {
			t.Fatalf("Test(%q) error = %v", path, err)
		}
		if got {
			t.Errorf("Test(%q) = true, want false", path)
		}
		if n := counter.count(path); n != 0 {
			t.Errorf("Test(%q) made %d requests, want 0", path, n)
		}
	}

	r := NewRSCComic(&Grabber{URL: srv.URL + rscComicSeriesPath})
	if got, _ := r.Test(); got {
		t.Error("domain-only Test() = true on an unlisted host, want false")
	}
	if n := counter.count(rscComicSeriesPath); n != 0 {
		t.Errorf("domain-only Test() made %d requests, want 0", n)
	}
}
