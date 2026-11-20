// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestLinkedIndex(t *testing.T) {
	listing := Filterables{
		&MangafireChapter{Chapter: Chapter{Number: 1, ID: "100"}},
		// another release of the same number: only the ID tells them apart
		&MangafireChapter{Chapter: Chapter{Number: 1, ID: "101"}},
		&MangafireChapter{Chapter: Chapter{Number: 1, ID: "volume-7"}},
	}
	site := func(url string) Site {
		return NewMangafire(&Grabber{URL: url})
	}

	cases := []struct {
		name      string
		site      Site
		wantIndex int
		wantNamed bool
	}{
		{"a chapter link", site("https://mangafire.to/title/abc-x/chapter/101"), 1, true},
		{"a volume link", site("https://mangafire.to/title/abc-x/volume/7"), 2, true},
		{"a release the listing lacks", site("https://mangafire.to/title/abc-x/chapter/999"), -1, true},
		{"a series link", site("https://mangafire.to/title/abc-x"), -1, false},
		// a site that can't name a release
		{"not a Linker", NewPlainHTML(&Grabber{URL: "https://example.com/manga/x"}), -1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			index, named := LinkedIndex(c.site, listing)
			if index != c.wantIndex || named != c.wantNamed {
				t.Errorf("LinkedIndex = %d, %v; want %d, %v", index, named, c.wantIndex, c.wantNamed)
			}
		})
	}
}
