// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMkissaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "series URL with trailing slash unchanged",
			input:  "https://mkissa.to/manga/eaYBWW65WLabNiLEi/",
			wanted: "https://mkissa.to/manga/eaYBWW65WLabNiLEi/",
		},
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://mkissa.to/manga/eaYBWW65WLabNiLEi/chapter-59-sub",
			wanted: "https://mkissa.to/manga/eaYBWW65WLabNiLEi",
		},
		{
			name:   "chapter-0 URL resolves to series URL",
			input:  "https://mkissa.to/manga/eaYBWW65WLabNiLEi/chapter-0-sub",
			wanted: "https://mkissa.to/manga/eaYBWW65WLabNiLEi",
		},
		{
			name:   "www. prefix preserved",
			input:  "https://www.mkissa.to/manga/eaYBWW65WLabNiLEi/chapter-59-sub",
			wanted: "https://www.mkissa.to/manga/eaYBWW65WLabNiLEi",
		},
		{
			name:   "series URL unchanged",
			input:  "https://mkissa.to/manga/eaYBWW65WLabNiLEi",
			wanted: "https://mkissa.to/manga/eaYBWW65WLabNiLEi",
		},
		{
			name:   "series URL with query unchanged",
			input:  "https://mkissa.to/manga/eaYBWW65WLabNiLEi?lang=en",
			wanted: "https://mkissa.to/manga/eaYBWW65WLabNiLEi?lang=en",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://mkissa.to/",
			wanted: "https://mkissa.to/",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Mkissa{Grabber: &Grabber{URL: c.input}}
			m.resolveSeriesURL()
			if m.URL != c.wanted {
				t.Errorf("got %q, want %q", m.URL, c.wanted)
			}
		})
	}
}
