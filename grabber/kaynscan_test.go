// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestKaynscanTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string // rewritten URL after Test(); empty means unchanged
	}{
		{"https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation", true, ""},
		{"https://kaynscans.com/series/comic/colorist/chapter/45", true, ""},
		// the old domain 301s to kaynscans.com remapping /series/{slug} to
		// /series/comic/{slug}; Test() applies the same rewrite in code
		{
			"https://kaynscan.org/series/heavenly-demon-cultivation-simulation",
			true,
			"https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation",
		},
		// an old URL already carrying the new path shape isn't doubled up
		{
			"http://kaynscan.org/series/comic/colorist",
			true,
			"https://kaynscans.com/series/comic/colorist",
		},
		{"https://kaynscan.org/", true, "https://kaynscans.com/"},
		{"https://example.com/series/comic/whatever", false, ""},
	}

	for _, c := range cases {
		k := NewKaynscan(&Grabber{URL: c.url})
		got, err := k.Test()
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
		if k.URL != wantURL {
			t.Errorf("Test(%q) rewrote URL to %q, want %q", c.url, k.URL, wantURL)
		}
	}
}
