// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangalibSeriesSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://mangalib.me/ru/manga/206--one-piece", "206--one-piece", false},
		{"https://mangalib.me/manga/206--one-piece", "206--one-piece", false},
		{"https://mangalib.me/manga/one-piece", "one-piece", false},
		{"https://mangalib.me/ru/manga/206--one-piece?query=1", "206--one-piece", false},
		{"https://mangalib.me/ru/manga/", "", true},
		{"https://mangalib.me/", "", true},
	}

	for _, c := range cases {
		m := Mangalib{Grabber: &Grabber{URL: c.url}}
		got, err := m.seriesSlug()
		if (err != nil) != c.wantErr {
			t.Errorf("seriesSlug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestMangalibTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://mangalib.me/ru/manga/206--one-piece", true},
		{"https://mangalib.me/manga/one-piece", true},
		{"https://mangadex.org/title/xyz", false},
	}

	for _, c := range cases {
		m := &Mangalib{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
