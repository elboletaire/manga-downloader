// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"testing"

	"github.com/elboletaire/manga-downloader/ranges"
)

// chapterWithPages builds a chapter of n pages, each with a recognisable URL
func chapterWithPages(n int) *Chapter {
	pages := make([]Page, n)
	for i := range pages {
		pages[i] = Page{
			Number: int64(i + 1),
			URL:    "https://example.com/" + string(rune('a'+i)) + ".jpg",
		}
	}

	return &Chapter{
		Title:      "Chapter 0001",
		Number:     1,
		PagesCount: int64(n),
		Pages:      pages,
	}
}

func pageURLs(c *Chapter) []string {
	urls := []string{}
	for _, p := range c.Pages {
		urls = append(urls, p.URL)
	}

	return urls
}

func TestChapterSkipPages(t *testing.T) {
	tests := []struct {
		name    string
		skip    string
		pages   int
		want    []string
		skipped int
	}{
		{
			// tcbscans' credits page
			name: "a single page", skip: "2", pages: 5,
			want: []string{"a", "c", "d", "e"}, skipped: 1,
		},
		{
			name: "several ranges", skip: "2,4-5", pages: 6,
			want: []string{"a", "c", "f"}, skipped: 3,
		},
		{
			// the tcbscans chapter whose last page 404s
			name: "the last page", skip: "-1", pages: 5,
			want: []string{"a", "b", "c", "d"}, skipped: 1,
		},
		{
			name: "the last three pages", skip: "-3--1", pages: 5,
			want: []string{"a", "b"}, skipped: 3,
		},
		{
			name: "credits at both ends", skip: "2,-1", pages: 5,
			want: []string{"a", "c", "d"}, skipped: 2,
		},
		{
			name: "a page the chapter doesn't have is a no-op", skip: "99", pages: 3,
			want: []string{"a", "b", "c"}, skipped: 0,
		},
		{
			name: "overlapping ranges skip a page once", skip: "1-3,2,-3", pages: 4,
			want: []string{"d"}, skipped: 3,
		},
		{
			name: "every page can be skipped", skip: "1--1", pages: 3,
			want: []string{}, skipped: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rngs, err := ranges.ParseRelative(tt.skip)
			if err != nil {
				t.Fatalf("ParseRelative(%q): %s", tt.skip, err)
			}

			chapter := chapterWithPages(tt.pages)
			skipped := chapter.SkipPages(rngs)

			if skipped != tt.skipped {
				t.Errorf("skipped %d pages, want %d", skipped, tt.skipped)
			}

			want := []string{}
			for _, name := range tt.want {
				want = append(want, "https://example.com/"+name+".jpg")
			}
			if got := pageURLs(chapter); !reflect.DeepEqual(got, want) {
				t.Errorf("kept %v, want %v", got, want)
			}

			// the progress bars budget by PagesCount, so it has to follow
			if chapter.PagesCount != int64(len(chapter.Pages)) {
				t.Errorf("PagesCount is %d, want %d", chapter.PagesCount, len(chapter.Pages))
			}
		})
	}
}

func TestChapterSkipPagesResolvesPerChapter(t *testing.T) {
	// -1 is each chapter's own last page, not a fixed position
	rngs, err := ranges.ParseRelative("-1")
	if err != nil {
		t.Fatal(err)
	}

	for _, count := range []int{2, 5, 20} {
		chapter := chapterWithPages(count)
		chapter.SkipPages(rngs)

		if int(chapter.PagesCount) != count-1 {
			t.Errorf("%d pages: kept %d, want %d", count, chapter.PagesCount, count-1)
		}
		if last := chapter.Pages[len(chapter.Pages)-1]; last.Number != int64(count-1) {
			t.Errorf("%d pages: last kept page is %d, want %d", count, last.Number, count-1)
		}
	}
}

func TestChapterSkipPagesWithoutRanges(t *testing.T) {
	// the flag is optional: no ranges must leave the chapter exactly as it was
	chapter := chapterWithPages(4)
	if skipped := chapter.SkipPages(nil); skipped != 0 {
		t.Errorf("skipped %d pages, want 0", skipped)
	}
	if chapter.PagesCount != 4 || len(chapter.Pages) != 4 {
		t.Errorf("chapter was modified: %d pages, PagesCount %d", len(chapter.Pages), chapter.PagesCount)
	}
}
