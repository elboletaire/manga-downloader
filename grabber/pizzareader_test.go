// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// pizzaReaderOrigin is the origin the PizzaReader fixtures were captured from
const pizzaReaderOrigin = "https://lupiteam.net"

// pizzaReaderFixture reads a JSON fixture from the identify fixtures
func pizzaReaderFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(identifyFixtureDir, name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

// pizzaReaderFeedFixture decodes the series fixture, optionally changed by mutate
func pizzaReaderFeedFixture(t *testing.T, mutate func(*pizzaReaderFeed)) *pizzaReaderFeed {
	t.Helper()

	var feed pizzaReaderFeed
	if err := json.Unmarshal(pizzaReaderFixture(t, "pizzareader_comic.json"), &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}
	if mutate != nil {
		mutate(&feed)
	}
	return &feed
}

func TestPizzaReaderResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://lupiteam.net/read/area-no-kishi/it/vol/30/ch/257/sub/5", "https://lupiteam.net/comics/area-no-kishi"},
		{"https://www.lupiteam.net/read/area-no-kishi/it/ch/1", "https://www.lupiteam.net/comics/area-no-kishi"},
		{"https://lupiteam.net/read/area-no-kishi/it/ch/1?foo=1#top", "https://lupiteam.net/comics/area-no-kishi"},
		{"https://lupiteam.net/read/area-no-kishi/it", "https://lupiteam.net/comics/area-no-kishi"},
		// series URLs unchanged
		{"https://lupiteam.net/comics/area-no-kishi", "https://lupiteam.net/comics/area-no-kishi"},
		// unrelated paths unchanged
		{"https://lupiteam.net/browse", "https://lupiteam.net/browse"},
	}

	for _, c := range cases {
		p := &PizzaReader{Grabber: &Grabber{URL: c.input}}
		p.resolveSeriesURL()
		if p.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, p.URL, c.want)
		}
	}
}

// TestPizzaReaderFetchChaptersFields checks the numbering, volumes, languages,
// groups and locked flag of the chapter rows
func TestPizzaReaderFetchChaptersFields(t *testing.T) {
	feed := pizzaReaderFeedFixture(t, func(f *pizzaReaderFeed) {
		// a row the API marks as licensed, which the reader won't open
		f.Comic.Chapters[0].Licensed = 1
	})
	p := &PizzaReader{Grabber: &Grabber{URL: pizzaReaderOrigin + "/comics/area-no-kishi", Settings: &Settings{}}, feed: feed}

	chapters, errs := p.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 4 {
		t.Fatalf("got %d chapters, want 4", len(chapters))
	}

	// a subchapter is the decimal after its chapter, its volume kept
	sub := chapters[1].(*PizzaReaderChapter)
	if sub.Number != 257.5 || sub.Volume != 30 || sub.Language != "it" || sub.Path != "/read/area-no-kishi/it/vol/30/ch/257/sub/5" {
		t.Errorf("subchapter = %+v, want 257.5 in volume 30, it", sub.Chapter)
	}
	if sub.Locked {
		t.Error("unlicensed chapter is locked")
	}

	// the licensed row is kept, marked locked
	first := chapters[0].(*PizzaReaderChapter)
	if !first.Locked || first.Number != 260 {
		t.Errorf("licensed chapter = number %v, locked %v; want 260, true", first.Number, first.Locked)
	}

	// a side story without a chapter number is unnumbered, not chapter 0
	side := chapters[2].(*PizzaReaderChapter)
	if !Unnumbered(side) || side.Number != 0 {
		t.Errorf("side story: unnumbered %v, number %v", Unnumbered(side), side.Number)
	}

	// a numbered row without a volume keeps 0, and its team is set
	loose := chapters[3].(*PizzaReaderChapter)
	if loose.Number != 1 || loose.Volume != 0 || loose.Group != "LupiTeam" {
		t.Errorf("row without volume = number %v, volume %v, group %q", loose.Number, loose.Volume, loose.Group)
	}
	if got := loose.GetGroups(); len(got) != 1 || got[0] != "LupiTeam" {
		t.Errorf("GetGroups() = %v, want [LupiTeam]", got)
	}
}

// TestPizzaReaderLanguageFilter checks that --language keeps one language's
// chapters, and that LanguageOptions offers only the languages of the readable
// ones: a licensed German row is listed, but neither offered nor kept
func TestPizzaReaderLanguageFilter(t *testing.T) {
	feed := pizzaReaderFeedFixture(t, func(f *pizzaReaderFeed) {
		f.Comic.Chapters[2].Language = "en"
		f.Comic.Chapters[3].Language = "de"
		f.Comic.Chapters[3].Licensed = 1
	})

	cases := []struct {
		lang  string
		chaps int
	}{
		{"", 4},
		{FilterAll, 4},
		{"en", 1},
	}
	for _, c := range cases {
		p := &PizzaReader{Grabber: &Grabber{URL: pizzaReaderOrigin + "/comics/area-no-kishi", Settings: &Settings{Language: c.lang}}, feed: feed}

		chapters, errs := p.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		if len(chapters) != c.chaps {
			t.Errorf("FetchChapters(%q) returned %d chapters, want %d", c.lang, len(chapters), c.chaps)
		}
		assertFilterOptions(t, "pizzareader "+c.lang, p.LanguageOptions(), []string{"it", "en"}, true, "")
	}
}

// TestPizzaReaderFetchChapter checks that a chapter's pages come from the api
// at its reader path, with the chapter's group and volume kept
func TestPizzaReaderFetchChapter(t *testing.T) {
	routes := map[string]string{
		"/api/comics/area-no-kishi":                      "pizzareader_comic.json",
		"/api/read/area-no-kishi/it/vol/30/ch/257/sub/5": "pizzareader_chapter.json",
	}
	base, _ := newFixtureServer(t, pizzaReaderOrigin, routes)

	p := NewPizzaReaderByMarkup(&Grabber{URL: base + "/comics/area-no-kishi", Settings: &Settings{}})
	chapters, errs := p.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	var sub Filterable
	for _, c := range chapters {
		if c.GetNumber() == 257.5 {
			sub = c
		}
	}
	if sub == nil {
		t.Fatal("subchapter 257.5 not listed")
	}

	chapter, err := p.FetchChapter(sub)
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if len(chapter.Pages) != 3 || chapter.PagesCount != 3 {
		t.Errorf("got %d pages, want 3", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != base+"/storage/comics/area-no-kishi_BdEQScmsOw0RPD76/it-30-257-5-capitolo-extra-problemi-per-la-nadeshiko_RUZmuWC99MuaofCe/01.jpg?v=1643475606" {
		t.Errorf("first page = %q", chapter.Pages[0].URL)
	}
	if chapter.Volume != 30 || chapter.Group != "LupiTeam" || chapter.ID != "/read/area-no-kishi/it/vol/30/ch/257/sub/5" {
		t.Errorf("chapter volume %d, group %q, ID %q", chapter.Volume, chapter.Group, chapter.ID)
	}
}

// TestPizzaReaderTestByMarkup checks that a PizzaReader page on any host is
// matched with one GET, and that the other pages are refused
func TestPizzaReaderTestByMarkup(t *testing.T) {
	cases := []struct {
		fixture string
		want    bool
	}{
		{"pizzareader_series.html", true},
		{"toivolite_home.html", false},
		{"madara_series.html", false},
		{"unrelated_page.html", false},
	}

	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			base, hits := newFixtureServer(t, pizzaReaderOrigin, map[string]string{"/": c.fixture})

			got, err := NewPizzaReaderByMarkup(&Grabber{URL: base + "/", Settings: &Settings{}}).Test()
			if err != nil || got != c.want {
				t.Errorf("Test() = %v, %v; want %v, nil", got, err, c.want)
			}
			if n := hits.count("/"); n != 1 {
				t.Errorf("GET / made %d requests, want 1", n)
			}
		})
	}
}

// TestPizzaReaderIdentifySite checks that a series and a chapter URL on an
// unlisted host are identified as PizzaReader with one GET each
func TestPizzaReaderIdentifySite(t *testing.T) {
	routes := map[string]string{
		"/comics/area-no-kishi":               "pizzareader_series.html",
		"/read/area-no-kishi/it/ch/257/sub/5": "pizzareader_series.html",
	}
	base, hits := newFixtureServer(t, pizzaReaderOrigin, routes)

	for path := range routes {
		t.Run(path, func(t *testing.T) {
			site, errs := NewSite(base+path, &Settings{})
			if _, ok := site.(*PizzaReader); !ok {
				t.Fatalf("IdentifySite() = %T, want *PizzaReader (errs %v)", site, errs)
			}
			if n := hits.count(path); n != 1 {
				t.Errorf("GET %s made %d requests, want 1", path, n)
			}
		})
	}
}
