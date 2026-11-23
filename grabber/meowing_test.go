// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"
)

// meowingRoutes serves the Luna Toons fixtures: the series page, a free reader,
// a locked reader and the chapter list fragment (answered on "/")
func meowingRoutes() map[string]string {
	return map[string]string{
		"/rita-the-oracles-chosen":            "lunatoons_series.html",
		"/rita-the-oracles-chosen-chapter-1":  "lunatoons_reader.html",
		"/rita-the-oracles-chosen-chapter-31": "lunatoons_locked.html",
		"/":                                   "lunatoons_chapters.json",
		"/unrelated":                          "unrelated_page.html",
	}
}

func TestMeowingResolveSeriesURL(t *testing.T) {
	base, hits := newFixtureServer(t, "https://lunatoons.net", meowingRoutes())

	cases := []struct {
		input string
		want  string
	}{
		{base + "/rita-the-oracles-chosen-chapter-1", base + "/rita-the-oracles-chosen"},
		{base + "/rita-the-oracles-chosen-chapter-1?x=1#top", base + "/rita-the-oracles-chosen"},
		// a series URL is left alone, without a fetch
		{base + "/rita-the-oracles-chosen", base + "/rita-the-oracles-chosen"},
	}

	for _, c := range cases {
		m := &Meowing{Grabber: &Grabber{URL: c.input}}
		if err := m.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL(%q): %v", c.input, err)
		}
		if m.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, m.URL, c.want)
		}
	}
	if n := hits.count("/rita-the-oracles-chosen"); n != 0 {
		t.Errorf("series URL fetched %d times, want 0", n)
	}
}

func TestMeowingTest(t *testing.T) {
	base, _ := newFixtureServer(t, "https://lunatoons.net", meowingRoutes())

	cases := []struct {
		path string
		want bool
	}{
		{"/rita-the-oracles-chosen", true},
		{"/rita-the-oracles-chosen-chapter-1", true},
		{"/unrelated", false},
	}
	for _, c := range cases {
		m := NewMeowingByMarkup(&Grabber{URL: base + c.path})
		got, err := m.Test()
		if err != nil {
			t.Fatalf("Test(%s): %v", c.path, err)
		}
		if got != c.want {
			t.Errorf("Test(%s) = %v, want %v", c.path, got, c.want)
		}
	}

	// the listed host matches from its URL alone, with no fetch
	listed := NewMeowing(&Grabber{URL: "https://lunatoons.net/rita-the-oracles-chosen"})
	if got, _ := listed.Test(); !got {
		t.Error("Test() on lunatoons.net = false, want true")
	}
}

func TestMeowingFetchChapters(t *testing.T) {
	base, _ := newFixtureServer(t, "https://lunatoons.net", meowingRoutes())
	m := NewMeowing(&Grabber{URL: base + "/rita-the-oracles-chosen"})

	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle: %v", err)
	}
	if want := "Rita, the Oracle’s Chosen"; title != want {
		t.Errorf("FetchTitle() = %q, want %q", title, want)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters: %v", errs)
	}

	cases := []struct {
		number float64
		id     string
		locked bool
		url    string
	}{
		{31, "660e4507a9e", true, base + "/rita-the-oracles-chosen-chapter-31"},
		{30, "66055a856fe", true, base + "/rita-the-oracles-chosen-chapter-30"},
		{1, "6582a084677", false, base + "/rita-the-oracles-chosen-chapter-1"},
	}
	if len(chapters) != len(cases) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(cases))
	}
	for i, c := range cases {
		chap := chapters[i].(*MeowingChapter)
		if chap.Number != c.number || chap.ID != c.id || chap.Locked != c.locked || chap.URL != c.url {
			t.Errorf("chapter %d = {%v %q %v %q}, want {%v %q %v %q}",
				i, chap.Number, chap.ID, chap.Locked, chap.URL, c.number, c.id, c.locked, c.url)
		}
		if chap.Title != "" {
			t.Errorf("chapter %d title = %q, want none (the label carries the chapter word)", i, chap.Title)
		}
	}
}

func TestMeowingFetchChapter(t *testing.T) {
	base, _ := newFixtureServer(t, "https://lunatoons.net", meowingRoutes())
	m := NewMeowing(&Grabber{URL: base + "/rita-the-oracles-chosen"})

	free := &MeowingChapter{
		Chapter: Chapter{Number: 1, Language: "en", ID: "6582a084677", Label: Label{Style: LabelNumbered}},
		URL:     base + "/rita-the-oracles-chosen-chapter-1",
	}
	chapter, err := m.FetchChapter(free)
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	want := []string{
		"https://cdn.meowing.org/uploads/6582a084677.avif",
		"https://cdn.meowing.org/uploads/6582a087b7a.avif",
		"https://cdn.meowing.org/uploads/6582a088bb2.avif",
	}
	if len(chapter.Pages) != len(want) || chapter.PagesCount != int64(len(want)) {
		t.Fatalf("got %d pages (count %d), want %d", len(chapter.Pages), chapter.PagesCount, len(want))
	}
	for i, p := range chapter.Pages {
		if p.Number != int64(i+1) || p.URL != want[i] {
			t.Errorf("page %d = {%d %q}, want {%d %q}", i, p.Number, p.URL, i+1, want[i])
		}
	}

	// a locked reader has no page images
	locked := &MeowingChapter{
		Chapter: Chapter{Number: 31, Language: "en", ID: "660e4507a9e", Label: Label{Style: LabelNumbered}, Locked: true},
		URL:     base + "/rita-the-oracles-chosen-chapter-31",
	}
	if _, err := m.FetchChapter(locked); err == nil {
		t.Error("FetchChapter on a locked reader succeeded, want an error")
	}
}
