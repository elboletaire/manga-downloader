// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"testing"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// countedGroups lists the releases of two groups in one listing, as a site
// that names its groups does, and counts the times its chapter list is fetched
type countedGroups struct {
	*fakeSite
	fetches *int
}

func (g countedGroups) FetchChapters() (grabber.Filterables, []error) {
	*g.fetches++
	return grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "One", Language: "en", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Title: "One", Language: "en", Group: "Delta"},
		&grabber.Chapter{Number: 2, Title: "Two", Language: "en", Group: "Delta"},
	}, nil
}

// countedLister is countedGroups for a ScanlatorLister: its listing keeps the
// group --scanlator selects, as the site would
type countedLister struct {
	countedGroups
}

func (countedLister) ScanlatorOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: []string{"Alpha", "Delta"}, All: true}
}

func (l countedLister) FetchChapters() (grabber.Filterables, []error) {
	chapters, _ := l.countedGroups.FetchChapters()
	kept, err := grabber.SelectGroup(chapters, l.Settings.Scanlator)
	if err != nil {
		return nil, []error{err}
	}

	return kept, nil
}

// newCountingController returns a controller over countedGroups (or, when
// lister is set, countedLister) and counts its fetches and identifications
func newCountingController(t *testing.T, lister bool) (c *Controller, fetches, identified *int) {
	t.Helper()
	c, base, _ := newTestController(t, false, nil)
	fetches, identified = new(int), new(int)
	group := countedGroups{fakeSite: base, fetches: fetches}
	var site grabber.Site = group
	if lister {
		site = countedLister{group}
	}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		*identified++
		base.Settings = s
		return site, nil
	}

	return c, fetches, identified
}

// setFilters changes the filters the next load uses
func setFilters(t *testing.T, c *Controller, language, scanlator string) {
	t.Helper()
	if err := c.Configure(func(s *grabber.Settings) {
		s.Language = language
		s.Scanlator = scanlator
	}); err != nil {
		t.Fatal(err)
	}
}

// mustLoad loads the test URL with the current settings
func mustLoad(t *testing.T, c *Controller) {
	t.Helper()
	if err := c.Load(testURL); err != nil {
		t.Fatalf("Load = %v", err)
	}
}

const testURL = "https://example.com/manga/test"

// assertApplied checks that the loaded list is the one the settings ask for
func assertApplied(t *testing.T, c *Controller) {
	t.Helper()
	language, scanlator := c.LoadedFilters()
	s := c.Settings()
	if language != s.Language || scanlator != s.Scanlator {
		t.Errorf("loaded filters (%q, %q) differ from the settings (%q, %q)", language, scanlator, s.Language, s.Scanlator)
	}
	if c.Stale() {
		t.Error("the loaded list is stale right after a load")
	}
}

func TestGroupChangeNeverFetchesAGenericSite(t *testing.T) {
	c, fetches, identified := newCountingController(t, false)
	mustLoad(t, c)

	for _, group := range []struct {
		name string
		want int
	}{{"Delta", 2}, {"alpha", 1}, {"", 3}, {grabber.FilterAll, 3}} {
		setFilters(t, c, "", group.name)
		mustLoad(t, c)
		if got := len(c.series.Chapters); got != group.want {
			t.Errorf("group %q listed %d chapters, want %d", group.name, got, group.want)
		}
		assertApplied(t, c)
	}
	if *fetches != 1 || *identified != 1 {
		t.Errorf("fetched %d times and identified %d, want the one load's", *fetches, *identified)
	}
}

func TestGroupChangeFetchesAgainForAScanlatorLister(t *testing.T) {
	c, fetches, _ := newCountingController(t, true)
	mustLoad(t, c)

	setFilters(t, c, "", "Delta")
	mustLoad(t, c)
	if *fetches != 2 {
		t.Errorf("fetched %d times, want 2: a lister filters its own listing", *fetches)
	}
	if got := len(c.series.Chapters); got != 2 {
		t.Errorf("listed %d chapters for Delta, want 2", got)
	}

	// the group's listing is cached too, so switching back is instant
	setFilters(t, c, "", "Alpha")
	mustLoad(t, c)
	setFilters(t, c, "", "Delta")
	mustLoad(t, c)
	if *fetches != 3 {
		t.Errorf("fetched %d times after Alpha and back to Delta, want 3", *fetches)
	}
}

func TestSwitchingLanguageBackServesTheCache(t *testing.T) {
	c, fetches, _ := newCountingController(t, false)
	setFilters(t, c, "en", "")
	mustLoad(t, c)

	setFilters(t, c, "es", "")
	mustLoad(t, c)
	setFilters(t, c, "en", "")
	mustLoad(t, c)

	if *fetches != 2 {
		t.Errorf("fetched %d times for en, es, en, want 2", *fetches)
	}
	assertApplied(t, c)
}

func TestExplicitReloadFetchesAgainAndRefreshesTheCache(t *testing.T) {
	c, fetches, identified := newCountingController(t, false)
	mustLoad(t, c)

	mustLoad(t, c)
	if *fetches != 2 || *identified != 2 {
		t.Errorf("explicit reload fetched %d times and identified %d, want 2 and 2", *fetches, *identified)
	}

	// the fresh listing is the one a filter change is served from
	setFilters(t, c, "", "Delta")
	mustLoad(t, c)
	if *fetches != 2 {
		t.Errorf("filter change after the reload fetched again (%d fetches)", *fetches)
	}
}

func TestUnknownGroupOnACachedListingFallsBackAsAFetchWould(t *testing.T) {
	c, fetches, _ := newCountingController(t, false)
	mustLoad(t, c)

	setFilters(t, c, "", "Omega")
	mustLoad(t, c)

	if *fetches != 1 {
		t.Errorf("fetched %d times, want the cached listing to answer the fallback", *fetches)
	}
	if err := c.LoadError(); err != nil {
		t.Errorf("LoadError = %v, want the fallback to load", err)
	}
	if language, scanlator := c.LoadedFilters(); scanlator != grabber.FilterAll || language != "" {
		t.Errorf("loaded filters (%q, %q), want (\"\", all)", language, scanlator)
	}
	if !logHas(c, `scanlation group "Omega" isn't in this series, loading all of them instead`) {
		t.Errorf("no fallback line in the log: %v", c.Log())
	}
	if got := len(c.series.Chapters); got != 3 {
		t.Errorf("listed %d chapters after the fallback, want 3", got)
	}
	assertApplied(t, c)
}

func TestPickIsNeverSeenAsNotApplied(t *testing.T) {
	c, _, _ := newCountingController(t, false)
	mustLoad(t, c)

	var staleSeen []bool
	c.notify = func() { staleSeen = append(staleSeen, c.Stale()) }

	// what the window does on a pick: one step that sets the filter and loads
	if err := c.ConfigureLoad(func(s *grabber.Settings) { s.Scanlator = "Delta" }, testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishLoad(testURL); err != nil {
		t.Fatal(err)
	}

	if len(staleSeen) == 0 {
		t.Fatal("no state was published")
	}
	for i, stale := range staleSeen {
		if stale {
			t.Errorf("state %d of the pick was stale, want the filter applied at once", i)
		}
	}
	assertApplied(t, c)
}

func TestConfigureShowsAnUnappliedFilterUntilALoad(t *testing.T) {
	// a typed filter is applied by a reload: until then the note must show
	c, _, _ := newCountingController(t, false)
	mustLoad(t, c)

	setFilters(t, c, "", "Delta")
	if !c.Stale() {
		t.Error("a filter changed without a load is not stale")
	}
	mustLoad(t, c)
	if c.Stale() {
		t.Error("the reload left the list stale")
	}
}

func TestLoadedFiltersAgreeWithTheSettingsAfterAnyLoad(t *testing.T) {
	for _, lister := range []bool{false, true} {
		c, _, _ := newCountingController(t, lister)
		for _, typed := range []struct{ language, scanlator string }{
			{"EN", "delta"},
			{"", "Omega"},
			{"", "ALPHA"},
			{"", ""},
			{"", "delta"}, // a cache hit, in another spelling
		} {
			setFilters(t, c, typed.language, typed.scanlator)
			mustLoad(t, c)
			assertApplied(t, c)
		}
	}
}

func TestListingCacheIsBoundedAndForgetsTheLeastRecentlyUsed(t *testing.T) {
	var cache listingCache
	site := countedGroups{fakeSite: &fakeSite{Grabber: &grabber.Grabber{}}}
	listing := func() *core.Listing { return &core.Listing{Site: site} }

	for i := range listingCacheSize {
		cache.put(testURL, &grabber.Settings{Language: langName(i)}, listing())
	}
	// touching l0 makes l1 the least recently used
	if cache.get(testURL, &grabber.Settings{Language: langName(0)}) == nil {
		t.Fatal("l0 not cached")
	}
	cache.put(testURL, &grabber.Settings{Language: "new"}, listing())

	if len(cache) != listingCacheSize {
		t.Errorf("cache holds %d listings, want %d", len(cache), listingCacheSize)
	}
	if cache.get(testURL, &grabber.Settings{Language: langName(1)}) != nil {
		t.Error("the least recently used listing survived")
	}
	if cache.get(testURL, &grabber.Settings{Language: langName(0)}) == nil {
		t.Error("the recently used listing was evicted")
	}
}

func TestListingCacheKeysAScanlatorListerByGroup(t *testing.T) {
	var cache listingCache
	fetches := 0
	lister := countedLister{countedGroups{fakeSite: &fakeSite{Grabber: &grabber.Grabber{}}, fetches: &fetches}}
	cache.put(testURL, &grabber.Settings{Scanlator: "Alpha"}, &core.Listing{Site: lister})

	if cache.get(testURL, &grabber.Settings{Scanlator: "Alpha"}) == nil {
		t.Error("the group's listing isn't cached")
	}
	if cache.get(testURL, &grabber.Settings{Scanlator: "Delta"}) != nil {
		t.Error("another group was served the listing of Alpha")
	}

	// a generic site's listing holds every group, so any group is served it
	generic := countedGroups{fakeSite: &fakeSite{Grabber: &grabber.Grabber{}}, fetches: &fetches}
	cache.put(testURL, &grabber.Settings{Scanlator: "Alpha"}, &core.Listing{Site: generic})
	if cache.get(testURL, &grabber.Settings{Scanlator: "Delta"}) == nil {
		t.Error("a generic site's listing isn't served for another group")
	}
}

// langName names the language of the i-th cached listing in the bound test
func langName(i int) string {
	return string(rune('a' + i))
}
