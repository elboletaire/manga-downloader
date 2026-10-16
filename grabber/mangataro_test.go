// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestMangataroResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// reader URL → series URL
		{"https://mangataro.org/read/one-piece/ch001-50", "https://mangataro.org/manga/one-piece"},
		{"https://www.mangataro.org/read/one-piece/ch001-50", "https://www.mangataro.org/manga/one-piece"},
		{"https://mangataro.org/read/one-piece/ch001-50?foo=bar", "https://mangataro.org/manga/one-piece"},
		{"https://MANGATARO.ORG/read/one-piece/ch001-50", "https://MANGATARO.ORG/manga/one-piece"},
		// decimal chapter number
		{"https://mangataro.org/read/some-manga/ch10.5-999", "https://mangataro.org/manga/some-manga"},
		// series URLs unchanged
		{"https://mangataro.org/manga/one-piece", "https://mangataro.org/manga/one-piece"},
		// unrelated paths unchanged
		{"https://mangataro.org/browse", "https://mangataro.org/browse"},
	}

	for _, c := range cases {
		m := &Mangataro{Grabber: &Grabber{URL: c.input}}
		m.resolveSeriesURL()
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
}

// two releases of chapter 26 with different ids and groups, and one release
// that names no group (group_name is null)
const mangataroReleasesFeed = `{"success":true,"has_more":false,"chapters":[
{"id":"328453","chapter":"26","title":"","language":"en","group_name":"Demonic Scans (Low Quality)"},
{"id":"330651","chapter":"26","title":"Night","language":"en","group_name":"qiscans"},
{"id":"330700","chapter":"27","title":"","language":"en","group_name":null},
{"id":"330800","chapter":"27","title":"","language":"es","group_name":"Otra"}
]}`

func TestMangataroReleases(t *testing.T) {
	var feed mangataroChaptersFeed
	if err := json.Unmarshal([]byte(mangataroReleasesFeed), &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}

	got := mangataroReleases(feed.Chapters, "en")
	if len(got) != 3 {
		t.Fatalf("got %d chapters, want 3 (one per release, the Spanish one filtered out)", len(got))
	}
	wants := []struct{ id, group, title string }{
		{"328453", "Demonic Scans (Low Quality)", "Chapter 26"},
		{"330651", "qiscans", "Night"},
		{"330700", "", "Chapter 27"},
	}
	for i, w := range wants {
		c := got[i].(*MangataroChapter)
		if c.ID != w.id || c.Id != w.id || c.Group != w.group || ChapterLabel(c) != w.title {
			t.Errorf("chapter[%d] = {ID %q, Id %q, Group %q, Title %q}, want {%q, %q, %q, %q}",
				i, c.ID, c.Id, c.Group, ChapterLabel(c), w.id, w.id, w.group, w.title)
		}
	}

	if all := mangataroReleases(feed.Chapters, FilterAll); len(all) != 4 {
		t.Errorf("language all: got %d chapters, want 4", len(all))
	}
}
