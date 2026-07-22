// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangaparkSeriesDataRe(t *testing.T) {
	cases := []struct {
		name      string
		html      string
		wantTitle string
		wantSlug  string
		wantOk    bool
	}{
		{
			name: "typical series page blob",
			html: `
        window.seriesData = {
        title: "Rowdy Reunion",
        slug: "rowdy-reunion",
        slug_hash: "rowdy-reunion.NTFztg",
        status: "Ongoing",
        kind: "manga",
        chapters: [{"comic_id":18307,"chapter_id":43,"chapter_name":"Chapter 41"}]
        };`,
			wantTitle: "Rowdy Reunion",
			wantSlug:  "rowdy-reunion",
			wantOk:    true,
		},
		{
			name:   "no seriesData in page",
			html:   `<html><body>not found</body></html>`,
			wantOk: false,
		},
	}

	for _, c := range cases {
		matches := mangaparkSeriesDataRe.FindStringSubmatch(c.html)
		ok := len(matches) == 3
		if ok != c.wantOk {
			t.Errorf("%s: match = %v, want %v", c.name, ok, c.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if matches[1] != c.wantTitle {
			t.Errorf("%s: title = %q, want %q", c.name, matches[1], c.wantTitle)
		}
		if matches[2] != c.wantSlug {
			t.Errorf("%s: slug = %q, want %q", c.name, matches[2], c.wantSlug)
		}
	}
}
