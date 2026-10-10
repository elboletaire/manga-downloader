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

func TestLoadFallsBackToTheDefaultWithoutAll(t *testing.T) {
	// the site shows one edition by default, which the URL points at
	c, _ := newLanguageController(t, grabber.FilterOptions{Values: []string{"en", "es"}, Current: "es"})
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "xx" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatalf("Load = %v, want the fallback to load", err)
	}
	if language, _ := c.LoadedFilters(); language != "" {
		t.Errorf("loaded language %q, want the default \"\"", language)
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
		{"unlisted, default", "xx", plain, "", true},
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
	en := languageListing{listing{1, "one"}, "en"}
	es := languageListing{listing{2, "dos"}, "es"}
	if got := commonLanguage(grabber.Filterables{en, languageListing{listing{2, "two"}, "en"}}); got != "en" {
		t.Errorf("one language = %q, want en", got)
	}
	if got := commonLanguage(grabber.Filterables{en, es}); got != "" {
		t.Errorf("two languages = %q, want none", got)
	}
	if got := commonLanguage(grabber.Filterables{listing{1, "one"}}); got != "" {
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
	saved := defaultSettings()
	if err := loadOptions(prefs, saved); err != nil {
		t.Fatal(err)
	}
	if saved.Language != grabber.FilterAll {
		t.Errorf("saved language %q, want the fallback %q", saved.Language, grabber.FilterAll)
	}
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
