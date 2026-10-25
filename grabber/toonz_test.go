// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"strings"
	"testing"
)

// toonzPage wraps an RSC stream the way the site's pages ship it: one
// self.__next_f.push call holding the stream as a JSON string
func toonzPage(stream string) string {
	quoted, _ := json.Marshal(stream)
	return `<html><body><script>self.__next_f.push([1,` + string(quoted) + `])</script></body></html>`
}

func TestToonzResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://toonz.to/manhwa/some-series/chapter/1", "https://toonz.to/manhwa/some-series"},
		{"https://www.toonz.to/manhwa/some-series/chapter/4.01", "https://www.toonz.to/manhwa/some-series"},
		{"https://toonz.to/manhwa/some-series/chapter/1?x=1", "https://toonz.to/manhwa/some-series"},
		{"https://toonz.to/manhwa/some-series", "https://toonz.to/manhwa/some-series"},
		{"https://TOONZ.to/Manhwa/Some-Series/Chapter/1", "https://TOONZ.to/Manhwa/Some-Series"},
	}

	for _, c := range cases {
		tz := &Toonz{Grabber: &Grabber{URL: c.input}}
		tz.resolveSeriesURL()
		if tz.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, tz.URL, c.want)
		}
	}
}

func TestToonzSeriesChapters(t *testing.T) {
	// the first entry is a reference into the payload, not a chapter
	stream := `7:["$","$L4b",null,{"initialChapters":["$34:props:latest",` +
		`{"id":10041845,"chapterNumber":"4.01","title":"Prologue. : Official","slug":"chapter-4.01","pageCount":29},` +
		`{"id":66038,"chapterNumber":"1.00","title":null,"slug":"chapter-1","pageCount":23}],"chapterCount":2}]`

	chapters, err := toonzSeriesChapters(toonzPage(stream))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	if chapters[0].ChapterNumber != "4.01" || chapters[0].Title != "Prologue. : Official" {
		t.Errorf("first chapter = %+v", chapters[0])
	}
	if chapters[1].ChapterNumber != "1.00" || chapters[1].Title != "" {
		t.Errorf("second chapter = %+v", chapters[1])
	}
}

func TestToonzChapterImages(t *testing.T) {
	stream := `5:["$","$L1f",null,{"chapter":{"id":66038,"chapterNumber":"1.00",` +
		`"images":[{"id":1,"filename":"chapters/1300/66038/001-01.webp","width":800,"height":1392,"sortOrder":1},` +
		`{"id":2,"filename":"chapters/1300/66038/001-02.webp","width":800,"height":1392,"sortOrder":2}]}}]`

	names, err := toonzChapterImages(toonzPage(stream))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"chapters/1300/66038/001-01.webp", "chapters/1300/66038/001-02.webp"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("images = %v, want %v", names, want)
	}
}

// TestToonzFetchChaptersURLAndID checks a chapter's reader URL comes from its
// slug (a float would turn "4.10" into "4.1") and its ID from the payload's id
func TestToonzFetchChaptersURLAndID(t *testing.T) {
	tz := &Toonz{
		Grabber: &Grabber{URL: "https://toonz.to/manhwa/some-series"},
		series: &toonzSeries{
			title: "Some Series",
			chapters: []toonzListedChapter{
				{ID: 10041845, ChapterNumber: "4.10", Slug: "chapter-4.10", Title: "Prologue"},
				{ID: 66038, ChapterNumber: "1.00", Slug: "chapter-1"},
			},
		},
	}

	chapters, errs := tz.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	want := []struct {
		url, id string
		number  float64
	}{
		{"https://toonz.to/manhwa/some-series/chapter/4.10", "10041845", 4.1},
		{"https://toonz.to/manhwa/some-series/chapter/1", "66038", 1},
	}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*ToonzChapter)
		if c.URL != w.url || c.ID != w.id || c.Number != w.number {
			t.Errorf("chapter %d = {URL %q, ID %q, Number %v}, want {%q, %q, %v}", i, c.URL, c.ID, c.Number, w.url, w.id, w.number)
		}
	}
}
