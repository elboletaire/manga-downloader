// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// languageSite lists languages and serves chapters only in the ones it offers,
// as a site does for --language, ignoring case: "" and "all" serve every chapter
type languageSite struct {
	*fakeSite
	options grabber.FilterOptions
}

func (l *languageSite) LanguageOptions() grabber.FilterOptions { return l.options }

func (l *languageSite) FetchChapters() (grabber.Filterables, []error) {
	lang := l.Settings.Language
	if lang != "" && lang != grabber.FilterAll && !slices.ContainsFunc(l.options.Values, func(v string) bool {
		return strings.EqualFold(v, lang)
	}) {
		return nil, nil
	}

	return l.fakeSite.FetchChapters()
}

// newLanguageController returns a controller whose site lists options
func newLanguageController(t *testing.T, options grabber.FilterOptions) (*Controller, *languageSite) {
	t.Helper()
	c, base, _ := newTestController(t, false, nil, chapters(3, 1)...)
	site := &languageSite{fakeSite: base, options: options}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}

	return c, site
}

func TestLoadFallsBackToAllWhenTypedLanguageIsMissing(t *testing.T) {
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, All: true})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "xx" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to load", err)
	}
	if language, _ := c.LoadedFilters(); language != grabber.FilterAll {
		t.Errorf("loaded language %q, want %q", language, grabber.FilterAll)
	}
	if c.Stale() {
		t.Error("the fallback left the list stale")
	}
	if len(c.Chapters()) != 3 {
		t.Errorf("%d chapters after the fallback, want 3", len(c.Chapters()))
	}
	if !logHas(c, `language "xx" isn't in this series, loading all of them instead`) {
		t.Errorf("no fallback line in the log: %v", c.Log())
	}

	language, _ := c.Filters()
	if language == nil || language.Selected != grabber.FilterAll || !language.All {
		t.Errorf("language selector after the fallback = %+v", language)
	}
}

func TestLoadFallsBackToTheCurrentWithoutAll(t *testing.T) {
	// the site shows one edition by default, which the URL points at
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, Current: "es"})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "xx" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to load", err)
	}
	if language, _ := c.LoadedFilters(); language != "es" {
		t.Errorf("loaded language %q, want the site's current \"es\"", language)
	}
	if !logHas(c, `language "xx" isn't in this series, loading "es" instead`) {
		t.Errorf("no fallback line naming the value in the log: %v", c.Log())
	}
	language, _ := c.Filters()
	if language == nil || language.Selected != "es" || language.All {
		t.Errorf("language selector = %+v, want Current es shown", language)
	}
}

func TestLoadKeepsAFoundLanguage(t *testing.T) {
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, All: true})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "es" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if language, _ := c.LoadedFilters(); language != "es" {
		t.Errorf("loaded language %q, want es kept", language)
	}
	if language, _ := c.Filters(); language == nil || language.Selected != "es" {
		t.Errorf("language selector = %+v, want es preselected", language)
	}
}

// errTest is a chapter list failure the filters don't explain
var errTest = errors.New("the site is down")

// failingSite fails its chapter list, and lists no options (as when the site
// can't be reached for them either)
type failingSite struct {
	*fakeSite
}

func (failingSite) FetchChapters() (grabber.Filterables, []error) {
	return nil, []error{errTest}
}

func TestLoadKeepsTheTypedValueWhenTheSiteFails(t *testing.T) {
	c, site := newLanguageController(t, grabber.FilterOptions{})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "xx" }); err != nil {
		t.Fatal(err)
	}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return failingSite{site.fakeSite}, nil
	}

	if err := c.Load("https://example.com/manga/test"); err == nil {
		t.Fatal("Load succeeded, want the failure reported")
	}
	if language, _ := c.LoadedFilters(); language != "xx" {
		t.Errorf("loaded language %q, want the typed value kept", language)
	}
}

func TestFiltersAfterLoadListTheOptions(t *testing.T) {
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, All: true})
	if language, scanlator := c.Filters(); language != nil || scanlator != nil {
		t.Errorf("Filters before a load = %+v, %+v", language, scanlator)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	// the chapters here carry no language, so with no filter the list shows
	// every value
	language, scanlator := c.Filters()
	if language == nil || !slices.Equal(language.Values, []string{"en", "es"}) || !language.All || language.Selected != grabber.FilterAll {
		t.Errorf("language selector = %+v", language)
	}
	if scanlator != nil {
		t.Errorf("scanlator selector = %+v, want nil: the site doesn't list groups", scanlator)
	}
}

func TestFiltersNilForASiteThatListsNothing(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if language, scanlator := c.Filters(); language != nil || scanlator != nil {
		t.Errorf("Filters = %+v, %+v, want nil for a site that lists nothing", language, scanlator)
	}
}

func TestUnmatchedFallback(t *testing.T) {
	listed := &grabber.FilterOptions{Values: []string{"en", "es-la"}, All: true}
	plain := &grabber.FilterOptions{Values: []string{"en"}}
	cases := []struct {
		name    string
		value   string
		options *grabber.FilterOptions
		want    string
		reset   bool
	}{
		{"no value", "", listed, "", false},
		{"all is never unmatched", grabber.FilterAll, listed, "", false},
		{"listed", "en", listed, "", false},
		{"listed, other case", "EN", listed, "", false},
		{"unlisted, all", "xx", listed, grabber.FilterAll, true},
		{"unlisted, no current, no filter", "xx", plain, "", true},
		{"unlisted, current", "xx", &grabber.FilterOptions{Values: []string{"en", "fr"}, Current: "fr"}, "fr", true},
		{"site lists nothing", "xx", nil, "", false},
		{"site lists no values", "xx", &grabber.FilterOptions{}, "", false},
	}
	for _, c := range cases {
		got, reset := unmatchedFallback(c.value, c.options)
		if got != c.want || reset != c.reset {
			t.Errorf("%s: unmatchedFallback(%q) = %q, %v; want %q, %v", c.name, c.value, got, reset, c.want, c.reset)
		}
	}
}

func TestSelectedFilter(t *testing.T) {
	plus := &grabber.FilterOptions{Values: []string{"en", "es"}, Current: "es"}
	listed := &grabber.FilterOptions{Values: []string{"en", "es"}, All: true}
	atsumaru := &grabber.FilterOptions{Values: []string{"Alpha", "Delta"}, All: true}
	cases := []struct {
		name    string
		loaded  string
		options *grabber.FilterOptions
		uniform string
		want    string
	}{
		{"loaded value shown", "es", listed, "", "es"},
		{"loaded all shown", grabber.FilterAll, listed, "", grabber.FilterAll},
		{"loaded value in the site's spelling", "DELTA", atsumaru, "", "Delta"},
		{"default is the current edition", "", plus, "", "es"},
		{"default is the one language the chapters share", "", listed, "en", "en"},
		{"default mixes every value", "", listed, "", grabber.FilterAll},
		{"default with no all and no current", "", &grabber.FilterOptions{Values: []string{"en"}}, "", ""},
		{"no options, nothing to show", "", nil, "", ""},
	}
	for _, c := range cases {
		if got := selectedFilter(c.loaded, c.options, c.uniform); got != c.want {
			t.Errorf("%s: selectedFilter = %q, want %q", c.name, got, c.want)
		}
	}
}

// languageListing is a chapter that reports its language
type languageListing struct {
	listing
	language string
}

func (l languageListing) GetLanguage() string { return l.language }

func TestCommonLanguage(t *testing.T) {
	en := languageListing{listing{number: 1, title: "one"}, "en"}
	es := languageListing{listing{number: 2, title: "dos"}, "es"}
	if got := commonLanguage(grabber.Filterables{en, languageListing{listing{number: 2, title: "two"}, "en"}}); got != "en" {
		t.Errorf("one language = %q, want en", got)
	}
	if got := commonLanguage(grabber.Filterables{en, es}); got != "" {
		t.Errorf("two languages = %q, want none", got)
	}
	if got := commonLanguage(grabber.Filterables{listing{number: 1, title: "one"}}); got != "" {
		t.Errorf("chapters without a language = %q, want none", got)
	}
}

// logHas reports whether a log line contains text
func logHas(c *Controller, text string) bool {
	for _, line := range c.Log() {
		if strings.Contains(line.Text, text) {
			return true
		}
	}

	return false
}

func TestStateCarriesTheSelectors(t *testing.T) {
	a, _, _ := newTestApp(t, 3)
	site := &languageSite{
		fakeSite: &fakeSite{Grabber: &grabber.Grabber{URL: "https://example.com/manga/test"}, chapters: chapters(3, 1)},
		options:  grabber.FilterOptions{Values: []string{"en", "es"}, All: true},
	}
	a.ctrl.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	st := a.State()
	want := &FilterDTO{Values: []string{"en", "es"}, All: true, Selected: grabber.FilterAll}
	if !reflect.DeepEqual(st.Languages, want) {
		t.Errorf("languages = %+v, want %+v", st.Languages, want)
	}
	if st.Scanlators != nil {
		t.Errorf("scanlators = %+v, want nil for a site that lists none", st.Scanlators)
	}
}

func TestFallbackIsSavedWithTheOptions(t *testing.T) {
	a, _, prefs := newTestApp(t, 3)
	site := &languageSite{
		fakeSite: &fakeSite{Grabber: &grabber.Grabber{URL: "https://example.com/manga/test"}, chapters: chapters(3, 1)},
		options:  grabber.FilterOptions{Values: []string{"en", "es"}, All: true},
	}
	a.ctrl.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	o := a.Options()
	o.Language = "xx"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	if got := a.Options().Language; got != grabber.FilterAll {
		t.Errorf("options language %q after the fallback, want %q", got, grabber.FilterAll)
	}
	// the load's goroutine saves just after it clears Loading
	waitUntil(t, "the fallback to be saved", func() bool {
		saved := defaultSettings()
		return loadOptions(prefs, saved) == nil && saved.Language == grabber.FilterAll
	})
}

func TestLoadCanonicalizesTheTypedSpelling(t *testing.T) {
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, All: true})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "ES" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if language, _ := c.LoadedFilters(); language != "es" {
		t.Errorf("loaded language %q, want the site's es", language)
	}
	if c.Settings().Language != "es" {
		t.Errorf("settings language %q, want es", c.Settings().Language)
	}
	if c.Stale() {
		t.Error("a case-only difference left the list stale")
	}
	if language, _ := c.Filters(); language == nil || language.Selected != "es" {
		t.Errorf("language selector = %+v, want es selected", language)
	}
}

func TestLoadDropsALanguageTheSiteListsNoneOf(t *testing.T) {
	// no options: the site doesn't say which languages it has, and the typed one
	// matches nothing (a NoChaptersError), so this load goes without it
	c, site := newLanguageController(t, grabber.FilterOptions{})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "ca" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the language dropped and the series loaded", err)
	}
	if len(c.Chapters()) != 3 {
		t.Errorf("%d chapters, want the unfiltered list of 3", len(c.Chapters()))
	}
	if !logHas(c, `language "ca" isn't in this series, loading no filter instead`) {
		t.Errorf("no fallback line in the log: %v", c.Log())
	}
	if language, _ := c.Filters(); language != nil {
		t.Errorf("language selector = %+v, want none for a site that lists none", language)
	}
	if c.Stale() {
		t.Error("the dropped language reads as stale")
	}
	// the saved language is what the next series gets, so it's never cleared
	if got := c.Settings().Language; got != "ca" {
		t.Errorf("settings language %q after the load, want the saved \"ca\" kept", got)
	}
	// the site the series downloads with must share the controller's settings,
	// or an option changed after the load is ignored by the download
	if err := c.Configure(func(s *grabber.Settings) { s.Retry = 7; s.Format = "raw" }); err != nil {
		t.Fatal(err)
	}
	if site.GetRetries() != 7 || site.GetFormat() != "raw" {
		t.Errorf("site retries/format = %d/%q after Configure, want 7/raw", site.GetRetries(), site.GetFormat())
	}
}

func TestLoadKeepsTheLanguageWhenAnotherFilterMayBeTheCause(t *testing.T) {
	// a typed scanlator the site doesn't list could be what emptied the list,
	// so the language isn't blamed: the load fails, and both values are kept
	c, _ := newLanguageController(t, grabber.FilterOptions{})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "ca"; s.Scanlator = "Zeta" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err == nil {
		t.Fatal("Load succeeded, want the empty list reported")
	}
	if got := c.Settings(); got.Language != "ca" || got.Scanlator != "Zeta" {
		t.Errorf("settings = %q/%q, want ca and Zeta kept", got.Language, got.Scanlator)
	}
}

func TestNoLanguageSiteLeavesTheSavedLanguageForTheNextSeries(t *testing.T) {
	a, _, prefs := newTestApp(t, 3)
	noLanguage := &languageSite{
		fakeSite: &fakeSite{Grabber: &grabber.Grabber{URL: "https://example.com/manga/none"}, chapters: chapters(3, 1)},
	}
	withLanguage := &languageSite{
		fakeSite: &fakeSite{Grabber: &grabber.Grabber{URL: "https://example.com/manga/ca"}, chapters: chapters(3, 1)},
		options:  grabber.FilterOptions{Values: []string{"en", "ca"}, All: true},
	}
	a.ctrl.identify = func(url string, s *grabber.Settings) (grabber.Site, []error) {
		site := noLanguage
		if strings.Contains(url, "/ca") {
			site = withLanguage
		}
		site.Settings = s
		return site, nil
	}
	o := a.Options()
	o.Language = "ca"
	if err := a.SetOptions(o); err != nil {
		t.Fatal(err)
	}

	if err := a.Load("https://example.com/manga/none"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load of the site that lists no languages", func() bool { return !a.State().Loading })
	if got := a.Options().Language; got != "ca" {
		t.Errorf("options language %q after the load of a site without languages, want ca kept", got)
	}
	saved := defaultSettings()
	if err := loadOptions(prefs, saved); err != nil || saved.Language != "ca" {
		t.Errorf("saved language %q (%v) after that load, want ca", saved.Language, err)
	}

	if err := a.Load("https://example.com/manga/ca"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load of the site that lists languages", func() bool { return !a.State().Loading })
	if language, _ := a.ctrl.LoadedFilters(); language != "ca" {
		t.Errorf("the next series loaded with %q, want the saved ca", language)
	}
	if got := len(a.ctrl.Chapters()); got != 3 {
		t.Errorf("%d chapters for the next series, want 3", got)
	}
}

func TestUnmatchedFallbackNeverDropsOnNoOptions(t *testing.T) {
	// without a NoChaptersError the typed value is kept, as for a site that
	// fails: only a load that matched nothing proves the language can't be used
	if fallback, reset := unmatchedFallback("ca", nil); reset || fallback != "" {
		t.Errorf("unmatchedFallback(ca, nil) = %q, %v, want no reset", fallback, reset)
	}
}

// titleLanguageSite picks its language while fetching the title, as mangafire
// does, so an unlisted one fails there rather than at the chapter list
type titleLanguageSite struct {
	*languageSite
}

func (s titleLanguageSite) FetchTitle() (string, error) {
	lang := s.Settings.Language
	if lang != "" && lang != grabber.FilterAll && !slices.Contains(s.options.Values, lang) {
		return "", &grabber.UnavailableFilterError{Filter: "language", Value: lang, Available: s.options.Values}
	}

	return s.languageSite.FetchTitle()
}

func TestLoadFallsBackWhenTheTitleFetchRejectsTheLanguage(t *testing.T) {
	c, site := newLanguageController(t, grabber.FilterOptions{Values: []string{"en"}, Current: "en"})
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return titleLanguageSite{site}, nil
	}
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "ca" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to the site's current language", err)
	}
	if language, _ := c.LoadedFilters(); language != "en" {
		t.Errorf("loaded language %q, want en", language)
	}
	language, _ := c.Filters()
	if language == nil || !reflect.DeepEqual(language.Values, []string{"en"}) || language.Selected != "en" {
		t.Errorf("language selector = %+v, want only en, selected", language)
	}
	if !logHas(c, `language "ca" isn't in this series, loading "en" instead`) {
		t.Errorf("no fallback line in the log: %v", c.Log())
	}
}

// scanlatorSite lists scanlation groups (does NOT implement Refilterable)
type scanlatorSite struct {
	*fakeSite
	options grabber.FilterOptions
}

func (s *scanlatorSite) ScanlatorOptions() grabber.FilterOptions { return s.options }

// refilterableSite wraps scanlatorSite and opts in to filter-only reuse,
// mimicking a grabber (like comix) that caches its chapter walk in memory.
type refilterableSite struct {
	*scanlatorSite
}

func (*refilterableSite) Refilterable() {}

// newScanlatorController returns a controller whose site lists scanlators
func newScanlatorController(t *testing.T, options grabber.FilterOptions) (*Controller, *scanlatorSite) {
	t.Helper()
	c, base, _ := newTestController(t, false, nil, chapters(3, 1)...)
	site := &scanlatorSite{fakeSite: base, options: options}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	return c, site
}

func TestFilterOnlyReloadReIdentifiesNonRefilterable(t *testing.T) {
	// A site that does not implement grabber.Refilterable (e.g. mangafire)
	// must be re-identified on every load, including a filter change, so
	// its internal per-language caches are not served stale.
	options := grabber.FilterOptions{Values: []string{"Alpha", "Beta"}, All: true}
	c, site := newScanlatorController(t, options)

	identifyCalls := 0
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		identifyCalls++
		site.Settings = s
		return site, nil
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 1 {
		t.Fatalf("first load: identify %d times, want 1", identifyCalls)
	}

	// Filter change on the same URL: must call identify again (no reuse)
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "Alpha" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 2 {
		t.Errorf("after filter reload: identify %d times, want 2 (no reuse for non-Refilterable)", identifyCalls)
	}
}

func TestFilterOnlyReloadReusesRefilterable(t *testing.T) {
	// A grabber.Refilterable site (like comix) is reused on a filter change
	// so its cached page walk is not repeated; a different URL or an explicit
	// reload of the same URL (no filter change) still creates a fresh site.
	options := grabber.FilterOptions{Values: []string{"Alpha", "Beta"}, All: true}
	c, base := newScanlatorController(t, options)
	site := &refilterableSite{scanlatorSite: base}

	identifyCalls := 0
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		identifyCalls++
		site.Settings = s
		return site, nil
	}

	// First load: identify called once
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 1 {
		t.Fatalf("first load: identify %d times, want 1", identifyCalls)
	}
	firstInstance := c.series.Site

	// Filter change on the same URL: reuse, no identify
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "Alpha" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 1 {
		t.Errorf("after filter reload: identify %d times, want still 1", identifyCalls)
	}
	if c.series.Site != firstInstance {
		t.Error("filter reload replaced the site instance, want the same one reused")
	}

	// Explicit reload (no filter change): fresh identify
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 2 {
		t.Errorf("after explicit reload: identify %d times, want 2", identifyCalls)
	}

	// Different URL: fresh identify
	identifyCalls = 0
	if err := c.Load("https://example.com/manga/other"); err != nil {
		t.Fatal(err)
	}
	if identifyCalls != 1 {
		t.Errorf("different URL: identify %d times, want 1", identifyCalls)
	}
}

// walkingSite is a Refilterable site that walks once, like comix, and fails
// an unlisted scanlator with an error of its own
type walkingSite struct {
	*scanlatorSite
	walks int
}

func (*walkingSite) Refilterable() {}

func (w *walkingSite) FetchChapters() (grabber.Filterables, []error) {
	if w.walks == 0 {
		w.walks++
	}
	scanlator := w.Settings.Scanlator
	if scanlator != "" && scanlator != grabber.FilterAll && !listsValue(&w.options, scanlator) {
		return nil, []error{errors.New("no scanlation group " + scanlator)}
	}

	return w.scanlatorSite.FetchChapters()
}

func TestUnmatchedScanlatorReusesRefilterableSite(t *testing.T) {
	options := grabber.FilterOptions{Values: []string{"Alpha", "Beta"}, All: true}
	c, base := newScanlatorController(t, options)
	identified := 0
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		identified++
		site := &walkingSite{scanlatorSite: &scanlatorSite{fakeSite: base.fakeSite, options: options}}
		site.Settings = s
		return site, nil
	}
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "Gone" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to load", err)
	}
	// a second site would walk the whole list again, which comix.to rate-limits
	if identified != 1 || c.series.Site.(*walkingSite).walks != 1 {
		t.Errorf("identified %d sites, walked %d times; want the one site, walked once",
			identified, c.series.Site.(*walkingSite).walks)
	}
	if _, scanlator := c.LoadedFilters(); scanlator != grabber.FilterAll {
		t.Errorf("loaded scanlator %q, want %q", scanlator, grabber.FilterAll)
	}
	if len(c.series.Chapters) != 3 {
		t.Errorf("listed %d chapters, want 3", len(c.series.Chapters))
	}
}

// groupSite lists every group's releases in one listing, as the sites that
// name their groups do, and serves them whatever --scanlator is: core picks the
// group. It's Refilterable, as such a site's listing is cached.
type groupSite struct {
	*fakeSite
}

func (groupSite) Refilterable() {}

func (g groupSite) FetchChapters() (grabber.Filterables, []error) {
	return grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "One", Language: "en", Group: "Alpha"},
		&grabber.Chapter{Number: 1, Title: "One", Language: "en", Group: "Delta"},
		&grabber.Chapter{Number: 2, Title: "Two", Language: "en", Group: "Delta"},
	}, nil
}

// newGroupController returns a controller whose site lists groups, and counts
// the times it's identified
func newGroupController(t *testing.T) (*Controller, *int) {
	t.Helper()
	c, base, _ := newTestController(t, false, nil, chapters(1, 1)...)
	site := groupSite{fakeSite: base}
	identified := new(int)
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		*identified++
		site.Settings = s
		return site, nil
	}

	return c, identified
}

func TestGroupSelectorOffersEveryGroupAndDefaultsToAll(t *testing.T) {
	c, _ := newGroupController(t)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}

	if len(c.Chapters()) != 3 {
		t.Errorf("%d chapters, want every release (3)", len(c.Chapters()))
	}
	if _, scanlator := c.Filters(); scanlator == nil || !slices.Equal(scanlator.Values, []string{"Alpha", "Delta"}) || !scanlator.All || scanlator.Selected != grabber.FilterAll {
		t.Errorf("scanlator selector = %+v, want Alpha and Delta, all groups, shown as all", scanlator)
	}
}

func TestLoadFiltersByGroupIgnoringCase(t *testing.T) {
	c, _ := newGroupController(t)
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "alpha" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if len(c.Chapters()) != 1 {
		t.Errorf("%d chapters for Alpha, want its 1", len(c.Chapters()))
	}
	if _, scanlator := c.LoadedFilters(); scanlator != "Alpha" {
		t.Errorf("loaded scanlator %q, want the listed spelling Alpha", scanlator)
	}
	if c.Stale() {
		t.Error("the case-only difference left the list stale")
	}
}

// a group change on a site that lists every group re-filters the listing it
// has, without asking the site for it again
func TestGroupChangeIsAFilterOnAReusedSite(t *testing.T) {
	c, identified := newGroupController(t)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "Delta" }); err != nil {
		t.Fatal(err)
	}
	if !c.Stale() {
		t.Fatal("a changed group should make the list stale")
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if *identified != 1 {
		t.Errorf("the site was identified %d times, want 1: the group change reuses it", *identified)
	}
	if len(c.Chapters()) != 2 {
		t.Errorf("%d chapters for Delta, want its 2", len(c.Chapters()))
	}
}

func TestUnknownGroupFallsBackToAll(t *testing.T) {
	c, _ := newGroupController(t)
	if err := c.Configure(func(s *grabber.Settings) { s.Scanlator = "Omega" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to load", err)
	}
	if len(c.Chapters()) != 3 {
		t.Errorf("%d chapters after the fallback, want all 3", len(c.Chapters()))
	}
	if _, scanlator := c.LoadedFilters(); scanlator != grabber.FilterAll {
		t.Errorf("loaded scanlator %q, want %q", scanlator, grabber.FilterAll)
	}
	if !logHas(c, `scanlation group "Omega" isn't in this series, loading all of them instead`) {
		t.Errorf("no fallback line in the log: %v", c.Log())
	}
}

// the group tag is sent only with the copies it tells apart
func TestChaptersCarryTheGroupWhenCopiesShareANumber(t *testing.T) {
	a, _, _ := newTestApp(t, 1)
	site := groupSite{fakeSite: &fakeSite{Grabber: &grabber.Grabber{URL: "https://example.com/manga/test"}}}
	a.ctrl.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	if err := a.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })

	if !a.State().HasGroupDuplicates {
		t.Error("state doesn't report the group copies")
	}
	rows := a.Chapters()
	want := []string{"Alpha", "Delta", "Delta"}
	for i, row := range rows {
		if row.Group != want[i] {
			t.Errorf("row %d group = %q, want %q", i, row.Group, want[i])
		}
	}
}
