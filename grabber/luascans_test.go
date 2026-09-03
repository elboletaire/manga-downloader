// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestLuascansSeriesSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://luacomic.org/series/even-today-the-ranker-dreams-of-retirement", "even-today-the-ranker-dreams-of-retirement", false},
		{"https://luacomic.org/series/even-today-the-ranker-dreams-of-retirement/chapter-56", "even-today-the-ranker-dreams-of-retirement", false},
		{"https://luacomic.org/series/", "", true},
		{"https://luacomic.org/", "", true},
	}

	for _, c := range cases {
		l := Luascans{Grabber: &Grabber{URL: c.url}}
		got, err := l.seriesSlug()
		if (err != nil) != c.wantErr {
			t.Errorf("seriesSlug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestLuascansParseChaptersPage(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantChapters []float64
		wantLastPage int
		wantDataLen  int
		wantErr      bool
	}{
		{
			name: "single page feed",
			body: `{"meta":{"total":2,"per_page":200,"current_page":1,"last_page":1},` +
				`"data":[{"chapter_name":"Chapter 2","chapter_slug":"chapter-2"},` +
				`{"chapter_name":"Chapter 1","chapter_slug":"chapter-1"}]}`,
			wantChapters: []float64{2, 1},
			wantLastPage: 1,
			wantDataLen:  2,
		},
		{
			name: "intermediate page of a paginated feed",
			body: `{"meta":{"total":250,"per_page":200,"current_page":1,"last_page":2},` +
				`"data":[{"chapter_name":"Chapter 250","chapter_slug":"chapter-250"}]}`,
			wantChapters: []float64{250},
			wantLastPage: 2,
			wantDataLen:  1,
		},
		{
			name:         "empty page past the end",
			body:         `{"meta":{"total":90,"per_page":200,"current_page":2,"last_page":1},"data":[]}`,
			wantChapters: []float64{},
			wantLastPage: 1,
			wantDataLen:  0,
		},
		{
			name: "unparseable chapter names are skipped but still count as data",
			body: `{"meta":{"total":2,"per_page":200,"current_page":1,"last_page":3},` +
				`"data":[{"chapter_name":"Special Extra","chapter_slug":"special-extra"},` +
				`{"chapter_name":"Chapter 12.5","chapter_slug":"chapter-12-5"}]}`,
			wantChapters: []float64{12.5},
			wantLastPage: 3,
			wantDataLen:  2,
		},
		{
			name:    "malformed json",
			body:    `{"meta":`,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chapters, feed, err := parseLuascansChaptersPage(c.body)
			if (err != nil) != c.wantErr {
				t.Fatalf("parseLuascansChaptersPage() error = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if len(chapters) != len(c.wantChapters) {
				t.Fatalf("got %d chapters, want %d", len(chapters), len(c.wantChapters))
			}
			for i, want := range c.wantChapters {
				if got := chapters[i].GetNumber(); got != want {
					t.Errorf("chapter %d number = %v, want %v", i, got, want)
				}
			}
			if feed.Meta.LastPage != c.wantLastPage {
				t.Errorf("last_page = %d, want %d", feed.Meta.LastPage, c.wantLastPage)
			}
			if len(feed.Data) != c.wantDataLen {
				t.Errorf("len(feed.Data) = %d, want %d", len(feed.Data), c.wantDataLen)
			}
		})
	}
}

func TestLuascansSeriesIDRe(t *testing.T) {
	// simplified excerpt of the escaped JSON payload the series page embeds
	// in an inline `self.__next_f.push(...)` React Server Components script
	body := `self.__next_f.push([1,",\"$\",\"$L24\",null,{\"series_id\":312,\"series_type\":\"Comic\",\"seasons\":[]}"])`

	matches := seriesIDRe.FindStringSubmatch(body)
	if len(matches) != 2 {
		t.Fatalf("seriesIDRe did not match, got %v", matches)
	}
	if matches[1] != "312" {
		t.Errorf("seriesIDRe = %q, want %q", matches[1], "312")
	}
}
