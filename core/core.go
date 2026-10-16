// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package core is the download flow behind the CLI, with no terminal or UI
// dependency: the CLI and the GUI drive the same steps, and each renders them
// its own way through the Events interface.
package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
)

// SettingError reports a setting that failed validation. Setting is the flag
// name ("format", "reading-direction", "convert-images", "volumes",
// "skip-pages" or, for a volume file's name, "filename-template") and Value
// the raw input.
type SettingError struct {
	Setting string
	Value   string
	Err     error
}

func (e *SettingError) Error() string {
	return fmt.Sprintf("invalid %s %q: %s", e.Setting, e.Value, e.Err)
}

func (e *SettingError) Unwrap() error { return e.Err }

// Validate checks the settings that can fail before anything is fetched, in
// the order they're reported, and returns the parsed --skip-pages ranges (nil
// when none were given). It also normalises the format and the reading
// direction in s.
func Validate(s *grabber.Settings) ([]ranges.Range, error) {
	s.Format = strings.ToLower(strings.TrimSpace(s.Format))
	if s.Format != packer.FormatCBZ && s.Format != packer.FormatRaw && s.Format != packer.FormatEPUB {
		return nil, &SettingError{
			Setting: "format",
			Value:   s.Format,
			Err:     fmt.Errorf("must be %q, %q or %q", packer.FormatCBZ, packer.FormatRaw, packer.FormatEPUB),
		}
	}

	s.ReadingDirection = packer.NormalizeReadingDirection(s.ReadingDirection)
	// only EPUB output reads the direction, so only an EPUB is refused over it.
	// Empty means the default, as the settings of a caller that never set it
	// (or a saved file from before it existed) mean
	if d := s.ReadingDirection; s.Format == packer.FormatEPUB && d != "" && d != packer.ReadingDirectionRTL && d != packer.ReadingDirectionLTR {
		return nil, &SettingError{
			Setting: "reading-direction",
			Value:   d,
			Err:     fmt.Errorf("must be %q or %q", packer.ReadingDirectionRTL, packer.ReadingDirectionLTR),
		}
	}

	if _, err := grabber.ParseConvertFormats(s.ConvertImages); err != nil {
		return nil, &SettingError{Setting: "convert-images", Value: s.ConvertImages, Err: err}
	}

	if _, err := ParseVolumes(s.Volumes); err != nil {
		return nil, &SettingError{Setting: "volumes", Value: s.Volumes, Err: err}
	}
	if strings.TrimSpace(s.Volumes) != "" && s.Bundle {
		return nil, &SettingError{Setting: "volumes", Value: s.Volumes, Err: errVolumesBundle}
	}

	if s.SkipPages == "" {
		return nil, nil
	}
	// Relative: unlike chapter ranges, page positions have a known end, so a
	// negative one counts back from it
	skip, err := ranges.ParseRelative(s.SkipPages)
	if err != nil {
		return nil, &SettingError{Setting: "skip-pages", Value: s.SkipPages, Err: err}
	}
	return skip, nil
}

// Identify normalises the settings and finds the site for url. It returns a
// nil site when none matches. The errors are the non-fatal ones from testing
// the sites (a site may be down): they're reported even when a site is found.
func Identify(url string, s *grabber.Settings) (grabber.Site, []error) {
	// the limits are only read by the download, never while identifying the
	// site, so clamping before NewSite is the same as clamping after it
	s.Normalize()
	return grabber.NewSite(url, s)
}

// TitleError is returned when the series title can't be fetched.
type TitleError struct {
	Err error
}

func (e *TitleError) Error() string { return "error fetching title: " + e.Err.Error() }

func (e *TitleError) Unwrap() error { return e.Err }

// ChaptersError is returned when fetching the chapter list reported errors.
type ChaptersError struct {
	Errs []error
}

func (e *ChaptersError) Error() string {
	msgs := make([]string, len(e.Errs))
	for i, err := range e.Errs {
		msgs[i] = err.Error()
	}
	return "error fetching chapters: " + strings.Join(msgs, "; ")
}

// Unwrap returns the errors the chapter list reported, so a filter error one of
// them is (an unknown --scanlator, say) is still found with errors.As
func (e *ChaptersError) Unwrap() []error { return e.Errs }

// NoChaptersError is returned when the chapter list is empty, which usually
// means the language or scanlator filter matched nothing.
type NoChaptersError struct {
	Language  string
	Scanlator string
}

func (e *NoChaptersError) Error() string {
	return NoChaptersMessage(e.Language, e.Scanlator)
}

// Series is an identified site with its title and listed chapters.
type Series struct {
	Site grabber.Site
	// Title is the series title, used in the filenames
	Title string
	// Chapters are the listed chapters, SortByNumber()-sorted
	Chapters grabber.Filterables
	// Languages and Scanlators are the filter options of the site, nil when it
	// doesn't list them (see FilterOptionsOf)
	Languages  *grabber.FilterOptions
	Scanlators *grabber.FilterOptions

	settings *grabber.Settings
}

// FilterOptionsOf returns the language and scanlation group options of a site,
// nil for each one it doesn't list. They stay valid when a filter matched nothing
// (err is the load's error), so a front end can still offer the values that exist.
func FilterOptionsOf(site grabber.Site, err error) (languages, scanlators *grabber.FilterOptions) {
	if l, ok := site.(grabber.LanguageLister); ok {
		options := l.LanguageOptions().Sorted()
		languages = &options
	}

	var unavailable *grabber.UnavailableFilterError
	if errors.As(err, &unavailable) && unavailable.Filter == "scanlator" {
		scanlators = &grabber.FilterOptions{Values: grabber.SortedValues(unavailable.Available), All: true}
	} else if s, ok := site.(grabber.ScanlatorLister); ok {
		options := s.ScanlatorOptions().Sorted()
		scanlators = &options
	}

	return languages, scanlators
}

// Listing is the title and chapter list of an identified site, before the
// scanlation group filter. Series applies that filter without a new request.
type Listing struct {
	Site grabber.Site
	// Title is the series title, used in the filenames
	Title string
	// Chapters are every group's releases, SortByNumber()-sorted
	Chapters grabber.Filterables
}

// Fetch fetches the title and the chapter list of an identified site: the part
// of Open that needs the site. Its errors are Open's.
func Fetch(site grabber.Site) (*Listing, error) {
	title, err := site.FetchTitle()
	if err != nil {
		return nil, &TitleError{Err: err}
	}

	chapters, errs := site.FetchChapters()
	if len(errs) > 0 {
		return nil, &ChaptersError{Errs: errs}
	}

	return &Listing{Site: site, Title: title, Chapters: chapters.SortByNumber()}, nil
}

// Open fetches the title and the chapter list of an identified site. s must
// be the same settings pointer Identify was given: the site reads its
// concurrency and format from it, and Download reads the bundle and output
// settings from it.
func Open(site grabber.Site, s *grabber.Settings) (*Series, error) {
	listing, err := Fetch(site)
	if err != nil {
		return nil, err
	}

	return listing.Series(s)
}

// Series applies the settings' filters to a listing, the second half of Open.
// It makes no request, so the same listing can be filtered again. s is as in Open.
func (l *Listing) Series(s *grabber.Settings) (*Series, error) {
	// an empty list would make the callers' prompt path index an empty slice
	if len(l.Chapters) == 0 {
		return nil, &NoChaptersError{Language: s.Language, Scanlator: s.Scanlator}
	}

	languages, scanlators := FilterOptionsOf(l.Site, nil)
	chapters := slices.Clone(l.Chapters)
	// a site that lists its groups itself (ScanlatorLister) filters them itself;
	// otherwise the groups its listing carries are what --scanlator picks from
	if _, ok := l.Site.(grabber.ScanlatorLister); !ok {
		var err error
		scanlators = grabber.GroupOptions(chapters)
		if chapters, err = grabber.SelectGroup(chapters, s.Scanlator); err != nil {
			return nil, err
		}
	}

	return &Series{
		Site:       l.Site,
		Title:      l.Title,
		Chapters:   chapters,
		Languages:  languages,
		Scanlators: scanlators,
		settings:   s,
	}, nil
}

// AllChaptersRange builds the range covering every listed chapter, for
// downloading the whole series. chapters must be SortByNumber()-sorted, so
// its ends are the true floor and ceiling: assuming Begin: 1 silently dropped
// chapter 0 and any 0.x prologue, and misidentified the floor of any series
// not starting at chapter 1 (#176).
func AllChaptersRange(chapters grabber.Filterables) ranges.Range {
	return ranges.Range{
		Begin: chapters[0].GetNumber(),
		End:   chapters[len(chapters)-1].GetNumber(),
	}
}

// RequestsBelowListedFloor reports whether any requested range starts below
// the first chapter the site lists. It only flags the gap, not the cause:
// pruning, a continuation series, and a --language/--scanlator filter can all
// produce a nonzero floor. Floors of 0 or 1 mean the list already starts at
// the beginning, so nothing is missing below them.
func RequestsBelowListedFloor(rngs []ranges.Range, floor float64) bool {
	if floor <= 1 {
		return false
	}
	for _, r := range rngs {
		if r.Begin < floor {
			return true
		}
	}

	return false
}

// NoChaptersMessage builds the message for an empty chapter list, pointing at
// whichever filter was set.
func NoChaptersMessage(language, scanlator string) string {
	switch {
	case language != "":
		return fmt.Sprintf(
			"No chapters found for language %q (perhaps the site uses a different language code, e.g. %q for Latin American Spanish)",
			language, "es-la",
		)
	case scanlator != "":
		return fmt.Sprintf("No chapters found for scanlation group %q", scanlator)
	}

	return "No chapters found"
}

// HasDuplicateChapterNumbers reports whether the same chapter number appears
// more than once in *different* languages, which happens when a site returns
// one entry per translated language for the same chapter. Duplicates sharing
// a language are HasGroupDuplicates' business.
func HasDuplicateChapterNumbers(chapters grabber.Filterables) bool {
	seen := map[float64]string{}
	for _, c := range chapters {
		lang := ""
		// not part of Filterable: only the sites that list one entry per
		// language have a language to report before the chapter is fetched
		if l, ok := c.(interface{ GetLanguage() string }); ok {
			lang = l.GetLanguage()
		}
		if prev, dup := seen[c.GetNumber()]; dup && prev != lang {
			return true
		}
		seen[c.GetNumber()] = lang
	}

	return false
}

// HasGroupDuplicates reports whether a chapter number is listed more than once in
// one language with a group named, so the group tells the copies apart.
func HasGroupDuplicates(chapters grabber.Filterables) bool {
	type key struct {
		number float64
		lang   string
	}
	count := map[key]int{}
	grouped := map[key]bool{}
	for _, c := range chapters {
		var lang, group string
		if l, ok := c.(interface{ GetLanguage() string }); ok {
			lang = l.GetLanguage()
		}
		if g, ok := c.(interface{ GetGroup() string }); ok {
			group = g.GetGroup()
		}
		k := key{c.GetNumber(), lang}
		count[k]++
		if group != "" {
			grouped[k] = true
		}
		if count[k] > 1 && grouped[k] {
			return true
		}
	}

	return false
}

// ReleaseLabel names a release: its chapter name (grabber.ChapterName), then its
// language and group in brackets when showLang and showGroup ask for them.
func ReleaseLabel(f grabber.Filterable, showLang, showGroup bool) string {
	return grabber.ChapterName(f) + ReleaseTags(f, showLang, showGroup)
}

// ReleaseTags is the bracketed part of ReleaseLabel: " [en] [Group A]", or ""
// when there is none to show. Renderers keep it apart from the title so a
// narrow row can truncate the title and still show which release it is.
func ReleaseTags(f grabber.Filterable, showLang, showGroup bool) string {
	var tags string
	if l, ok := f.(interface{ GetLanguage() string }); ok && showLang && l.GetLanguage() != "" {
		tags += " [" + l.GetLanguage() + "]"
	}
	if g, ok := f.(interface{ GetGroup() string }); ok && showGroup && g.GetGroup() != "" {
		tags += " [" + g.GetGroup() + "]"
	}
	return tags
}
