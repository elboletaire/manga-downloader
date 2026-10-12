// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestHijalaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// chapter URLs: strip the /chapter-N suffix
		{"https://en-hijala.com/series/double-click/chapter-248", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/my-series/chapter-1", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5/", "https://en-hijala.com/series/my-series"},
		// series URLs: unchanged
		{"https://en-hijala.com/series/double-click", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/", "https://en-hijala.com/series/double-click/"},
		// www prefix, query, fragment: unchanged (no chapter suffix)
		{"https://www.en-hijala.com/series/double-click?foo=bar", "https://www.en-hijala.com/series/double-click?foo=bar"},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			h := &Hijala{Grabber: &Grabber{URL: c.in}}
			h.resolveSeriesURL()
			if h.URL != c.want {
				t.Errorf("resolveSeriesURL(%q) URL = %q, want %q", c.in, h.URL, c.want)
			}
		})
	}
}
