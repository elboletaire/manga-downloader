// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestElftoonTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string // rewritten URL after Test(); empty means unchanged
	}{
		{"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow", true, ""},
		{"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow/chapter/123", true, ""},
		// the old domain 301s /manga/{slug}/ on to elftoon.net/series/comic/{slug};
		// Test() applies the same rewrite in code
		{
			"https://elftoon.com/manga/god-level-assassin-im-the-shadow/",
			true,
			"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow/",
		},
		{
			"http://elftoon.com/manga/god-level-assassin-im-the-shadow",
			true,
			"https://elftoon.net/series/comic/god-level-assassin-im-the-shadow",
		},
		{"https://elftoon.com/", true, "https://elftoon.net/"},
		{"https://example.com/series/comic/whatever", false, ""},
	}

	for _, c := range cases {
		e := NewElftoon(&Grabber{URL: c.url})
		got, err := e.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
			continue
		}
		wantURL := c.wantURL
		if wantURL == "" {
			wantURL = c.url
		}
		if e.URL != wantURL {
			t.Errorf("Test(%q) rewrote URL to %q, want %q", c.url, e.URL, wantURL)
		}
	}
}
