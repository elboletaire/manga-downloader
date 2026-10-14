// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestRoliascanResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// reader URL → series URL
		{"https://roliascan.com/read/no-marriage/ch1-276741/", "https://roliascan.com/manga/no-marriage/"},
		{"https://www.roliascan.com/read/no-marriage/ch1-276741/", "https://www.roliascan.com/manga/no-marriage/"},
		{"https://roliascan.com/read/no-marriage/ch1-276741/?foo=bar", "https://roliascan.com/manga/no-marriage/"},
		{"https://ROLIASCAN.COM/read/no-marriage/ch1-276741/", "https://ROLIASCAN.COM/manga/no-marriage/"},
		// decimal chapter number
		{"https://roliascan.com/read/some-manga/ch10.5-99999/", "https://roliascan.com/manga/some-manga/"},
		// series URLs unchanged
		{"https://roliascan.com/manga/no-marriage/", "https://roliascan.com/manga/no-marriage/"},
		// unrelated paths unchanged
		{"https://roliascan.com/browse/", "https://roliascan.com/browse/"},
	}

	for _, c := range cases {
		m := &Roliascan{Grabber: &Grabber{URL: c.input}}
		m.resolveSeriesURL()
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
}

// the same release shape as mangataro: chapter 26 is released twice, with
// different ids and groups, and chapter 27's release names no group
const roliascanReleasesFeed = `{"success":true,"has_more":false,"chapters":[
{"id":"328453","chapter":"26","title":"","language":"en","group_name":"Demonic Scans (Low Quality)"},
{"id":"330651","chapter":"26","title":"","language":"en","group_name":"qiscans"},
{"id":"330700","chapter":"27","title":"","language":"","group_name":null}
]}`

func TestRoliascanReleases(t *testing.T) {
	var feed roliascanChaptersFeed
	if err := json.Unmarshal([]byte(roliascanReleasesFeed), &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}

	got := roliascanReleases(feed.Chapters, "en")
	if len(got) != 3 {
		t.Fatalf("got %d chapters, want 3 (one per release)", len(got))
	}
	wants := []struct{ id, group string }{
		{"328453", "Demonic Scans (Low Quality)"},
		{"330651", "qiscans"},
		{"330700", ""},
	}
	for i, w := range wants {
		c := got[i].(*RoliascanChapter)
		if c.ID != w.id || c.Id != w.id || c.Group != w.group {
			t.Errorf("chapter[%d] = {ID %q, Id %q, Group %q}, want {%q, %q, %q}",
				i, c.ID, c.Id, c.Group, w.id, w.id, w.group)
		}
	}
}
