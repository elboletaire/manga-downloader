// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestAzoraflyTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://azorafly.com/series/a-ghostly-song", true},
		{"https://azorafly.com/series/a-ghostly-song/chapter-20", true},
		{"https://example.com/series/a-ghostly-song", false},
	}

	for _, c := range cases {
		a := NewAzorafly(&Grabber{URL: c.url})
		got, err := a.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
