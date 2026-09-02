// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestSacachispaSeriesData(t *testing.T) {
	// a trimmed-down series page payload: Next.js streams the RSC data as
	// several `self.__next_f.push([1,"..."])` chunks and splits it wherever it
	// likes, so the chapters array below straddles two of them on purpose. The
	// series title lives in a different react element tuple than the chapters.
	html := `<script>self.__next_f.push([1,"1d:[\"$\",\"$L21\",null,{\"seriesId\":\"ae6ecebb\",\"title\":\"Some \\\"Quoted\\\" Series\",\"nextChapterNumber\":1}]\n1e:[\"$\",\"$L22\",null,{\"slug\":\"some-series\",\"chapters\":[{\"id\":\"c1\",\"chapter_number\":1,\"title\":null,\"page_count\":2,\"source_type\":\"upload\",\"pages\":[\"https://uwu.sacachispa.site/chapters/x/1/a.jpg\",\"https:"])</script>` +
		`<script>self.__next_f.push([1,"//uwu.sacachispa.site/chapters/x/1/b.jpg\"]},{\"id\":\"c2\",\"chapter_number\":8.5,\"title\":\"Named one\",\"page_count\":0,\"source_type\":\"cubari\",\"pages\":[]}]}]\n"])</script>`

	data, err := parseSacachispaSeriesData(html)
	if err != nil {
		t.Fatalf("parseSacachispaSeriesData() error = %v", err)
	}

	if data.Title != `Some "Quoted" Series` {
		t.Errorf("parseSacachispaSeriesData() title = %q, unexpected", data.Title)
	}
	if len(data.Chapters) != 2 {
		t.Fatalf("parseSacachispaSeriesData() got %d chapters, want 2", len(data.Chapters))
	}
	// most chapters carry a null title, which is what makes FetchChapters
	// fall back to "Chapter N"
	c := data.Chapters[0]
	if c.Number != 1 || c.Title != "" || c.SourceType != "upload" || len(c.Pages) != 2 {
		t.Errorf("parseSacachispaSeriesData() chapters[0] = %+v, unexpected", c)
	}
	if c.Pages[1] != "https://uwu.sacachispa.site/chapters/x/1/b.jpg" {
		t.Errorf("parseSacachispaSeriesData() chapters[0].Pages[1] = %q, unexpected", c.Pages[1])
	}
	// chapters hosted off-site ship no pages; FetchChapter fails loudly on them
	c = data.Chapters[1]
	if c.Number != 8.5 || c.Title != "Named one" || c.SourceType != "cubari" || len(c.Pages) != 0 {
		t.Errorf("parseSacachispaSeriesData() chapters[1] = %+v, unexpected", c)
	}

	if _, err := parseSacachispaSeriesData("no payload here"); err == nil {
		t.Error("parseSacachispaSeriesData() with no __next_f payload should error")
	}
	// a chapter page has a flight payload too, but no seriesId+title tuple
	if _, err := parseSacachispaSeriesData(`<script>self.__next_f.push([1,"8:{\"pages\":[\"a.jpg\"]}"])</script>`); err == nil {
		t.Error("parseSacachispaSeriesData() without a series title should error")
	}
}

func TestSacachispaSeriesURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://sacachispa.site/series/some-series", "https://sacachispa.site/series/some-series"},
		// chapter URLs are mapped down to the series page, which is the one
		// carrying the full chapter list in its payload
		{"https://sacachispa.site/series/some-series/chapter/15", "https://sacachispa.site/series/some-series"},
	}

	for _, c := range cases {
		s := Sacachispa{Grabber: &Grabber{URL: c.url}}
		if got := s.seriesURL(); got != c.want {
			t.Errorf("seriesURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestSacachispaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://sacachispa.site/series/boku-no-seito-wa-otona-gal", true},
		{"https://sacachispa.site/series/boku-no-seito-wa-otona-gal/chapter/15", true},
		{"https://example.com/series/boku-no-seito-wa-otona-gal", false},
	}

	for _, c := range cases {
		s := Sacachispa{Grabber: &Grabber{URL: c.url}}
		got, err := s.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
