// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangataroResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// reader URL → series URL
		{"https://mangataro.org/read/one-piece/ch001-50", "https://mangataro.org/manga/one-piece"},
		{"https://www.mangataro.org/read/one-piece/ch001-50", "https://www.mangataro.org/manga/one-piece"},
		{"https://mangataro.org/read/one-piece/ch001-50?foo=bar", "https://mangataro.org/manga/one-piece"},
		{"https://MANGATARO.ORG/read/one-piece/ch001-50", "https://MANGATARO.ORG/manga/one-piece"},
		// decimal chapter number
		{"https://mangataro.org/read/some-manga/ch10.5-999", "https://mangataro.org/manga/some-manga"},
		// series URLs unchanged
		{"https://mangataro.org/manga/one-piece", "https://mangataro.org/manga/one-piece"},
		// unrelated paths unchanged
		{"https://mangataro.org/browse", "https://mangataro.org/browse"},
	}

	for _, c := range cases {
		m := &Mangataro{Grabber: &Grabber{URL: c.input}}
		m.resolveSeriesURL()
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
}
