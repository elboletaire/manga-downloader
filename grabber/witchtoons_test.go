// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestWitchtoonsTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://witchtoons.net/series/comic/some-series", true},
		{"https://witchtoons.net/series/comic/some-series/chapter/12", true},
		// the old domain 301s to witchtoons.net dropping the path, so its
		// URLs can't be mapped to the new /series/comic/{slug} shape
		{"https://witchscans.com/manga/some-series/", false},
		{"https://example.com/series/comic/some-series", false},
	}

	for _, c := range cases {
		w := NewWitchtoons(&Grabber{URL: c.url})
		got, err := w.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestFormatChapterNumber(t *testing.T) {
	cases := []struct {
		number float64
		want   string
	}{
		{61, "61"},
		{61.5, "61.5"},
		{61.25, "61.25"},
		{0, "0"},
	}

	for _, c := range cases {
		if got := formatChapterNumber(c.number); got != c.want {
			t.Errorf("formatChapterNumber(%v) = %q, want %q", c.number, got, c.want)
		}
	}
}
