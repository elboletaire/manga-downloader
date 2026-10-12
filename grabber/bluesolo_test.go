// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestBluesoloResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://bluesolo.org/read/frieren/fr/ch/147", "https://bluesolo.org/comics/frieren"},
		{"https://www.bluesolo.org/read/frieren/fr/ch/147", "https://www.bluesolo.org/comics/frieren"},
		{"https://bluesolo.org/read/frieren/fr/ch/147?foo=1", "https://bluesolo.org/comics/frieren"},
		{"https://BLUESOLO.ORG/read/frieren/fr/ch/147", "https://BLUESOLO.ORG/comics/frieren"},
		// decimal chapter
		{"https://bluesolo.org/read/frieren/fr/ch/147.5/", "https://bluesolo.org/comics/frieren"},
		// series URLs unchanged
		{"https://bluesolo.org/comics/frieren", "https://bluesolo.org/comics/frieren"},
		// unrelated paths unchanged
		{"https://bluesolo.org/browse", "https://bluesolo.org/browse"},
	}

	for _, c := range cases {
		b := &Bluesolo{Grabber: &Grabber{URL: c.input}}
		b.resolveSeriesURL()
		if b.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, b.URL, c.want)
		}
	}
}
