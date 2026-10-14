// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// identifyFixtureDir holds the trimmed pages the identification tests serve.
const identifyFixtureDir = "testdata/identify"

// identifyServer serves fixture files by path. Each fixture is captured from a
// real site, so its links point at that site's origin; ServeHTTP swaps the
// origin for the test server, which keeps every follow-up request offline.
type identifyServer struct {
	// origin is the real site's origin as written in the fixtures
	origin string
	// base is the test server's URL, set once it is started
	base string
	// routes maps a request path to the fixture file that answers it
	routes map[string]string
}

func (s *identifyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	file, ok := s.routes[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := os.ReadFile(filepath.Join(identifyFixtureDir, file))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(file, ".json") {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write([]byte(strings.ReplaceAll(string(body), s.origin, s.base)))
}

// newIdentifyServer starts a test server answering routes from origin's fixtures.
func newIdentifyServer(t *testing.T, origin string, routes map[string]string) *httptest.Server {
	t.Helper()

	handler := &identifyServer{origin: origin, routes: routes}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	handler.base = srv.URL

	return srv
}

// TestIdentifySiteOrder checks which grabber IdentifySite hands each kind of
// page to. The domain-matched grabbers can't run against 127.0.0.1, so these
// cases cover the markup-matched ones and the order they're tried in.
func TestIdentifySiteOrder(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		// path is the page to identify, relative to the test server
		path   string
		routes map[string]string
		// want is a typed nil of the grabber expected to win, or nil for none
		want Site
	}{
		{
			name:   "themesia series page",
			origin: "https://kingofshojo.com",
			path:   "/manga/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing/",
			routes: map[string]string{
				"/manga/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing/": "themesia_series.html",
			},
			want: (*PlainHTML)(nil),
		},
		{
			// The chapter page is a WordPress post with a posts REST link, so
			// WordPressCategory would claim it if it ran before the fallback.
			// PlainHTML only wins by resolving the chapter to its series.
			name:   "themesia chapter page",
			origin: "https://kingofshojo.com",
			path:   "/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing-chapter-28/",
			routes: map[string]string{
				"/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing-chapter-28/": "themesia_chapter.html",
				"/wp-json/wp/v2/posts/464264": "themesia_chapter_rest.json",
				"/manga/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing/": "themesia_series.html",
			},
			want: (*PlainHTML)(nil),
		},
		{
			name:   "madara series page",
			origin: "https://mangasushi.org",
			path:   "/manga/lonely-attack-on-the-different-world/",
			routes: map[string]string{
				"/manga/lonely-attack-on-the-different-world/":              "madara_series.html",
				"/manga/lonely-attack-on-the-different-world/ajax/chapters": "madara_chapters.html",
			},
			want: (*Tcb)(nil),
		},
		{
			name:   "wordpress category archive",
			origin: "https://animeallstar30.com",
			path:   "/category/one-piece-espanol/",
			routes: map[string]string{
				"/category/one-piece-espanol/": "wp_category.html",
			},
			want: (*WordPressCategory)(nil),
		},
		{
			name:   "wordpress chapter post",
			origin: "https://animeallstar30.com",
			path:   "/one-piece-1180-esp/",
			routes: map[string]string{
				"/one-piece-1180-esp/":      "wp_chapter.html",
				"/wp-json/wp/v2/posts/4727": "wp_chapter_rest.json",
			},
			want: (*WordPressCategory)(nil),
		},
		{
			name:   "blogger search page",
			origin: "https://www.animeallstar1.com",
			path:   "/search?q=One+Piece",
			routes: map[string]string{
				"/search": "blogger_search.html",
			},
			want: (*Blogger)(nil),
		},
		{
			name:   "blogger chapter post",
			origin: "https://www.animeallstar1.com",
			path:   "/2025/10/one-piece-manga-1164-espanol.html",
			routes: map[string]string{
				"/2025/10/one-piece-manga-1164-espanol.html": "blogger_post.html",
			},
			want: (*Blogger)(nil),
		},
		{
			// Documents current behaviour: a WordPress post with a REST link
			// and a category is claimed by WordPressCategory, since nothing
			// tells a chapter post from any other post in a category.
			name:   "wordpress blog post, not manga",
			origin: "https://wordpress.org/news",
			path:   "/2026/10/wordpress-7-1-3-maintenance-and-security-release/",
			routes: map[string]string{
				"/2026/10/wordpress-7-1-3-maintenance-and-security-release/": "wp_blog_post.html",
				"/wp-json/wp/v2/posts/21753":                                 "wp_blog_post_rest.json",
			},
			want: (*WordPressCategory)(nil),
		},
		{
			// Documents current behaviour: any Blogger post is claimed by the
			// Blogger grabber, whatever its topic.
			name:   "blogger post, not manga",
			origin: "https://android-developers.googleblog.com",
			path:   "/2026/10/android-cli-device-streaming-and-skills.html",
			routes: map[string]string{
				"/2026/10/android-cli-device-streaming-and-skills.html": "blogger_nonmanga_post.html",
			},
			want: (*Blogger)(nil),
		},
		{
			name:   "page without platform markers",
			origin: "https://example.com",
			path:   "/",
			routes: map[string]string{
				"/": "unrelated_page.html",
			},
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newIdentifyServer(t, c.origin, c.routes)

			site, errs := NewSite(srv.URL+c.path, &Settings{})
			for _, err := range errs {
				t.Logf("IdentifySite() err = %v", err)
			}

			if c.want == nil {
				if site != nil {
					t.Fatalf("IdentifySite() = %T, want nil", site)
				}
				return
			}
			if site == nil {
				t.Fatalf("IdentifySite() = nil, want %T", c.want)
			}
			if got, want := reflect.TypeOf(site), reflect.TypeOf(c.want); got != want {
				t.Fatalf("IdentifySite() = %v, want %v", got, want)
			}
		})
	}
}
