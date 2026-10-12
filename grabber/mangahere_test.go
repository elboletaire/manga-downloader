// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangahereTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string
	}{
		// www. is preserved
		{"https://www.mangahere.cc/manga/kengan_omega/", true, "https://www.mangahere.cc/manga/kengan_omega/"},
		// bare host → www.
		{"https://mangahere.cc/manga/kengan_omega/c369/1.html", true, "https://www.mangahere.cc/manga/kengan_omega/c369/1.html"},
		// mobile hosts → www.
		{"https://m.mangahere.cc/manga/a_villainous_baby_killer_whale/c116/", true, "https://www.mangahere.cc/manga/a_villainous_baby_killer_whale/c116/"},
		{"https://newm.mangahere.cc/manga/hinako_note/c034/1.html", true, "https://www.mangahere.cc/manga/hinako_note/c034/1.html"},
		// other domains — declined
		{"https://www.fanfox.net/manga/chainsaw_man/c228/1.html", false, "https://www.fanfox.net/manga/chainsaw_man/c228/1.html"},
		{"https://mangahere.cc.example.org/manga/x/", false, "https://mangahere.cc.example.org/manga/x/"},
	}

	for _, c := range cases {
		m := Mangahere{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
		if m.URL != c.wantURL {
			t.Errorf("Test(%q) left URL %q, want %q", c.url, m.URL, c.wantURL)
		}
	}
}
