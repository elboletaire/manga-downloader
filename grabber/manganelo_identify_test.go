// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	mhttp "github.com/elboletaire/manga-downloader/http"
)

// manganeloIdentifyServer serves the identify fixtures routed by path, counting
// the requests each path receives
func manganeloIdentifyServer(t *testing.T, routes map[string]string) (*requestCounter, string) {
	t.Helper()

	handler := &identifyServer{origin: "https://www.mangabats.com", routes: routes}
	srv, counter := newCountingServer(t, handler.ServeHTTP)
	handler.base = srv.URL

	return counter, srv.URL
}

// TestManganeloTestByMarkup checks that a host outside the list is matched by
// its series or reader page's chapters API marker, with one fetch, and that a
// page without the marker or a URL outside /manga/{slug} is refused
func TestManganeloTestByMarkup(t *testing.T) {
	counter, base := manganeloIdentifyServer(t, map[string]string{
		"/manga/apotheosis":              "manganelo_series.html",
		"/manga/apotheosis/chapter-1297": "manganelo_chapter.html",
		"/manga/themesia":                "themesia_series.html",
		"/manga/madara":                  "madara_series.html",
		"/manga/unrelated":               "unrelated_page.html",
		"/comic/apotheosis":              "manganelo_series.html",
	})

	cases := []struct {
		path     string
		want     bool
		wantHits int
	}{
		{"/manga/apotheosis", true, 1},
		{"/manga/apotheosis/chapter-1297", true, 1},
		{"/manga/themesia", false, 1},
		{"/manga/madara", false, 1},
		{"/manga/unrelated", false, 1},
		// another URL shape is never fetched
		{"/comic/apotheosis", false, 0},
		{"/", false, 0},
	}
	for _, c := range cases {
		got, err := NewManganeloByMarkup(&Grabber{URL: base + c.path, Settings: &Settings{}}).Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v; want %v, nil", c.path, got, err, c.want)
		}
		if hits := counter.count(c.path); hits != c.wantHits {
			t.Errorf("Test(%q) made %d requests, want %d", c.path, hits, c.wantHits)
		}
	}
}

// TestManganeloIdentifySiteByMarkup checks that IdentifySite hands a clone's
// series page to Manganelo with a single GET of that page
func TestManganeloIdentifySiteByMarkup(t *testing.T) {
	counter, base := manganeloIdentifyServer(t, map[string]string{
		"/manga/apotheosis": "manganelo_series.html",
	})

	g := &Grabber{URL: base + "/manga/apotheosis", Settings: &Settings{}}
	site, errs := g.IdentifySite()
	if _, ok := site.(*Manganelo); !ok {
		t.Fatalf("IdentifySite() = %T, want *Manganelo (errs %v)", site, errs)
	}
	if hits := counter.count("/manga/apotheosis"); hits != 1 {
		t.Errorf("IdentifySite() fetched the page %d times, want 1", hits)
	}
}

// manganeloNoNetwork counts the requests Test would send, and fails them
type manganeloNoNetwork struct{ hits atomic.Int32 }

func (n *manganeloNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.hits.Add(1)
	return nil, errors.New("no network in tests")
}

// TestManganeloTestListedHostNoRequest checks that the listed Cloudflare hosts
// match by domain without a request, from both instances, since the browser
// hosts must never be fetched to identify them
func TestManganeloTestListedHostNoRequest(t *testing.T) {
	counter := &manganeloNoNetwork{}
	mhttp.SetRoundTripper(counter)
	defer mhttp.SetRoundTripper(nil)

	for _, u := range []string{
		"https://www.mangakakalot.gg/manga/apotheosis",
		"https://www.natomanga.com/manga/rebirth-from-0-to-1",
		"https://www.manganato.gg/manga/apotheosis",
		"https://www.mangabats.com/manga/apotheosis",
	} {
		for _, m := range []*Manganelo{NewManganelo(&Grabber{URL: u}), NewManganeloByMarkup(&Grabber{URL: u})} {
			got, err := m.Test()
			if err != nil || !got {
				t.Errorf("Test(%q) = %v, %v; want true, nil", u, got, err)
			}
		}
	}
	if n := counter.hits.Load(); n != 0 {
		t.Errorf("Test made %d requests for listed hosts, want 0", n)
	}
}
