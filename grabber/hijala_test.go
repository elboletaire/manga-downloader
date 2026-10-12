// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestHijalaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://en-hijala.com/series/double-click", true},
		{"https://EN-HIJALA.COM/series/double-click", true},
		{"https://En-Hijala.Com/series/double-click", true},
		{"https://www.en-hijala.com/series/double-click", true},
		{"https://othermanga.com/series/double-click", false},
	}
	for _, c := range cases {
		h := NewHijala(&Grabber{URL: c.url})
		got, _ := h.Test()
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

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
		// query and fragment on chapter URL: stripped
		{"https://en-hijala.com/series/double-click/chapter-248?x=1", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/chapter-1#page2", "https://en-hijala.com/series/double-click"},
		// www prefix, query, fragment on series URL: unchanged (no chapter suffix)
		{"https://www.en-hijala.com/series/double-click?foo=bar", "https://www.en-hijala.com/series/double-click?foo=bar"},
		// uppercase host: chapter stripped
		{"https://EN-HIJALA.COM/series/double-click/chapter-248", "https://EN-HIJALA.COM/series/double-click"},
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
