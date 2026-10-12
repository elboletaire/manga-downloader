// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestFlamecomicsResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://flamecomics.xyz/series/154/dac3a44c48a764ca", "https://flamecomics.xyz/series/154"},
		{"https://flamecomics.xyz/series/154/dac3a44c48a764ca/", "https://flamecomics.xyz/series/154"},
		{"https://flamecomics.xyz/series/154/dac3a44c48a764ca?foo=1", "https://flamecomics.xyz/series/154"},
		{"https://flamecomics.xyz/series/154", "https://flamecomics.xyz/series/154"},
		{"https://flamecomics.xyz/series/154/", "https://flamecomics.xyz/series/154/"},
		{"https://flamecomics.xyz/browse", "https://flamecomics.xyz/browse"},
	}

	for _, c := range cases {
		f := &Flamecomics{Grabber: &Grabber{URL: c.input}}
		f.resolveSeriesURL()
		if f.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, f.URL, c.want)
		}
	}
}
