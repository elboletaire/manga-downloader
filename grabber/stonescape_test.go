// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestStonescapeSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://stonescape.xyz/series/only-see-you", "only-see-you", false},
		{"https://stonescape.xyz/series/only-see-you/ch-21", "only-see-you", false},
		{"https://stonescape.xyz/series/only-see-you?foo=bar", "only-see-you", false},
		{"https://stonescape.xyz/browse", "", true},
		{"https://stonescape.xyz/", "", true},
	}

	for _, c := range cases {
		s := Stonescape{Grabber: &Grabber{URL: c.url}}
		got, err := s.slug()
		if (err != nil) != c.wantErr {
			t.Errorf("slug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestStonescapeTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://stonescape.xyz/series/only-see-you", true},
		{"https://www.stonescape.xyz/series/only-see-you", true},
		{"https://stonescape.xyz/", true},
		{"https://notstonescape.xyz/series/foo", false},
		{"https://example.com/series/foo", false},
	}

	for _, c := range cases {
		s := Stonescape{Grabber: &Grabber{URL: c.url}}
		got, err := s.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
