// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strconv"
	"testing"
)

// TestChapterLabelGolden pins every changed grabber's labels to the titles the
// grabber used to write, which file names and the app still show as is
func TestChapterLabelGolden(t *testing.T) {
	// numbered builds a listed chapter as the grabbers falling back to the
	// number do now: the site's title, or none
	numbered := func(title string, number float64) Chapter {
		return Chapter{Title: title, Number: number, LabelStyle: LabelNumbered}
	}
	// oldFallback is the title those grabbers wrote before labels were composed
	oldFallback := func(title string, number float64) string {
		if title == "" {
			return "Chapter " + strconv.FormatFloat(number, 'f', -1, 64)
		}
		return title
	}
	// oldNumber is the title of the grabbers that wrote the site's number string
	oldNumber := func(raw string) string { return "Chapter " + raw }

	type row struct {
		name    string
		chapter Chapter
		want    string
	}
	var rows []row

	// the site's title, or "Chapter N" when it gives none
	for _, g := range []string{"atsumaru", "flamecomics", "guya", "mangafire", "manganelo", "qimanga", "rsccomic", "sacachispa", "stonescape", "tanstackcomic", "plainhtml"} {
		for _, c := range []struct {
			title  string
			number float64
		}{
			{"", 12}, {"", 10.5}, {"", 0}, {"Dead End", 36}, {"Chapter 186 - Foo", 186}, {"Capítulo 0012", 12},
		} {
			rows = append(rows, row{fmt.Sprintf("%s %q %v", g, c.title, c.number), numbered(c.title, c.number), oldFallback(c.title, c.number)})
		}
	}
	// never a site title: always "Chapter N"
	for _, g := range []string{"comix", "heavenmanga", "lelscans", "mangitto", "mgeko", "weebcentral", "blogger", "wordpresscategory"} {
		for _, n := range []float64{1, 10.5, 1196} {
			rows = append(rows, row{fmt.Sprintf("%s %v", g, n), numbered("", n), oldFallback("", n)})
		}
	}
	// a part keeps its own words
	for _, g := range []string{"blogger", "wordpresscategory"} {
		rows = append(rows, row{g + " part", numbered(NumberLabel(364)+" Part 2", 364), "Chapter 364 Part 2"})
	}
	// the site's number string: canonical ones are composed, others stay as written
	for _, g := range []string{"chainsawmanonline", "heavenlydelusion", "jestful", "mangataro", "mkissa", "roliascan"} {
		for _, raw := range []string{"231", "13.5", "05", "10.50"} {
			n, _ := strconv.ParseFloat(raw, 64)
			rows = append(rows, row{g + " " + raw, numbered(numberTitle(raw, n), n), oldNumber(raw)})
		}
	}
	rows = append(rows,
		row{"jestful titled", numbered("Foo", 3), "Foo"},
		row{"mangataro titled", numbered("Night", 26), "Night"},
		row{"roliascan titled", numbered("Night", 26), "Night"},
	)
	// a link text left with only the number, or with more
	for _, c := range []struct {
		text   string
		number float64
		want   string
	}{
		{"Series 10", 10, "Chapter 10"},
		{"Series 10.5", 10.5, "Chapter 10.5"},
		{"Series 10 Extra Story", 10, "10 Extra Story"},
		{"Chapter 3", 3, "Chapter 3"},
		{"", 4, "Chapter 4"},
	} {
		rows = append(rows, row{"mangatown " + c.text, numbered(mangatownChapterTitle("Series", c.text), c.number), c.want})
	}
	for _, c := range []struct {
		span   string
		number float64
		want   string
	}{
		{"Chapter 267: Finale", 267, "Finale"},
		{"Chapter 267", 267, "Chapter 267"},
		{"Chapter 267.5", 267.5, "Chapter 267.5"},
		{"", 3, "Chapter 3"},
	} {
		rows = append(rows, row{"mangapark " + c.span, numbered(mangaparkChapterTitle(c.span), c.number), c.want})
	}
	// "Chapter N", then " - title" when the site gives one
	for _, c := range []struct {
		title  string
		number float64
	}{
		{"", 5}, {"   ", 5}, {"Foo", 5}, {"  Foo  ", 10.5}, {"Chapter 5 - Foo", 5},
	} {
		old := "Chapter " + strconv.FormatFloat(c.number, 'f', -1, 64)
		if c.title != "" && c.title != "   " {
			old += " - " + c.title
		}
		rows = append(rows, row{fmt.Sprintf("teamshadowi %q %v", c.title, c.number),
			Chapter{Title: c.title, Number: c.number, LabelStyle: LabelPrefixed}, cleanTitle(old)})
	}
	// the fetched chapter, zero-padded and its decimals dropped; the listing keeps the title
	for _, c := range []struct {
		title  string
		number float64
	}{
		{"Market", 1}, {"", 1}, {"", 10.5}, {"Foo", 1186}, {"Chapter 5", 5},
	} {
		rows = append(rows,
			row{fmt.Sprintf("mangadex fetched %q %v", c.title, c.number),
				Chapter{Title: c.title, Number: c.number, LabelStyle: LabelPadded},
				cleanTitle(fmt.Sprintf("Chapter %04d %s", int64(c.number), c.title))},
			row{fmt.Sprintf("mangadex listed %q %v", c.title, c.number),
				Chapter{Title: c.title, Number: c.number}, c.title})
	}

	for _, r := range rows {
		if got := ChapterLabel(r.chapter); got != r.want {
			t.Errorf("%s: ChapterLabel = %q, want %q", r.name, got, r.want)
		}
		// messages name the chapter from its label, as they did from the title
		old := r.chapter
		old.Title, old.LabelStyle = r.want, LabelTitle
		if got, want := ChapterName(r.chapter), ChapterName(old); got != want {
			t.Errorf("%s: ChapterName = %q, want %q", r.name, got, want)
		}
	}
}

func TestChapterLabelStyles(t *testing.T) {
	tests := []struct {
		name    string
		chapter Chapter
		want    string
	}{
		{"title as is", Chapter{Title: " Foo\nBar ", Number: 3}, "Foo Bar"},
		{"title as is, empty", Chapter{Number: 3}, ""},
		{"numbered, empty", Chapter{Number: 3, LabelStyle: LabelNumbered}, "Chapter 3"},
		{"numbered, decimal", Chapter{Number: 3.25, LabelStyle: LabelNumbered}, "Chapter 3.25"},
		{"numbered, titled", Chapter{Title: "Foo", Number: 3, LabelStyle: LabelNumbered}, "Foo"},
		{"prefixed, empty", Chapter{Number: 3, LabelStyle: LabelPrefixed}, "Chapter 3"},
		{"prefixed, titled", Chapter{Title: "Foo", Number: 3, LabelStyle: LabelPrefixed}, "Chapter 3 - Foo"},
		{"padded, empty", Chapter{Number: 3, LabelStyle: LabelPadded}, "Chapter 0003"},
		{"padded, titled", Chapter{Title: "Foo", Number: 3.5, LabelStyle: LabelPadded}, "Chapter 0003 Foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChapterLabel(tt.chapter); got != tt.want {
				t.Errorf("ChapterLabel() = %q, want %q", got, tt.want)
			}
			// a listing type embedding Chapter composes the same
			if got := ChapterLabel(&MangadexChapter{Chapter: tt.chapter}); got != tt.want {
				t.Errorf("ChapterLabel(listed) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLabelStyleOf(t *testing.T) {
	listed := &MangadexChapter{Chapter: Chapter{Number: 2, LabelStyle: LabelNumbered}}
	if got := labelStyleOf(listed); got != LabelNumbered {
		t.Errorf("labelStyleOf(listed) = %v, want LabelNumbered", got)
	}
	fetched := &Chapter{Title: listed.GetTitle(), Number: listed.Number, LabelStyle: labelStyleOf(listed)}
	if got := ChapterLabel(fetched); got != "Chapter 2" {
		t.Errorf("fetched label = %q, want %q", got, "Chapter 2")
	}
}
