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
		{"mangakakalot.gg", true},
		{"www.mangakakalot.gg", true},
		{"natomanga.com", true},
		{"www.natomanga.com", true}, // www. prefix is stripped
		{"manhuaus.com", true},
		{"mangahub.io", true},
		{"www.mangahub.io", true}, // www. prefix is stripped
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
