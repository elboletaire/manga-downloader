// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangahereChapterURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		// standard chapter (no volume)
		{"https://www.mangahere.cc/manga/kengan_omega/c369/1.html", true},
		// with volume segment
		{"https://www.mangahere.cc/manga/one_piece/v98/c1195/1.html", true},
		// decimal chapter number
		{"https://www.mangahere.cc/manga/one_piece/v00/c001.1/1.html", true},
		// without www.
		{"https://mangahere.cc/manga/kengan_omega/c369/1.html", true},
		// series URLs — must return false
		{"https://www.mangahere.cc/manga/kengan_omega/", false},
		{"https://www.mangahere.cc/manga/kengan_omega", false},
		// path without /manga/ prefix
		{"https://www.mangahere.cc/other/kengan_omega/c1/1.html", false},
	}
	for _, c := range cases {
		if got := mangahereChapterURL(c.url); got != c.want {
			t.Errorf("mangahereChapterURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestMangahereSeriesURL(t *testing.T) {
	cases := []struct {
		url     string
		wantURL string
		wantOK  bool
	}{
		{"https://www.mangahere.cc/manga/kengan_omega/c369/1.html", "https://www.mangahere.cc/manga/kengan_omega/", true},
		{"https://www.mangahere.cc/manga/one_piece/v98/c1195/1.html", "https://www.mangahere.cc/manga/one_piece/", true},
		{"https://www.mangahere.cc/manga/one_piece/v00/c001.1/1.html", "https://www.mangahere.cc/manga/one_piece/", true},
		{"https://mangahere.cc/manga/kengan_omega/c369/1.html", "https://mangahere.cc/manga/kengan_omega/", true},
		// series URL — no-op
		{"https://www.mangahere.cc/manga/kengan_omega/", "", false},
		{"https://www.mangahere.cc/manga/kengan_omega", "", false},
	}
	for _, c := range cases {
		got, ok := mangahereSeriesURL(c.url)
		if ok != c.wantOK || got != c.wantURL {
			t.Errorf("mangahereSeriesURL(%q) = (%q, %v), want (%q, %v)", c.url, got, ok, c.wantURL, c.wantOK)
		}
	}
}
