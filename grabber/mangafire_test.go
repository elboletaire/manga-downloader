// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangafireHid(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://mangafire.to/title/dkw-one-piece", "dkw", false},
		{"https://mangafire.to/title/dkw-one-piece/chapter/9054304", "dkw", false},
		{"https://mangafire.to/manga/one-piecee.dkw", "dkw", false}, // legacy format
		{"https://mangafire.to/title/", "", true},
		{"https://mangafire.to/", "", true},
	}

	for _, c := range cases {
		m := Mangafire{Grabber: &Grabber{URL: c.url}}
		got, err := m.hid()
		if (err != nil) != c.wantErr {
			t.Errorf("hid(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("hid(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
