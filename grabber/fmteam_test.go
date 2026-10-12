// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestFmteamSeriesSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://fmteam.fr/comics/batuque", "batuque", false},
		{"https://fmteam.fr/comics/batuque/", "batuque", false},
		{"https://fmteam.fr/comics/", "", true},
		{"https://fmteam.fr/", "", true},
	}

	for _, c := range cases {
		f := Fmteam{Grabber: &Grabber{URL: c.url}}
		got, err := f.seriesSlug()
		if (err != nil) != c.wantErr {
			t.Errorf("seriesSlug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestFmteamChapterNumber(t *testing.T) {
	sub5 := "5"
	sub0 := "0"

	cases := []struct {
		name    string
		chapter fmteamChapterJson
		want    float64
		wantErr bool
	}{
		{"whole number, no subchapter", fmteamChapterJson{Number: 157}, 157, false},
		{"nil subchapter", fmteamChapterJson{Number: 10, Subchapter: nil}, 10, false},
		{"subchapter '0' is ignored", fmteamChapterJson{Number: 10, Subchapter: &sub0}, 10, false},
		{"subchapter appends decimal", fmteamChapterJson{Number: 10, Subchapter: &sub5}, 10.5, false},
	}

	for _, c := range cases {
		got, err := fmteamChapterNumber(c.chapter)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: error = %v, wantErr %v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%s: fmteamChapterNumber() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFmteamResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://fmteam.fr/read/batuque/fr/ch/153",
			wanted: "https://fmteam.fr/comics/batuque",
		},
		{
			name:   "chapter URL with query string",
			input:  "https://fmteam.fr/read/batuque/fr/ch/153?page=1",
			wanted: "https://fmteam.fr/comics/batuque",
		},
		{
			name:   "series URL unchanged",
			input:  "https://fmteam.fr/comics/batuque",
			wanted: "https://fmteam.fr/comics/batuque",
		},
		{
			name:   "series URL with trailing slash unchanged",
			input:  "https://fmteam.fr/comics/batuque/",
			wanted: "https://fmteam.fr/comics/batuque/",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://fmteam.fr/",
			wanted: "https://fmteam.fr/",
		},
	}

	for _, c := range cases {
		f := &Fmteam{Grabber: &Grabber{URL: c.input}}
		f.resolveSeriesURL()
		if f.URL != c.wanted {
			t.Errorf("%s: got %q, want %q", c.name, f.URL, c.wanted)
		}
	}
}
