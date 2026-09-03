// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestHivetoonsTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://hivetoons.org/series/eleceed", true},
		{"https://hivetoons.org/series/eleceed/chapter-3", true},
		{"https://example.com/series/eleceed", false},
	}

	for _, c := range cases {
		h := NewHivetoons(&Grabber{URL: c.url})
		got, err := h.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
