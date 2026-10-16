// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"reflect"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ranges"
)

func TestNoChaptersMessage(t *testing.T) {
	if got := NoChaptersMessage("", ""); got != "No chapters found" {
		t.Errorf("unexpected message without filters: %q", got)
	}
	want := `No chapters found for language "mx" (perhaps the site uses a different language code, e.g. "es-la" for Latin American Spanish)`
	if got := NoChaptersMessage("mx", ""); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	want = `No chapters found for scanlation group "Omega"`
	if got := NoChaptersMessage("", "Omega"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRequestsBelowListedFloor(t *testing.T) {
	// manhuaplus tales-of-demons-and-gods01: the site pruned everything below
	// chapter 280, so asking from 1 must warn...
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 529}}, 280) {
		t.Error("a range starting below the listed floor must be flagged")
	}
	// ...but a request within the listed window must not
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 300}}, 280) {
		t.Error("a range within the listed window must not be flagged")
	}
	// any of several ranges dipping below the floor is enough
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 300, End: 310}, {Begin: 100, End: 120}}, 280) {
		t.Error("a later range below the floor must be flagged")
	}
	// lists starting at the beginning have nothing missing below them, even
	// when the user asks for chapter 0 (prologue/oneshot numbering)
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 1) {
		t.Error("a floor of 1 must never be flagged")
	}
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 0) {
		t.Error("a floor of 0 must never be flagged")
	}
	// fractional floors above 1 still count (a list starting at e.g. 1.5
	// means chapter 1 was pruned)
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 10}}, 1.5) {
		t.Error("a fractional floor above 1 must be flagged")
	}
	// decimal chapter numbers within the listed window must not be flagged,
	// floor included (>= floor is served, only < floor is missing)
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 529.1, End: 529.1}}, 280) {
		t.Error("a single decimal chapter inside the window must not be flagged")
	}
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 280}}, 280) {
		t.Error("a range starting exactly at the floor must not be flagged")
	}
	// a continuation series (season 2 numbered 111+) is the reason Run only
	// consults this for user-supplied ranges: the implicit "download all"
	// range is synthesised as {Begin: 1, End: last}, which the predicate
	// flags even though the user asked for nothing that is missing
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 179}}, 111) {
		t.Error("the synthesised all-chapters range is flagged, hence Run's rangeRequested guard")
	}
	// no ranges at all must not panic nor flag
	if RequestsBelowListedFloor(nil, 280) {
		t.Error("an empty range set must not be flagged")
	}
}

func TestHasDuplicateChapterNumbers(t *testing.T) {
	dup := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en"},
		&grabber.Chapter{Number: 1, Language: "es"},
		&grabber.Chapter{Number: 2, Language: "en"},
	}
	if !HasDuplicateChapterNumbers(dup) {
		t.Error("expected duplicates to be detected")
	}

	uniq := grabber.Filterables{
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
		&grabber.Chapter{Number: 3},
	}
	if HasDuplicateChapterNumbers(uniq) {
		t.Error("expected no duplicates")
	}

	// same number twice in the same language: one entry per scanlation group,
	// which the titles already disambiguate (see #164)
	sameLang := grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Alpha]"},
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Delta]"},
	}
	if HasDuplicateChapterNumbers(sameLang) {
		t.Error("duplicates sharing a language must not trigger the language tag")
	}
}

// a same-language copy of a chapter naming its group is what the group tag tells
// apart; copies with no group at all have nothing to tell them by
func TestHasGroupDuplicates(t *testing.T) {
	grouped := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Language: "en", Group: "Delta"},
	}
	if !HasGroupDuplicates(grouped) {
		t.Error("expected same-language copies naming groups to be detected")
	}

	// one copy grouped is enough: the list still shows which copy is which
	oneGrouped := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Language: "en"},
	}
	if !HasGroupDuplicates(oneGrouped) {
		t.Error("expected a copy naming a group to be detected")
	}

	ungrouped := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en"},
		&grabber.Chapter{Number: 1, Language: "en"},
	}
	if HasGroupDuplicates(ungrouped) {
		t.Error("copies with no group must not trigger the group tag")
	}

	// the same number in two languages is the language tag's business
	otherLang := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Language: "es", Group: "Delta"},
	}
	if HasGroupDuplicates(otherLang) {
		t.Error("copies in different languages must not trigger the group tag")
	}
}

func TestReleaseLabel(t *testing.T) {
	chapter := &grabber.Chapter{Title: "Chapter 5", Number: 5, Language: "en", Group: "Group A"}
	tests := []struct {
		name                string
		showLang, showGroup bool
		want                string
	}{
		{"neither", false, false, "Chapter 5"},
		{"group", false, true, "Chapter 5 [Group A]"},
		{"language and group", true, true, "Chapter 5 [en] [Group A]"},
		{"language only", true, false, "Chapter 5 [en]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReleaseLabel(chapter, tt.showLang, tt.showGroup); got != tt.want {
				t.Errorf("ReleaseLabel() = %q, want %q", got, tt.want)
			}
		})
	}

	tagsTests := []struct {
		name                string
		chapter             *grabber.Chapter
		showLang, showGroup bool
		want                string
	}{
		{"both", chapter, true, true, " [en] [Group A]"},
		{"neither", chapter, false, false, ""},
		{"language only", chapter, true, false, " [en]"},
		{"nothing to show", &grabber.Chapter{Title: "Chapter 5"}, true, true, ""},
	}
	for _, tt := range tagsTests {
		t.Run("tags "+tt.name, func(t *testing.T) {
			if got := ReleaseTags(tt.chapter, tt.showLang, tt.showGroup); got != tt.want {
				t.Errorf("ReleaseTags() = %q, want %q", got, tt.want)
			}
		})
	}

	ungrouped := &grabber.Chapter{Title: "Chapter 5", Language: "en"}
	if got := ReleaseLabel(ungrouped, false, true); got != "Chapter 5" {
		t.Errorf("ungrouped ReleaseLabel() = %q, want %q", got, "Chapter 5")
	}

	// a title that numbers its own chapters gets the real number named too
	ownNumbering := &grabber.Chapter{Title: "111. Dong-a-jul", Number: 186, Group: "Group A"}
	if got, want := ReleaseLabel(ownNumbering, false, true), "Chapter 186 (111. Dong-a-jul) [Group A]"; got != want {
		t.Errorf("own numbering ReleaseLabel() = %q, want %q", got, want)
	}
}

func TestAllChaptersRangeIncludesChapterZero(t *testing.T) {
	// a hardcoded Begin: 1 silently dropped chapter 0 and any 0.x prologue
	// from the "download all" prompt path (#176); the range must instead
	// span the sorted list's own ends
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 0},
		&grabber.Chapter{Number: 0.5},
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
	}

	got := AllChaptersRange(chapters)
	want := ranges.Range{Begin: 0, End: 2}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}

	filtered := chapters.FilterRanges([]ranges.Range{got})
	if len(filtered) != len(chapters) {
		t.Errorf("expected all %d chapters to survive filtering, got %d", len(chapters), len(filtered))
	}
}

func TestAllChaptersRangeNotStartingAtOne(t *testing.T) {
	// a continuation series, or one thinned by --language/--scanlator, can
	// legitimately list nothing below its own floor; the synthesized range
	// must reflect that floor rather than assuming 1
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 280},
		&grabber.Chapter{Number: 281},
		&grabber.Chapter{Number: 282.5},
	}

	got := AllChaptersRange(chapters)
	want := ranges.Range{Begin: 280, End: 282.5}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// listSite is a site with one chapter list and no filter options
type listSite struct {
	grabber.Site
	chapters grabber.Filterables
}

func (f listSite) FetchTitle() (string, error) { return "Title", nil }

func (f listSite) FetchChapters() (grabber.Filterables, []error) { return f.chapters, nil }

// listedSite also lists its languages and scanlation groups
type listedSite struct {
	listSite
}

func (f listedSite) LanguageOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: []string{"en", "es"}, All: true}
}

func (f listedSite) ScanlatorOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: []string{"Alpha"}, All: true}
}

func TestOpenFillsFilterOptions(t *testing.T) {
	site := listedSite{listSite{chapters: grabber.Filterables{grabber.Chapter{Number: 1}}}}

	series, err := Open(site, &grabber.Settings{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if series.Languages == nil || !reflect.DeepEqual(series.Languages.Values, []string{"en", "es"}) || !series.Languages.All {
		t.Errorf("Languages = %+v", series.Languages)
	}
	if series.Scanlators == nil || !reflect.DeepEqual(series.Scanlators.Values, []string{"Alpha"}) {
		t.Errorf("Scanlators = %+v", series.Scanlators)
	}
}

// groupedSite lists chapters carrying their groups, and no group options of its
// own: core filters the listing by --scanlator
type groupedSite struct {
	listSite
}

func newGroupedSite() groupedSite {
	return groupedSite{listSite{chapters: grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "One", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Title: "One", Group: "Delta"},
		&grabber.Chapter{Number: 2, Title: "Two", Group: "Delta"},
		&grabber.Chapter{Number: 3, Title: "Three"},
	}}}
}

func TestOpenFiltersTheListingByGroup(t *testing.T) {
	cases := []struct {
		name      string
		scanlator string
		want      int
	}{
		{"default keeps every release", "", 4},
		{"all keeps every release", "all", 4},
		{"a group, ignoring case", "delta", 2},
		{"a group, with spaces", "  ALPHA ", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			series, err := Open(newGroupedSite(), &grabber.Settings{Scanlator: c.scanlator})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if len(series.Chapters) != c.want {
				t.Errorf("Open(%q) kept %d chapters, want %d", c.scanlator, len(series.Chapters), c.want)
			}
		})
	}
}

// the groups offered are the listing's, whatever is selected
func TestOpenListsEveryGroupWhateverIsSelected(t *testing.T) {
	for _, scanlator := range []string{"", "Alpha"} {
		series, err := Open(newGroupedSite(), &grabber.Settings{Scanlator: scanlator})
		if err != nil {
			t.Fatalf("Open(%q) error = %v", scanlator, err)
		}
		want := &grabber.FilterOptions{Values: []string{"Alpha", "Delta"}, All: true}
		if !reflect.DeepEqual(series.Scanlators, want) {
			t.Errorf("Open(%q) Scanlators = %+v, want %+v", scanlator, series.Scanlators, want)
		}
	}
}

func TestOpenUnknownGroupListsTheAvailableOnes(t *testing.T) {
	_, err := Open(newGroupedSite(), &grabber.Settings{Scanlator: "Omega"})

	var unavailable *grabber.UnavailableFilterError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Open() error = %v, want an UnavailableFilterError", err)
	}
	if unavailable.Filter != "scanlator" || unavailable.Value != "Omega" {
		t.Errorf("error = %+v", unavailable)
	}
	// the load's error still names the groups, for the window's selector
	languages, scanlators := FilterOptionsOf(newGroupedSite(), err)
	if languages != nil || scanlators == nil || !reflect.DeepEqual(scanlators.Values, []string{"Alpha", "Delta"}) || !scanlators.All {
		t.Errorf("FilterOptionsOf after the error = %+v, %+v", languages, scanlators)
	}
}

// a site that lists its groups itself gets --scanlator passed through to its
// own listing: core doesn't filter it again, and its options are its own
func TestOpenLeavesAScanlatorListerToItsOwnFilter(t *testing.T) {
	site := scanlatorLister{groupedSite: newGroupedSite()}
	series, err := Open(site, &grabber.Settings{Scanlator: "Omega"})
	if err != nil {
		t.Fatalf("Open() error = %v, want the site's own listing (it filters nothing here)", err)
	}
	if len(series.Chapters) != 4 {
		t.Errorf("Open kept %d chapters, want the site's 4", len(series.Chapters))
	}
	if series.Scanlators == nil || !reflect.DeepEqual(series.Scanlators.Values, []string{"Site Group"}) {
		t.Errorf("Scanlators = %+v, want the site's own options", series.Scanlators)
	}
}

// scanlatorLister lists the groups its site chose, ignoring what's listed
type scanlatorLister struct {
	groupedSite
}

func (scanlatorLister) ScanlatorOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: []string{"Site Group"}, All: true}
}

func TestOpenWithoutListersHasNoFilterOptions(t *testing.T) {
	site := listSite{chapters: grabber.Filterables{grabber.Chapter{Number: 1}}}

	series, err := Open(site, &grabber.Settings{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if series.Languages != nil || series.Scanlators != nil {
		t.Errorf("Languages = %+v, Scanlators = %+v; want both nil", series.Languages, series.Scanlators)
	}
}

func TestFilterOptionsOfSurvivesNoChapters(t *testing.T) {
	site := listedSite{}

	_, err := Open(site, &grabber.Settings{Language: "xx"})
	var nerr *NoChaptersError
	if !errors.As(err, &nerr) {
		t.Fatalf("Open() error = %v, want a NoChaptersError", err)
	}

	languages, scanlators := FilterOptionsOf(site, nerr)
	if languages == nil || !reflect.DeepEqual(languages.Values, []string{"en", "es"}) {
		t.Errorf("languages = %+v", languages)
	}
	if scanlators == nil || !reflect.DeepEqual(scanlators.Values, []string{"Alpha"}) {
		t.Errorf("scanlators = %+v", scanlators)
	}
}

// the options a front end offers come out sorted, whatever order the site lists
func TestFilterOptionsOfSorts(t *testing.T) {
	languages, _ := FilterOptionsOf(unsortedSite{}, nil)
	if !reflect.DeepEqual(languages.Values, []string{"en", "es", "pt-br"}) {
		t.Errorf("languages = %v, want them sorted", languages.Values)
	}

	err := &grabber.UnavailableFilterError{Filter: "scanlator", Value: "x", Available: []string{"Zed", "alpha"}}
	_, scanlators := FilterOptionsOf(unsortedSite{}, err)
	if !reflect.DeepEqual(scanlators.Values, []string{"alpha", "Zed"}) {
		t.Errorf("scanlators = %v, want them sorted", scanlators.Values)
	}
}

type unsortedSite struct{ listedSite }

func (unsortedSite) LanguageOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: []string{"pt-br", "en", "es"}}
}

// countingSite counts the times its chapter list is fetched
type countingSite struct {
	listSite
	fetches int
}

func (c *countingSite) FetchChapters() (grabber.Filterables, []error) {
	c.fetches++
	return c.listSite.FetchChapters()
}

// a listing filters again without a request, and gives what Open gives
func TestListingSeriesFiltersTheListingAgain(t *testing.T) {
	site := &countingSite{listSite: newGroupedSite().listSite}
	listing, err := Fetch(site)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	for _, scanlator := range []string{"", "Delta", "Alpha", "Omega"} {
		settings := &grabber.Settings{Scanlator: scanlator}
		got, gotErr := listing.Series(settings)
		want, wantErr := Open(newGroupedSite(), &grabber.Settings{Scanlator: scanlator})
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("Series(%q) error = %v, Open error = %v", scanlator, gotErr, wantErr)
		}
		if gotErr != nil {
			continue
		}
		if len(got.Chapters) != len(want.Chapters) || !reflect.DeepEqual(got.Scanlators, want.Scanlators) {
			t.Errorf("Series(%q) = %d chapters, %+v; Open gives %d, %+v",
				scanlator, len(got.Chapters), got.Scanlators, len(want.Chapters), want.Scanlators)
		}
	}
	if site.fetches != 1 {
		t.Errorf("the chapter list was fetched %d times, want 1", site.fetches)
	}
	if len(listing.Chapters) != 4 {
		t.Errorf("filtering changed the listing: %d chapters, want its 4", len(listing.Chapters))
	}
}

func TestListingSeriesUnknownGroupIsAnUnavailableFilterError(t *testing.T) {
	listing, err := Fetch(newGroupedSite())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	_, err = listing.Series(&grabber.Settings{Scanlator: "Omega"})

	var unavailable *grabber.UnavailableFilterError
	if !errors.As(err, &unavailable) || unavailable.Filter != "scanlator" {
		t.Errorf("Series() error = %v, want the scanlator's UnavailableFilterError", err)
	}
}
