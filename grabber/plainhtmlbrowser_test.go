// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMatchBrowserSelector(t *testing.T) {
	cases := []struct {
		host      string
		wantMatch bool
	}{
		{"toongod.org", true},
		{"www.toongod.org", true}, // www. prefix is stripped
		{"dragontea.ink", true},
		{"kappabeast.com", true},
		{"sushiscan.net", true},
		{"drakecomic.org", false},  // handled by the Drakecomic grabber
		{"mangakakalot.gg", false}, // handled by the Manganelo grabber
		{"natomanga.com", false},   // handled by the Manganelo grabber
		{"manhuaus.com", true},
		{"toonily.com", true},
		{"www.toonily.com", true}, // www. prefix is stripped
		{"mangahub.io", true},
		{"www.mangahub.io", true}, // www. prefix is stripped
		{"manhuatop.org", true},
		{"www.manhuatop.org", true}, // www. prefix is stripped
		{"setsuscans.com", true},
		{"www.setsuscans.com", true}, // www. prefix is stripped
		{"example.com", false},
		{"notatoongod.org", false}, // must be an exact host match
	}

	for _, c := range cases {
		_, ok := matchBrowserSelector(c.host)
		if ok != c.wantMatch {
			t.Errorf("matchBrowserSelector(%q) matched = %v, want %v", c.host, ok, c.wantMatch)
		}
	}
}

// TestBrowserSelectorsAreComplete guards against a selector entry missing the
// fields the grabber relies on.
func TestBrowserSelectorsAreComplete(t *testing.T) {
	for _, s := range browserSelectors {
		if len(s.Domains) == 0 {
			t.Error("a browser selector has no domains")
		}
		if s.Title == "" || s.Rows == "" || s.Image == "" {
			t.Errorf("browser selector for %v is missing a required selector", s.Domains)
		}
		if s.ChaptersWait == "" || s.ImageWait == "" {
			t.Errorf("browser selector for %v is missing a wait selector", s.Domains)
		}
	}
}

func TestBrowserPathSeriesURL(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		depth int
		want  string
	}{
		// chapter URL resolved to series URL
		{"toonily chapter", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6/chapter-244/", 2, "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6"},
		// series URL with trailing slash — already at depth, no change
		{"toonily series trailing slash", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6/", 2, ""},
		// series URL without trailing slash — already at depth, no change
		{"toonily series", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6", 2, ""},
		// www. prefix is preserved in the result
		{"toongod www chapter", "https://www.toongod.org/webtoon/solo-leveling/chapter-200/", 2, "https://www.toongod.org/webtoon/solo-leveling"},
		// kappabeast reader adds two extra segments
		{"kappabeast chapter", "https://kappabeast.com/series/tekkarian/reader/chapter-2", 2, "https://kappabeast.com/series/tekkarian"},
		// depth 0 disables reconstruction
		{"depth 0 disabled", "https://toonily.com/serie/slug/chapter-1/", 0, ""},
		// query and fragment are stripped
		{"query stripped", "https://sushiscan.net/catalogue/mushoku-tensei/chapter-17/?style=list", 2, "https://sushiscan.net/catalogue/mushoku-tensei"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &PlainHTMLBrowser{
				PlainHTML: NewPlainHTML(&Grabber{URL: c.url}),
				selector:  BrowserSiteSelector{SeriesPathDepth: c.depth},
			}
			got := m.pathSeriesURL()
			if got != c.want {
				t.Errorf("pathSeriesURL() = %q, want %q", got, c.want)
			}
		})
	}
}
