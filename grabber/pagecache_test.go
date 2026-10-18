// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"testing"
)

// unknownPage has no platform markers and no links, so no grabber matches it
const unknownPage = `<html><head><title>Nothing here</title></head><body><p>No series here.</p></body></html>`

// TestIdentifySiteFetchesPageOnce checks that the grabbers identifying a page by
// its markup share one GET of it, whichever of them the URL's shape lets through
func TestIdentifySiteFetchesPageOnce(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"unknown series", "/unknown/series/"},
		{"gigaviewer episode", "/episode/1"},
		{"comici episode", "/episodes/c1ffef58c01d8"},
		{"blogger post", "/2025/10/one-piece-1-espanol.html"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, hits := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, unknownPage)
			})

			if site, _ := NewSite(srv.URL+c.path, &Settings{}); site != nil {
				t.Fatalf("IdentifySite() = %T, want nil", site)
			}
			if n := hits.count(c.path); n != 1 {
				t.Errorf("GET %s made %d requests, want 1", c.path, n)
			}
		})
	}
}

// TestIdentifySiteReportsFetchErrorOnce checks that a page failing to load is
// requested once, and its error reported once, not by every grabber that reads it
func TestIdentifySiteReportsFetchErrorOnce(t *testing.T) {
	const path = "/unknown/series/"

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv, hits := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", status)
			})

			site, errs := NewSite(srv.URL+path, &Settings{})
			if site != nil {
				t.Fatalf("IdentifySite() = %T, want nil", site)
			}
			if n := hits.count(path); n != 1 {
				t.Errorf("GET %s made %d requests, want 1", path, n)
			}
			if len(errs) != 1 {
				t.Errorf("IdentifySite() errs = %v, want exactly one", errs)
			}
		})
	}
}

// TestIdentifyPageKeysByURL checks that a URL changed after a fetch, as a Test()
// rewrites it, gets its own entry, while the URL as first requested stays cached
func TestIdentifyPageKeysByURL(t *testing.T) {
	srv, hits := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, unknownPage)
	})

	g := &Grabber{URL: srv.URL + "/old/series/", Settings: &Settings{}}
	g.pages = newPageCache()

	for _, u := range []string{"/old/series/", "/new/series/", "/old/series/"} {
		g.URL = srv.URL + u
		if _, err := g.identifyPage(g.URL); err != nil {
			t.Fatalf("identifyPage(%s) error = %v", u, err)
		}
	}

	if n := hits.count("/old/series/"); n != 1 {
		t.Errorf("GET /old/series/ made %d requests, want 1", n)
	}
	if n := hits.count("/new/series/"); n != 1 {
		t.Errorf("GET /new/series/ made %d requests, want 1", n)
	}
}

// TestIdentifySiteDropsPageCache checks that the page cache is gone once
// IdentifySite returns, whether a grabber matched the page or none did, so the
// matched site doesn't keep the page
func TestIdentifySiteDropsPageCache(t *testing.T) {
	unknown, _ := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, unknownPage)
	})
	themesia := newIdentifyServer(t, "https://example.com", map[string]string{
		"/manga/x/": "themesia_series.html",
	})

	cases := []struct {
		name      string
		url       string
		wantMatch bool
	}{
		{"unmatched", unknown.URL + "/unknown/series/", false},
		{"matched", themesia.URL + "/manga/x/", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := &Grabber{URL: c.url, Settings: &Settings{}}
			site, _ := g.IdentifySite()

			if (site != nil) != c.wantMatch {
				t.Fatalf("IdentifySite() = %T, want a match: %v", site, c.wantMatch)
			}
			if g.pages != nil {
				t.Errorf("pages = %v after IdentifySite, want nil", g.pages)
			}
		})
	}
}

// TestIdentifyPageTruncatedBody checks that a body the connection cut short is
// cached as an error, never returned as the page, and that it's fetched once
func TestIdentifyPageTruncatedBody(t *testing.T) {
	const path = "/unknown/series/"
	srv, hits := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(unknownPage)))
		fmt.Fprint(w, unknownPage[:20])
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})

	g := &Grabber{URL: srv.URL + path, Settings: &Settings{}}
	g.pages = newPageCache()
	for range 2 {
		body, err := g.identifyPage(g.URL)
		if err == nil {
			t.Fatal("identifyPage() error = nil for a truncated body, want an error")
		}
		if body != "" {
			t.Errorf("identifyPage() body = %q, want none", body)
		}
	}

	if n := hits.count(path); n != 1 {
		t.Errorf("GET %s made %d requests, want 1", path, n)
	}
}

// TestIdentifyPageWithoutCacheFetches checks that a grabber built outside
// IdentifySite, with no cache, fetches the page on every call
func TestIdentifyPageWithoutCacheFetches(t *testing.T) {
	const path = "/unknown/series/"
	srv, hits := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, unknownPage)
	})

	g := &Grabber{URL: srv.URL + path, Settings: &Settings{}}
	for range 2 {
		if _, err := g.identifyPage(g.URL); err != nil {
			t.Fatalf("identifyPage() error = %v", err)
		}
	}

	if n := hits.count(path); n != 2 {
		t.Errorf("GET %s made %d requests, want 2", path, n)
	}
}
