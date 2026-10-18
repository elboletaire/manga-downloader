// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"strings"
	"testing"
)

func TestTeamshadowiResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://team-shadowi.com/read/the-regressed-mercenary-has-a-plan/109", "https://team-shadowi.com/series/the-regressed-mercenary-has-a-plan"},
		{"https://www.team-shadowi.com/read/the-regressed-mercenary-has-a-plan/109", "https://www.team-shadowi.com/series/the-regressed-mercenary-has-a-plan"},
		{"https://team-shadowi.com/read/the-regressed-mercenary-has-a-plan/109?foo=1", "https://team-shadowi.com/series/the-regressed-mercenary-has-a-plan"},
		{"https://TEAM-SHADOWI.COM/read/the-regressed-mercenary-has-a-plan/109", "https://TEAM-SHADOWI.COM/series/the-regressed-mercenary-has-a-plan"},
		// decimal chapter
		{"https://team-shadowi.com/read/some-manga/10.5/", "https://team-shadowi.com/series/some-manga"},
		// series URLs unchanged
		{"https://team-shadowi.com/series/the-regressed-mercenary-has-a-plan", "https://team-shadowi.com/series/the-regressed-mercenary-has-a-plan"},
		// unrelated paths unchanged
		{"https://team-shadowi.com/browse", "https://team-shadowi.com/browse"},
	}

	for _, c := range cases {
		ts := &Teamshadowi{Grabber: &Grabber{URL: c.input}}
		ts.resolveSeriesURL()
		if ts.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, ts.URL, c.want)
		}
	}
}

// TestTeamshadowiParseSeries parses a trimmed copy of a series page as the
// site's current markup renders it (the RSC stream reduced to three chapters)
func TestTeamshadowiParseSeries(t *testing.T) {
	b, err := os.ReadFile("testdata/teamshadowi/series.html")
	if err != nil {
		t.Fatal(err)
	}

	data, err := parseTeamshadowiPublicData(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if want := "The Regressed Mercenary Has a Plan"; data.Series.Title != want {
		t.Errorf("title = %q, want %q", data.Series.Title, want)
	}

	ts := &Teamshadowi{Grabber: &Grabber{URL: "https://www.team-shadowi.com/series/the-regressed-mercenary-has-a-plan"}, data: data}
	chapters, errs := ts.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	want := []struct {
		number float64
		pages  int
	}{{1, 24}, {2, 22}, {110, 12}}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*TeamshadowiChapter)
		if c.Number != w.number || len(c.Pages) != w.pages {
			t.Errorf("chapter %d: number %v with %d pages, want %v with %d", i, c.Number, len(c.Pages), w.number, w.pages)
		}
	}
	if got := chapters[0].(*TeamshadowiChapter).Pages[0].URL; !strings.HasSuffix(got, "/Series/The%20Regressed%20Mercenary%20Has%20a%20Plan/1/page_001.webp") {
		t.Errorf("first page URL = %q", got)
	}
}
