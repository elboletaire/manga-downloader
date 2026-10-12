// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

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
