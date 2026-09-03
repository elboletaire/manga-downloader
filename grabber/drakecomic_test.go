// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestDrakecomicTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string // rewritten URL after Test(); empty means unchanged
	}{
		{"https://drakecomic.net/series/comic/beast-evolution", true, ""},
		{"https://drakecomic.net/series/comic/beast-evolution/chapter/70", true, ""},
		// the old domain 301s /manga/{slug}/ to
		// drakecomic.net/series/comic/{slug}/; Test() applies the same
		// rewrite in code
		{
			"https://drakecomic.org/manga/beast-evolution/",
			true,
			"https://drakecomic.net/series/comic/beast-evolution/",
		},
		{
			"http://drakecomic.org/manga/beast-evolution",
			true,
			"https://drakecomic.net/series/comic/beast-evolution",
		},
		{"https://drakecomic.org/", true, "https://drakecomic.net/"},
		{"https://example.com/series/comic/whatever", false, ""},
	}

	for _, c := range cases {
		d := NewDrakecomic(&Grabber{URL: c.url})
		got, err := d.Test()
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
		if d.URL != wantURL {
			t.Errorf("Test(%q) rewrote URL to %q, want %q", c.url, d.URL, wantURL)
		}
	}
}
