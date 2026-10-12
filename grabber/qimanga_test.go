// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestQimangaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://qimanga.com/series/4190634673-eleceed/chapter-417", "https://qimanga.com/series/4190634673-eleceed"},
		{"https://www.qimanga.com/series/4190634673-eleceed/chapter-417", "https://www.qimanga.com/series/4190634673-eleceed"},
		{"https://qimanga.com/series/4190634673-eleceed/chapter-417?foo=1", "https://qimanga.com/series/4190634673-eleceed"},
		{"https://QIMANGA.COM/series/4190634673-eleceed/chapter-417", "https://QIMANGA.COM/series/4190634673-eleceed"},
		// rebrand aliases
		{"https://aurorascans.com/series/4190634673-eleceed/chapter-417", "https://aurorascans.com/series/4190634673-eleceed"},
		{"https://qimanhwa.com/series/4190634673-eleceed/chapter-417", "https://qimanhwa.com/series/4190634673-eleceed"},
		// decimal chapter number
		{"https://qimanga.com/series/4190634673-eleceed/chapter-417.5/", "https://qimanga.com/series/4190634673-eleceed"},
		// series URLs unchanged
		{"https://qimanga.com/series/4190634673-eleceed", "https://qimanga.com/series/4190634673-eleceed"},
		{"https://qimanga.com/series/4190634673-eleceed/", "https://qimanga.com/series/4190634673-eleceed/"},
		// unrelated paths unchanged
		{"https://qimanga.com/browse", "https://qimanga.com/browse"},
	}

	for _, c := range cases {
		q := &Qimanga{Grabber: &Grabber{URL: c.input}}
		q.resolveSeriesURL()
		if q.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, q.URL, c.want)
		}
	}
}
