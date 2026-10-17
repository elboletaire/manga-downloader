// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	mhttp "github.com/elboletaire/manga-downloader/http"
)

// toivoLiteOrigin is the origin the Toivo Lite fixtures were captured from
const toivoLiteOrigin = "https://w9.jojolandsmanga.com"

// TestToivoLiteTestByMarkup checks that a Toivo Lite page on an unlisted host
// is matched with one GET, on the root or a post, and that the other WordPress
// and Madara pages are refused
func TestToivoLiteTestByMarkup(t *testing.T) {
	cases := []struct {
		fixture string
		want    bool
	}{
		{"toivolite_home.html", true},
		{"madara_series.html", false},
		{"themesia_series.html", false},
		{"wp_blog_post.html", false},
		{"wp_category.html", false},
		{"unrelated_page.html", false},
	}

	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			base, hits := newFixtureServer(t, toivoLiteOrigin, map[string]string{"/": c.fixture})

			got, err := NewToivoLiteByMarkup(&Grabber{URL: base + "/", Settings: &Settings{}}).Test()
			if err != nil || got != c.want {
				t.Errorf("Test() = %v, %v; want %v, nil", got, err, c.want)
			}
			if n := hits.count("/"); n != 1 {
				t.Errorf("GET / made %d requests, want 1", n)
			}
		})
	}
}

// TestToivoLiteIdentifySiteOneGet checks that a Toivo Lite site on an unlisted
// host is identified with a single GET, the one the markup check makes and
// PlainHTML would have made too
func TestToivoLiteIdentifySiteOneGet(t *testing.T) {
	routes := map[string]string{
		"/":                           "toivolite_home.html",
		"/manga/jojolands-chapter-1/": "toivolite_home.html",
	}
	base, hits := newFixtureServer(t, toivoLiteOrigin, routes)

	for path := range routes {
		t.Run(path, func(t *testing.T) {
			site, errs := NewSite(base+path, &Settings{})
			if _, ok := site.(*ToivoLite); !ok {
				t.Fatalf("IdentifySite() = %T, want *ToivoLite (errs %v)", site, errs)
			}
			if n := hits.count(path); n != 1 {
				t.Errorf("GET %s made %d requests, want 1", path, n)
			}
		})
	}
}

// TestToivoLiteDomainOnlyNoRequest checks that the listed-host instance never
// fetches an unlisted host, so the fast path stays request-free
func TestToivoLiteDomainOnlyNoRequest(t *testing.T) {
	base, hits := newFixtureServer(t, toivoLiteOrigin, map[string]string{"/": "toivolite_home.html"})

	got, err := NewToivoLite(&Grabber{URL: base + "/", Settings: &Settings{}}).Test()
	if err != nil || got {
		t.Errorf("Test() = %v, %v; want false, nil", got, err)
	}
	if n := hits.count("/"); n != 0 {
		t.Errorf("GET / made %d requests, want 0", n)
	}
}

// fixtureRoundTripper answers every request with one body, counting them
type fixtureRoundTripper struct {
	body string
	hits atomic.Int32
}

func (f *fixtureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.hits.Add(1)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(f.body)), Header: http.Header{}, Request: req}, nil
}

// TestHeavenlyDelusionWinsOverToivoLiteMarkup checks that heavenly-delusion.com
// stays on HeavenlyDelusion: its page has the Toivo Lite markup, and the
// markup instance would match it with a GET if it ran first
func TestHeavenlyDelusionWinsOverToivoLiteMarkup(t *testing.T) {
	page, err := os.ReadFile(filepath.Join(identifyFixtureDir, "toivolite_home.html"))
	if err != nil {
		t.Fatal(err)
	}
	rt := &fixtureRoundTripper{body: string(page)}
	mhttp.SetRoundTripper(rt)
	defer mhttp.SetRoundTripper(nil)

	site, _ := NewSite("https://heavenly-delusion.com/", &Settings{})
	if _, ok := site.(*HeavenlyDelusion); !ok {
		t.Errorf("IdentifySite() = %T, want *HeavenlyDelusion", site)
	}
	if n := rt.hits.Load(); n != 0 {
		t.Errorf("identifying heavenly-delusion.com made %d requests, want 0", n)
	}
}

// newFixtureServer serves routes from origin's fixtures and counts the requests
// per path, returning the server's URL
func newFixtureServer(t *testing.T, origin string, routes map[string]string) (string, *requestCounter) {
	t.Helper()

	ident := &identifyServer{origin: origin, routes: routes}
	srv, hits := newCountingServer(t, ident.ServeHTTP)
	ident.base = srv.URL

	return srv.URL, hits
}
