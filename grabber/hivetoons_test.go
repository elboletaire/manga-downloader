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

// hivetoons used to be a PlainHTML selector entry, which silently truncated
// every series to the ~20 server-rendered anchors. IdentifySite must hand a
// hivetoons URL to the dedicated grabber, which means both that it's
// registered in the domain-matching block (before PlainHTML) and that the old
// selector entry is really gone — a re-added one would shadow it. All the
// grabbers tested before it match by domain, so this needs no network.
func TestHivetoonsIsIdentified(t *testing.T) {
	g := &Grabber{URL: "https://hivetoons.org/series/eleceed", Settings: &Settings{}}

	site, errs := g.IdentifySite()
	if len(errs) > 0 {
		t.Fatalf("IdentifySite() errs = %v", errs)
	}
	if _, ok := site.(*Hivetoons); !ok {
		t.Fatalf("IdentifySite() = %T, want *Hivetoons", site)
	}
}
