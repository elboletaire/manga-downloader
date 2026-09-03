// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

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

// luascansFeed builds a chapters api response with the given last_page and
// chapter slugs (named "Chapter <slug>" so they all parse)
func luascansFeed(lastPage int, numbers ...int) string {
	rows := make([]string, 0, len(numbers))
	for _, n := range numbers {
		rows = append(rows, fmt.Sprintf(
			`{"chapter_name":"Chapter %d","chapter_slug":"chapter-%d"}`, n, n,
		))
	}
	return fmt.Sprintf(
		`{"meta":{"total":%d,"per_page":200,"last_page":%d},"data":[%s]}`,
		len(numbers), lastPage, strings.Join(rows, ","),
	)
}

func chapterNumbers(f Filterables) []float64 {
	got := make([]float64, 0, len(f))
	for _, c := range f {
		got = append(got, c.GetNumber())
	}
	return got
}

func TestWalkLuascansChapters(t *testing.T) {
	cases := []struct {
		name string
		// pages maps the requested page number to its response body; a page
		// with no entry makes the fake fetcher fail the test
		pages     map[int]string
		fetchErr  map[int]bool
		want      []float64
		wantErrs  int
		wantCalls int
	}{
		{
			name:      "single page",
			pages:     map[int]string{1: luascansFeed(1, 3, 2, 1)},
			want:      []float64{3, 2, 1},
			wantCalls: 1,
		},
		{
			// the whole point of the fix: a single-shot fetcher would stop
			// after page 1 and silently return a third of the series
			name: "walks every page",
			pages: map[int]string{
				1: luascansFeed(3, 9, 8, 7),
				2: luascansFeed(3, 6, 5, 4),
				3: luascansFeed(3, 3, 2, 1),
			},
			want:      []float64{9, 8, 7, 6, 5, 4, 3, 2, 1},
			wantCalls: 3,
		},
		{
			name: "stops on an empty page even if last_page says otherwise",
			pages: map[int]string{
				1: luascansFeed(5, 2, 1),
				2: `{"meta":{"total":2,"per_page":200,"last_page":5},"data":[]}`,
			},
			want:      []float64{2, 1},
			wantCalls: 2,
		},
		{
			// a desc-ordered feed shifts when a chapter is published between
			// two requests, re-serving rows we already collected
			name: "dedups a shifted window",
			pages: map[int]string{
				1: luascansFeed(2, 5, 4, 3),
				2: luascansFeed(2, 3, 2, 1),
			},
			want:      []float64{5, 4, 3, 2, 1},
			wantCalls: 2,
		},
		{
			// an api that stops honouring `page` must not re-append the same
			// rows last_page times, nor loop forever
			name: "stops when a page makes no progress",
			pages: map[int]string{
				1: luascansFeed(4, 3, 2, 1),
				2: luascansFeed(4, 3, 2, 1),
			},
			want:      []float64{3, 2, 1},
			wantCalls: 2,
		},
		{
			// unparseable names must not be mistaken for "no progress"
			name: "an all-unparseable page does not truncate the walk",
			pages: map[int]string{
				1: luascansFeed(3, 4, 3),
				2: `{"meta":{"total":6,"per_page":200,"last_page":3},` +
					`"data":[{"chapter_name":"Extras","chapter_slug":"extras"},` +
					`{"chapter_name":"Prologue","chapter_slug":"prologue"}]}`,
				3: luascansFeed(3, 2, 1),
			},
			want:      []float64{4, 3, 2, 1},
			wantCalls: 3,
		},
		{
			// a failed intermediate fetch must surface, not be swallowed
			name:      "a mid-walk fetch error surfaces",
			pages:     map[int]string{1: luascansFeed(3, 6, 5, 4)},
			fetchErr:  map[int]bool{2: true},
			want:      []float64{6, 5, 4},
			wantErrs:  1,
			wantCalls: 2,
		},
		{
			name:      "malformed json surfaces",
			pages:     map[int]string{1: `{"meta":`},
			wantErrs:  1,
			wantCalls: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			got, errs := walkLuascansChapters(func(page int) (string, error) {
				calls++
				if calls > 20 {
					t.Fatalf("walkLuascansChapters did not terminate (page %d)", page)
				}
				if c.fetchErr[page] {
					return "", errors.New("boom")
				}
				body, ok := c.pages[page]
				if !ok {
					t.Fatalf("unexpected request for page %d", page)
				}
				return body, nil
			})

			if len(errs) != c.wantErrs {
				t.Fatalf("got %d errors (%v), want %d", len(errs), errs, c.wantErrs)
			}
			if calls != c.wantCalls {
				t.Errorf("fetched %d pages, want %d", calls, c.wantCalls)
			}
			if diff := chapterNumbers(got); !reflect.DeepEqual(diff, c.want) &&
				!(len(diff) == 0 && len(c.want) == 0) {
				t.Errorf("chapters = %v, want %v", diff, c.want)
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
