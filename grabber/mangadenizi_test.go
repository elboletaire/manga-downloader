// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangadeniziResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://mangadenizi.net/read/one-piece/1193", "https://mangadenizi.net/manga/one-piece"},
		{"https://www.mangadenizi.net/read/one-piece/1193", "https://www.mangadenizi.net/manga/one-piece"},
		{"https://mangadenizi.net/read/one-piece/1193?foo=1", "https://mangadenizi.net/manga/one-piece"},
		{"https://MANGADENIZI.NET/read/one-piece/1193", "https://MANGADENIZI.NET/manga/one-piece"},
		// decimal chapter number
		{"https://mangadenizi.net/read/one-piece/1193.5/", "https://mangadenizi.net/manga/one-piece"},
		// series URLs unchanged
		{"https://mangadenizi.net/manga/one-piece", "https://mangadenizi.net/manga/one-piece"},
		// unrelated paths unchanged
		{"https://mangadenizi.net/search", "https://mangadenizi.net/search"},
	}

	for _, c := range cases {
		m := &Mangadenizi{Grabber: &Grabber{URL: c.input}}
		m.resolveSeriesURL()
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
}
