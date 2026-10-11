// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangaparkSeriesURL(t *testing.T) {
	const series = "https://mangapark1.com/manga/rowdy-reunion"

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"series page", series, series, false},
		{"series page with query", "https://mangapark1.com/manga/rowdy-reunion?tab=chapters#top", series, false},
		{"reader page resolves to the series", "https://mangapark1.com/read/rowdy-reunion/chapter-41", series, false},
		{"old host series path", "https://mangapark.page/series/rowdy-reunion", series, false},
		{"old host bare slug", "https://mangapark.page/rowdy-reunion", series, false},
		{"bare path on the new host", "https://mangapark1.com/home", "", true},
		{"no slug", "https://mangapark1.com/", "", true},
	}

	for _, c := range cases {
		got, err := mangaparkSeriesURL(c.input)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%s: url = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMangaparkTest(t *testing.T) {
	cases := map[string]bool{
		"https://mangapark1.com/manga/rowdy-reunion":  true,
		"https://mangapark.page/series/rowdy-reunion": true,
		"https://mangapark.to/series/rowdy-reunion":   true,
		"https://example.com/manga/rowdy-reunion":     false,
	}

	for url, want := range cases {
		m := &Mangapark{Grabber: &Grabber{URL: url}}
		got, err := m.Test()
		if err != nil || got != want {
			t.Errorf("%s: Test() = %v, %v, want %v", url, got, err, want)
		}
	}
}

func TestMangaparkTitleRe(t *testing.T) {
	html := `<script type="application/ld+json">{
    "@context": "https://schema.org",
    "@type": "ComicSeries",
    "name": "Rowdy Reunion",
    "url": "https://mangapark1.com/manga/rowdy-reunion"
}</script>`

	matches := mangaparkTitleRe.FindStringSubmatch(html)
	if len(matches) != 2 || matches[1] != "Rowdy Reunion" {
		t.Errorf("title = %v, want Rowdy Reunion", matches)
	}

	if matches := mangaparkTitleRe.FindStringSubmatch(`<html>not found</html>`); matches != nil {
		t.Errorf("matched a page without a ComicSeries block: %v", matches)
	}
}

func TestParseMangaparkDropdown(t *testing.T) {
	body := `<select class="chapter-dropdown" onchange="goToChapter(this)">
<option value="">-- Select Chapter --</option>
<option value="https://mangapark1.com/read/rowdy-reunion/chapter-41" selected=""> Chapter 41 </option>
<option value="https://mangapark1.com/read/rowdy-reunion/chapter-23.5"> Chapter 23.5 </option>
<option value="https://mangapark1.com/read/rowdy-reunion/chapter-0"> Chapter 0 </option>
</select>`
	// the page renders the dropdown twice, top and bottom bars
	body += body

	chapters, err := parseMangaparkDropdown(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 (the second dropdown must not be read again)", len(chapters))
	}

	want := []struct {
		slug   string
		number float64
		title  string
	}{
		{"chapter-41", 41, "Chapter 41"},
		{"chapter-23.5", 23.5, "Chapter 23.5"},
		{"chapter-0", 0, "Chapter 0"},
	}
	for i, w := range want {
		c := chapters[i].(*MangaparkChapter)
		if c.Slug != w.slug || c.Number != w.number || c.Title != w.title {
			t.Errorf("chapter %d = {%q %v %q}, want {%q %v %q}", i, c.Slug, c.Number, c.Title, w.slug, w.number, w.title)
		}
	}

	if _, err := parseMangaparkDropdown(`<html>no dropdown</html>`); err == nil {
		t.Error("expected an error for a page without a chapter dropdown")
	}
}
