// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestBluesoloResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://bluesolo.org/read/frieren/fr/ch/147", "https://bluesolo.org/comics/frieren"},
		{"https://www.bluesolo.org/read/frieren/fr/ch/147", "https://www.bluesolo.org/comics/frieren"},
		{"https://bluesolo.org/read/frieren/fr/ch/147?foo=1", "https://bluesolo.org/comics/frieren"},
		{"https://BLUESOLO.ORG/read/frieren/fr/ch/147", "https://BLUESOLO.ORG/comics/frieren"},
		// decimal chapter
		{"https://bluesolo.org/read/frieren/fr/ch/147.5/", "https://bluesolo.org/comics/frieren"},
		// series URLs unchanged
		{"https://bluesolo.org/comics/frieren", "https://bluesolo.org/comics/frieren"},
		// unrelated paths unchanged
		{"https://bluesolo.org/browse", "https://bluesolo.org/browse"},
	}

	for _, c := range cases {
		b := &Bluesolo{Grabber: &Grabber{URL: c.input}}
		b.resolveSeriesURL()
		if b.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, b.URL, c.want)
		}
	}
}

// a chapter two teams made, one team, and one with a null entry in its teams
func TestBluesoloFetchChaptersGroups(t *testing.T) {
	const feedJSON = `{"comic":{"title":"Frieren","chapters":[
		{"chapter":147,"title":"","full_title":"Chapitre 147","language":"fr","url":"/read/frieren/fr/ch/147","licensed":0,"id":null,"teams":[{"name":"Alpha","url":"https://a"},{"name":"Beta","url":"https://b"}]},
		{"chapter":148,"title":"","full_title":"Chapitre 148","language":"fr","url":"/read/frieren/fr/ch/148","licensed":0,"id":null,"teams":[{"name":"Gamma","url":"https://g"},null]},
		{"chapter":149,"title":"","full_title":"Chapitre 149","language":"fr","url":"/read/frieren/fr/ch/149","licensed":0,"id":null,"teams":[null]}
	]}}`

	var feed bluesoloComicFeed
	if err := json.Unmarshal([]byte(feedJSON), &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}

	b := &Bluesolo{Grabber: &Grabber{URL: "https://bluesolo.org/comics/frieren", Settings: &Settings{}}, feed: &feed}
	chapters, errs := b.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wants := []struct{ id, group string }{
		{"/read/frieren/fr/ch/147", "Alpha, Beta"},
		{"/read/frieren/fr/ch/148", "Gamma"},
		{"/read/frieren/fr/ch/149", ""},
	}
	if len(chapters) != len(wants) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(wants))
	}
	for i, w := range wants {
		c := chapters[i].(*BluesoloChapter)
		if c.ID != w.id || c.Group != w.group {
			t.Errorf("chapter[%d] ID = %q, Group = %q, want %q, %q", i, c.ID, c.Group, w.id, w.group)
		}
	}
}
