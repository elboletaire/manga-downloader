// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestRoliascanResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// reader URL → series URL
		{"https://roliascan.com/read/no-marriage/ch1-276741/", "https://roliascan.com/manga/no-marriage/"},
		{"https://www.roliascan.com/read/no-marriage/ch1-276741/", "https://www.roliascan.com/manga/no-marriage/"},
		{"https://roliascan.com/read/no-marriage/ch1-276741/?foo=bar", "https://roliascan.com/manga/no-marriage/"},
		{"https://ROLIASCAN.COM/read/no-marriage/ch1-276741/", "https://ROLIASCAN.COM/manga/no-marriage/"},
		// decimal chapter number
		{"https://roliascan.com/read/some-manga/ch10.5-99999/", "https://roliascan.com/manga/some-manga/"},
		// series URLs unchanged
		{"https://roliascan.com/manga/no-marriage/", "https://roliascan.com/manga/no-marriage/"},
		// unrelated paths unchanged
		{"https://roliascan.com/browse/", "https://roliascan.com/browse/"},
	}

	for _, c := range cases {
		m := &Roliascan{Grabber: &Grabber{URL: c.input}}
		m.resolveSeriesURL()
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
}
