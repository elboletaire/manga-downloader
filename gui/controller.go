// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/ranges"
)

// maxLogLines caps the log so a long download can't grow it without bound
const maxLogLines = 2000

// errBusy is returned when an action is refused because a series is loading
// or a download is running
var errBusy = errors.New("a series is loading or a download is running")

// LogLevel is the severity of a log line
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarn
	LogError
)

// LogLine is one entry of the log
type LogLine struct {
	Level LogLevel
	Text  string
}

// JobState is where a chapter's download is
type JobState int

const (
	// JobDownloading is a chapter whose pages are being fetched
	JobDownloading JobState = iota
	// JobArchiving is a chapter whose pages are all downloaded and being packed
	JobArchiving
	// JobDone is a chapter that was packed (or, in bundle mode, the bundle)
	JobDone
	// JobFailed is a chapter that couldn't be fetched, downloaded or packed
	JobFailed
	// JobSkipped is a chapter whose pages were all excluded by --skip-pages
	JobSkipped
)

// Job is the progress of one chapter. In bundle mode there's a single Job for
// the whole bundle, whose Total counts each page's download and archive steps
// (core.DownloadWeight and core.ArchiveWeight, as the CLI's bar does), and with
// volumes one per volume, whose Done is core.VolumeProgress out of a fixed
// Total of volumeScale (its pages aren't known until it's packed).
type Job struct {
	Title  string
	Number float64
	Total  int64
	Done   int64
	State  JobState
	Reason string
	// Status is the bundle's or the volume's phase ("Downloading
	// <chapter>", "Creating bundle..."); unused for single chapters
	Status string
	// Volume is set on the row of a volume
	Volume bool
}

// RunInfo describes the run in progress, or the last one
type RunInfo struct {
	// Chapters is how many chapters the run was started with
	Chapters int
	Bundle   bool
	// Volumes is set when the run packs one file per volume
	Volumes bool
	// Rows is how many progress rows the run ends up with: one per chapter,
	// one for a bundle, or one per volume plus one per chapter outside them
	Rows int
}

// Summary is what a finished run produced
type Summary struct {
	Dir       string
	Cancelled bool
	// Saved counts chapters written to disk (or, in bundle mode, kept for the
	// bundle, and with volumes, kept for their volume)
	Saved int
	// Volumes counts the volume files written
	Volumes int
	// Bundled is set in bundle mode, where Bundle is the bundle's filename
	// (empty when none was written)
	Bundled  bool
	Bundle   string
	Skipped  int
	Failures []string
	// Err is a bundle packing error
	Err error
}

// runStats collects what the events report during a run, for the summary
type runStats struct {
	saved    int
	volumes  int
	skipped  int
	failures []string
	bundle   string
	err      error
}

// Controller holds the state of the app and runs the core flow. It knows
// nothing about the window toolkit: the window reads its state through the
// getters and is told about changes through notify, which is called from any
// goroutine and must not call back into the controller.
//
// Every mutation happens under mu; getters return copies, so the view never
// shares memory with the event goroutines.
type Controller struct {
	notify func()
	// identify finds the site for a URL; core.Identify, replaced in tests
	identify func(url string, s *grabber.Settings) (grabber.Site, []error)
	// closeBrowser is browser.Close, replaced in tests
	closeBrowser func()
	// needsVisible is browser.NeedsVisible, and bringToFront
	// browser.BringToFront, replaced in tests
	needsVisible func(url string) bool
	bringToFront func()
	// closing tracks a browser close running in the background (see Clear),
	// which loads wait for rather than meet a browser going away
	closing sync.WaitGroup
	// reuseOnLoad is set by BeginLoad when a filter-only reload can skip
	// re-identifying the site; load() reads and clears it
	reuseOnLoad grabber.Site

	mu sync.Mutex
	// settings is the one pointer the site and core read their settings
	// through, so it's replaced (not mutated) by Load, and mutated by
	// Configure only while nothing is running
	settings *grabber.Settings

	url     string
	series  *core.Series
	loading bool
	running bool
	// cancelling is set once a run has been asked to stop, so repeated
	// cancels log once
	cancelling bool
	selected   []bool
	// hasDuplicates is core.HasDuplicateChapterNumbers for the loaded list
	hasDuplicates bool
	// loadedLanguage and loadedScanlator are the filters the list was fetched
	// with, which Configure can make stale
	loadedLanguage  string
	loadedScanlator string
	// languages and scanlators are the filter options of the site of the last
	// load, nil for each one it doesn't list. They outlive a failed load, so
	// its filters can still be changed.
	languages  *grabber.FilterOptions
	scanlators *grabber.FilterOptions

	// loadErr is the last load's error, which the view shows under the URL
	loadErr error
	// volumesText is the Volumes box, the volumes a run in volume mode packs:
	// --volumes' syntax, "" meaning the site's own. siteVolumes is the site's
	// volumes in the same syntax, what the box starts with on each load.
	volumesText string
	siteVolumes string
	// update is the newer release the update check found, nil when none
	update *Update
	// attention is the browser window waiting for the user (a check to pass
	// there), nil when none is or the user dismissed it
	attention *browser.Attention
	// selGen and logGen count changes to the selection and the log, so a view
	// that pushes state can skip re-sending either when it hasn't moved
	selGen uint64
	logGen uint64

	cancel  context.CancelFunc
	runinfo RunInfo
	rng     string
	jobs    []*Job
	byChap  map[*grabber.Chapter]*Job
	bundle  *Job
	failedC map[*grabber.Chapter]bool
	// the rows of the volumes, by volume, by listed chapter (for a failed
	// fetch) and by fetched chapter
	byVolume  map[*core.Volume]*Job
	listedVol map[grabber.Filterable]*Job
	chapVol   map[*grabber.Chapter]*Job
	stats     runStats
	summary   *Summary
	log       []LogLine
}

// NewController returns a controller that works on its own copy of settings
// and routes the library's warnings into its log.
func NewController(settings *grabber.Settings, notifyFn func()) *Controller {
	if notifyFn == nil {
		notifyFn = func() {}
	}
	c := &Controller{
		notify:       notifyFn,
		identify:     core.Identify,
		closeBrowser: browser.Close,
		needsVisible: browser.NeedsVisible,
		bringToFront: browser.BringToFront,
		settings:     settings,
	}
	notice.SetHandler(func(level notice.Level, msg string) {
		lvl := LogInfo
		if level == notice.LevelWarn {
			lvl = LogWarn
		}
		c.addLog(lvl, msg)
	})
	browser.SetAttentionHandler(c.onAttention)
	return c
}

// onAttention keeps the browser's attention requests, which the view shows
// over the window until the page renders or its load ends
func (c *Controller) onAttention(a browser.Attention) {
	c.mu.Lock()
	if a.Active {
		c.attention = &a
		c.appendLogLocked(LogWarn, a.Message())
	} else {
		c.attention = nil
	}
	c.mu.Unlock()
	c.emit()
}

// BrowserAlert is the message of the browser window waiting for the user, ""
// when none is
func (c *Controller) BrowserAlert() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attention == nil {
		return ""
	}
	return c.attention.Message()
}

// DismissBrowserAlert hides the alert; the window stays open, and the load
// goes on waiting for it
func (c *Controller) DismissBrowserAlert() {
	c.mu.Lock()
	had := c.attention != nil
	c.attention = nil
	c.mu.Unlock()
	if had {
		c.emit()
	}
}

// ShowBrowser brings the browser window to the front again
func (c *Controller) ShowBrowser() {
	c.bringToFront()
}

// BrowserVisibleAuto reports whether the browser opens visible on its own for
// the site of the URL loaded: the site needs it (declared by its grabber, or
// escalated earlier in this session), whatever the option says
func (c *Controller) BrowserVisibleAuto() bool {
	c.mu.Lock()
	url := c.url
	c.mu.Unlock()
	return url != "" && c.needsVisible(url)
}

// emit tells the view something changed. Callers must not hold mu.
func (c *Controller) emit() { c.notify() }

// addLog appends a line and notifies. Callers must not hold mu.
func (c *Controller) addLog(level LogLevel, text string) {
	c.mu.Lock()
	c.appendLogLocked(level, text)
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) appendLogLocked(level LogLevel, text string) {
	c.logGen++
	c.log = append(c.log, LogLine{Level: level, Text: text})
	if len(c.log) > maxLogLines {
		c.log = c.log[len(c.log)-maxLogLines:]
	}
}

// Configure applies fn to the settings. It's refused while a series loads or
// a download runs, so the goroutines reading the settings never see a write.
func (c *Controller) Configure(fn func(*grabber.Settings)) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	fn(c.settings)
	c.mu.Unlock()
	c.emit()
	return nil
}

// Settings returns a copy of the current settings
func (c *Controller) Settings() grabber.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.settings
}

// Load identifies the site for url and fetches its title and chapter list.
// It replaces any loaded series and its selection.
func (c *Controller) Load(url string) error {
	if err := c.BeginLoad(url); err != nil {
		return err
	}
	return c.FinishLoad(url)
}

// BeginLoad marks a load as started, synchronously, so the view can disable
// its controls before the slow part (FinishLoad) runs on another goroutine.
func (c *Controller) BeginLoad(url string) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	c.loading = true
	c.loadErr = nil
	// a fresh settings pointer: the previous series keeps its own, and no
	// Configure can run while this load reads it
	s := new(grabber.Settings)
	*s = *c.settings
	c.settings = s
	// Reuse the existing site for a filter-only reload only when the grabber
	// explicitly opts in via grabber.Refilterable (same URL, series loaded,
	// filter changed). Grabbers that must re-fetch to apply their filter
	// (mangadex, mangafire, mangaplus) are always re-identified.
	if c.series != nil && url == c.url &&
		(s.Language != c.loadedLanguage || s.Scanlator != c.loadedScanlator) {
		if _, ok := c.series.Site.(grabber.Refilterable); ok {
			c.reuseOnLoad = c.series.Site
		} else {
			c.reuseOnLoad = nil
		}
	} else {
		c.reuseOnLoad = nil
	}
	c.url = url
	c.mu.Unlock()
	c.emit()
	return nil
}

// abortLoad ends a load that panicked, so the window isn't left locked
// waiting for it. It's the same outcome as FinishLoad failing.
func (c *Controller) abortLoad(err error) {
	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	c.series = nil
	c.selected = nil
	c.selGen++
	c.mu.Unlock()
	c.emit()
}

// FinishLoad does the work BeginLoad started. Call it exactly once after a
// successful BeginLoad.
func (c *Controller) FinishLoad(url string) error {
	c.mu.Lock()
	s := c.settings
	c.mu.Unlock()

	// a browser still closing after Clear would fail the load
	c.closing.Wait()
	// the browser's visibility is read when it starts, and loading a site
	// may be what starts it
	browser.SetVisible(s.BrowserVisible)
	release := browser.Hold()
	defer release()
	err := c.load(url, s)

	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	if err != nil {
		c.series = nil
		c.selected = nil
		c.selGen++
	}
	c.mu.Unlock()
	c.emit()
	return err
}

func (c *Controller) load(url string, s *grabber.Settings) error {
	// Take and clear the reuse-site set by BeginLoad so it never bleeds
	// into a later load, then skip re-identifying when it's available.
	c.mu.Lock()
	reuse := c.reuseOnLoad
	c.reuseOnLoad = nil
	c.mu.Unlock()

	var site grabber.Site
	if reuse != nil {
		// Update the site's settings pointer to the new one so any options
		// configured between loads are visible to FetchChapter and helpers.
		if su, ok := reuse.(interface{ SetSettings(*grabber.Settings) }); ok {
			su.SetSettings(s)
		}
		site = reuse
	} else {
		var siteErrs []error
		site, siteErrs = c.identify(url, s)
		for _, err := range siteErrs {
			c.addLog(LogWarn, "site check: "+err.Error())
		}
		if site == nil {
			c.mu.Lock()
			c.languages, c.scanlators = nil, nil
			c.mu.Unlock()
			return errors.New("site not recognised")
		}
	}

	series, err := core.Open(site, s)
	if err != nil {
		// the reload gets a site of its own, with the settings dropUnmatchedFilters
		// chose: the failed one may already have read the filters it was given
		if reload, dropLanguage := c.dropUnmatchedFilters(site, s, err); reload {
			// the site keeps the pointer it's identified with, and Configure
			// changes c.settings: a copy would leave the download ignoring
			// later option changes. So the language is cleared in place just
			// for the retry (nothing else touches s while a load runs).
			typed := s.Language
			if dropLanguage {
				c.mu.Lock()
				s.Language = ""
				c.mu.Unlock()
			}
			// a Refilterable site re-applies the filters to what it already
			// fetched: a fresh one would walk the whole list again
			retry := site
			if _, ok := site.(grabber.Refilterable); !ok {
				retry, _ = c.identify(url, s)
			}
			if retry != nil {
				site = retry
				series, err = core.Open(site, s)
			}
			if dropLanguage {
				c.mu.Lock()
				s.Language = typed
				c.mu.Unlock()
			}
		}
	}

	// the options are kept whether or not the load worked; a series already
	// carries them, and asking the site again costs requests
	var languages, scanlators *grabber.FilterOptions
	if series != nil {
		languages, scanlators = series.Languages, series.Scanlators
	} else {
		languages, scanlators = core.FilterOptionsOf(site)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.languages, c.scanlators = languages, scanlators
	// a value typed in another case takes the site's spelling, so the options
	// saved, the loaded filters and the selector all agree on it
	s.Language = listedSpelling(s.Language, languages)
	s.Scanlator = listedSpelling(s.Scanlator, scanlators)
	// a language dropped for this load on a site that lists none is recorded as
	// typed: no selector shows it, and the field would read as stale otherwise
	c.loadedLanguage = s.Language
	c.loadedScanlator = s.Scanlator
	if err != nil {
		return err
	}
	c.series = series
	c.selGen++
	c.selected = make([]bool, len(series.Chapters))
	c.hasDuplicates = core.HasDuplicateChapterNumbers(series.Chapters)
	c.siteVolumes = core.SiteVolumes(series.Chapters)
	c.volumesText = c.siteVolumes
	c.resetResultsLocked()
	c.appendLogLocked(LogInfo, fmt.Sprintf("Loaded %s: %d chapters", series.Title, len(series.Chapters)))
	return nil
}

// resetResultsLocked forgets the last run's jobs and summary, so the previous
// series' results don't show next to a new title. The caller holds c.mu.
func (c *Controller) resetResultsLocked() {
	c.jobs, c.bundle, c.byChap, c.failedC = nil, nil, nil, nil
	c.byVolume, c.listedVol, c.chapVol = nil, nil, nil
	c.summary = nil
	c.runinfo = RunInfo{}
	c.stats = runStats{}
}

// dropUnmatchedFilters handles a load that found nothing for a filter the
// series doesn't have, which the window would otherwise just fail on: each
// such filter is reset to its fallback (see unmatchedFallback) and logged, and
// whether to reload is returned, and whether to do so without the language. It
// only looks at errors that mean "no chapters for this filter": a network
// failure keeps the value.
//
// A language on a site that lists none is dropped for this load only: the
// saved language stays, for the other sites that take it. The caller clears it
// on the shared settings for the reload only and puts it back. Such a site can't say which languages it has, so only
// a NoChaptersError with no scanlator to blame is taken as the language's
// doing: a scanlator the site lists, or none, can't have emptied the list. A
// typed scanlator it doesn't list is reset instead, so the language is kept.
func (c *Controller) dropUnmatchedFilters(site grabber.Site, s *grabber.Settings, err error) (reload, dropLanguage bool) {
	var noChapters *core.NoChaptersError
	var chaptersErr *core.ChaptersError
	var unavailable *grabber.UnavailableFilterError
	// a site that picks the filter while fetching the title fails there
	// instead, with an error of its own
	rejected := errors.As(err, &unavailable)
	if !errors.As(err, &noChapters) && !errors.As(err, &chaptersErr) && !rejected {
		return false, false
	}

	languages, scanlators := core.FilterOptionsOf(site)
	// s is the settings the controller holds, so the typed values are read
	// before they're overwritten
	typedLanguage, typedScanlator := s.Language, s.Scanlator
	language, languageReset := unmatchedFallback(typedLanguage, languages)
	scanlator, scanlatorReset := unmatchedFallback(typedScanlator, scanlators)
	scanlatorCleared := typedScanlator == "" || typedScanlator == grabber.FilterAll || listsValue(scanlators, typedScanlator)
	languageDropped := !languageReset && !listsValues(languages) && (noChapters != nil || (rejected && unavailable.Filter == "language")) &&
		typedLanguage != "" && typedLanguage != grabber.FilterAll && scanlatorCleared
	if !languageReset && !scanlatorReset && !languageDropped {
		return false, false
	}

	c.mu.Lock()
	if languageReset {
		c.settings.Language = language
	}
	if scanlatorReset {
		c.settings.Scanlator = scanlator
	}
	c.mu.Unlock()
	if languageReset {
		c.addLog(LogInfo, fallbackMessage("language", typedLanguage, language))
	}
	if scanlatorReset {
		c.addLog(LogInfo, fallbackMessage("scanlation group", typedScanlator, scanlator))
	}

	if languageDropped {
		c.addLog(LogInfo, fallbackMessage("language", typedLanguage, ""))
	}

	return true, languageDropped
}

// listsValue reports whether the site offers value for a filter, ignoring case
func listsValue(options *grabber.FilterOptions, value string) bool {
	if !listsValues(options) {
		return false
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return true
		}
	}

	return false
}

// unmatchedFallback is what a typed filter value falls back to when the series
// doesn't offer it: "all" where the site takes it, the site's Current value
// otherwise, and no filter ("") when it has neither. reset is false when
// there's nothing to fall back from: no value, "all" itself, a site that lists
// nothing, or a value the series does offer.
func unmatchedFallback(value string, options *grabber.FilterOptions) (fallback string, reset bool) {
	if value == "" || value == grabber.FilterAll || !listsValues(options) {
		return "", false
	}
	if listsValue(options, value) {
		return "", false
	}
	if options.All {
		return grabber.FilterAll, true
	}

	return options.Current, true
}

// listsValues reports whether the site offers any value for a filter: the
// selector isn't shown otherwise (see newFilter), so nothing can be matched
func listsValues(options *grabber.FilterOptions) bool {
	return options != nil && len(options.Values) > 0
}

// fallbackMessage is the log line for a filter that was reset, naming what
// the load went on with instead
func fallbackMessage(kind, value, fallback string) string {
	shown := fmt.Sprintf("%q", fallback)
	switch fallback {
	case grabber.FilterAll:
		shown = "all of them"
	case "":
		shown = "no filter"
	}

	return fmt.Sprintf("%s %q isn't in this series, loading %s instead", kind, value, shown)
}

// Filter is what a filter's selector offers for the loaded series
type Filter struct {
	// Values are the values the site lists for the filter
	Values []string
	// All is set when the site also takes "all" (every value, mixed)
	All bool
	// Selected is the value the loaded list shows: the one it was loaded with,
	// or else what the site shows by default (see selectedFilter)
	Selected string
}

// Filters returns the language and scanlator selectors: nil for a filter the
// site doesn't list, or when no site was identified
func (c *Controller) Filters() (language, scanlator *Filter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var chapters grabber.Filterables
	if c.series != nil {
		chapters = c.series.Chapters
	}

	language = newFilter(c.languages, selectedFilter(c.loadedLanguage, c.languages, commonLanguage(chapters)))
	scanlator = newFilter(c.scanlators, selectedFilter(c.loadedScanlator, c.scanlators, ""))

	return language, scanlator
}

// newFilter builds the selector of one filter, nil when the site doesn't list it
func newFilter(options *grabber.FilterOptions, selected string) *Filter {
	if options == nil || len(options.Values) == 0 {
		return nil
	}

	return &Filter{
		Values:   append([]string(nil), options.Values...),
		All:      options.All,
		Selected: selected,
	}
}

// listedSpelling returns the spelling the site lists value under when it
// matches one ignoring case ("delta" is "Delta"), value itself otherwise
func listedSpelling(value string, options *grabber.FilterOptions) string {
	if options == nil {
		return value
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return v
		}
	}

	return value
}

// selectedFilter is the value the loaded list shows for one filter. A value
// the list was loaded with is what it shows, in the site's spelling. With none,
// the site's default shows: Current when the site has one, else the language
// every chapter shares (sites whose default is one language, like "en"), else
// every value when the site takes "all". uniform is commonLanguage, or "" for a
// filter whose chapters don't say.
func selectedFilter(loaded string, options *grabber.FilterOptions, uniform string) string {
	switch {
	case loaded != "" || options == nil:
		return listedSpelling(loaded, options)
	case options.Current != "":
		return options.Current
	case uniform != "":
		return uniform
	case options.All:
		return grabber.FilterAll
	}

	return ""
}

// commonLanguage returns the language every chapter reports, "" when they
// differ or don't report one
func commonLanguage(chapters grabber.Filterables) string {
	lang := ""
	for i, chap := range chapters {
		l, ok := chap.(interface{ GetLanguage() string })
		if !ok {
			return ""
		}
		if i == 0 {
			lang = l.GetLanguage()
		} else if l.GetLanguage() != lang {
			return ""
		}
	}

	return lang
}

// Clear forgets the loaded series: its URL, title, chapters, selection, the
// outcome of its load and the last run's results (the Activity tab), so the
// window starts from nothing. The options and the log stay. It's refused while a
// series loads or a download runs.
func (c *Controller) Clear() error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	c.url = ""
	c.series = nil
	c.selected = nil
	c.selGen++
	c.hasDuplicates = false
	c.volumesText, c.siteVolumes = "", ""
	c.languages, c.scanlators = nil, nil
	c.loadedLanguage, c.loadedScanlator = "", ""
	c.loadErr = nil
	c.attention = nil
	c.resetResultsLocked()
	// done with the series: its browser (and maybe window) can go. Nothing
	// can be using it, as Clear is refused while loading or running. It's
	// closed in the background, as that can take seconds; added under mu so
	// a load that starts next is sure to wait for it.
	c.closing.Add(1)
	c.mu.Unlock()
	c.emit()
	go func() {
		defer c.closing.Done()
		c.closeBrowser()
	}()
	return nil
}

// Stale reports whether the language or scanlator filter changed since the
// loaded list was fetched, which means the list must be reloaded to match. A
// load already running is the reload that applies the change, so it isn't stale.
func (c *Controller) Stale() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil && !c.loading && (c.settings.Language != c.loadedLanguage || c.settings.Scanlator != c.loadedScanlator)
}

// URL is the last URL passed to Load
func (c *Controller) URL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.url
}

// Loaded reports whether a series is loaded
func (c *Controller) Loaded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil
}

// LoadError is the error of the last load, nil when it succeeded
func (c *Controller) LoadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadErr
}

// Cancelling reports whether a run has been asked to stop
func (c *Controller) Cancelling() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelling
}

// SelectionGen changes whenever the selection does (see selGen)
func (c *Controller) SelectionGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selGen
}

// LogGen changes whenever a line is added to the log (see logGen)
func (c *Controller) LogGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logGen
}

// LoadedFilters returns the language and scanlator the loaded list was
// fetched with
func (c *Controller) LoadedFilters() (language, scanlator string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadedLanguage, c.loadedScanlator
}

// Loading reports whether a series is being loaded
func (c *Controller) Loading() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loading
}

// Running reports whether a download is running
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Title is the loaded series' title
func (c *Controller) Title() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return ""
	}
	return c.series.Title
}

// Chapters returns the loaded chapter list, sorted by number. The list is
// never modified after Load, so the slice is shared with the caller.
func (c *Controller) Chapters() grabber.Filterables {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return nil
	}
	return c.series.Chapters
}

// HasDuplicates reports whether the list repeats chapter numbers across
// languages, so the view should show each chapter's language
func (c *Controller) HasDuplicates() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasDuplicates
}

// SelectedIndices returns the indices of the selected chapters, in list order,
// read under one lock so a concurrent load can't interleave with it
func (c *Controller) SelectedIndices() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []int{}
	for i, on := range c.selected {
		if on {
			out = append(out, i)
		}
	}
	return out
}

// IsSelected reports whether chapter i of Chapters is selected
func (c *Controller) IsSelected(i int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return i >= 0 && i < len(c.selected) && c.selected[i]
}

// SelectedCount returns how many chapters are selected
func (c *Controller) SelectedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, on := range c.selected {
		if on {
			n++
		}
	}
	return n
}

// SetSelected selects or deselects chapter i. It's refused while a series
// loads or a download runs, as SetSelectedMany is.
func (c *Controller) SetSelected(i int, on bool) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	if i < 0 || i >= len(c.selected) {
		c.mu.Unlock()
		return nil
	}
	c.selected[i] = on
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetSelectedMany selects or deselects the chapters at indices in one step,
// with one change and one push. Every index is checked before any is set: one
// out of range rejects the whole call. It's refused while a series loads or a
// download runs, as Configure is.
func (c *Controller) SetSelectedMany(indices []int, on bool) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	for _, i := range indices {
		if i < 0 || i >= len(c.selected) {
			c.mu.Unlock()
			return fmt.Errorf("chapter %d is not in the list", i)
		}
	}
	for _, i := range indices {
		c.selected[i] = on
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetAll selects or deselects every chapter. It's refused while a series
// loads or a download runs.
func (c *Controller) SetAll(on bool) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	for i := range c.selected {
		c.selected[i] = on
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SelectRange selects the chapters whose number falls in the range text
// (e.g. "1-10,12"), leaving the rest of the selection as it is. It returns how
// many chapters the range matched. It's refused while a series loads or a
// download runs.
func (c *Controller) SelectRange(text string) (int, error) {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return 0, errBusy
	}
	// parsed under the lock: it's cheap, and the busy check must come first
	rngs, err := ranges.Parse(text)
	if err != nil {
		c.mu.Unlock()
		return 0, err
	}
	n := 0
	if c.series != nil {
		for i, chap := range c.series.Chapters {
			if inRanges(chap.GetNumber(), rngs) {
				c.selected[i] = true
				n++
			}
		}
		c.selGen++
	}
	c.mu.Unlock()
	c.emit()
	return n, nil
}

func inRanges(n float64, rngs []ranges.Range) bool {
	for _, r := range rngs {
		if n >= r.Begin && n <= r.End {
			return true
		}
	}
	return false
}

// Volumes returns the Volumes box and the site's volumes, in --volumes'
// syntax; site is "" when the site doesn't say
func (c *Controller) Volumes() (text, site string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.volumesText, c.siteVolumes
}

// SetVolumes sets the Volumes box: chapter ranges, one group per volume
// ("1-7/8-16", "16:112-114"), "auto" or "" for the site's own. A value that doesn't parse
// is refused, as is any change while a series loads or a download runs.
func (c *Controller) SetVolumes(text string) error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	if _, err := core.ParseVolumes(text); err != nil {
		c.mu.Unlock()
		return err
	}
	c.volumesText = strings.TrimSpace(text)
	c.mu.Unlock()
	c.emit()
	return nil
}

// runVolumes is the --volumes value a run in volume mode packs: the box,
// or the site's volumes when it's empty. Callers hold mu.
func (c *Controller) runVolumes() string {
	if c.volumesText == "" {
		return core.VolumesAuto
	}
	return c.volumesText
}

// Run describes the current or last run
func (c *Controller) Run() RunInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runinfo
}

// Jobs returns a copy of the progress rows of the current or last run, in
// chapter order (completion order is what the events arrive in)
func (c *Controller) Jobs() []Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Job, len(c.jobs))
	for i, j := range c.jobs {
		out[i] = *j
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// Summary returns the outcome of the last run, or nil while one runs or
// before the first run
func (c *Controller) Summary() *Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.summary == nil {
		return nil
	}
	s := *c.summary
	s.Failures = append([]string(nil), c.summary.Failures...)
	return &s
}

// Log returns a copy of the log
func (c *Controller) Log() []LogLine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogLine(nil), c.log...)
}

// Start validates the settings and starts downloading the selected chapters
// in the background. Validation errors are returned synchronously.
func (c *Controller) Start() error {
	c.mu.Lock()
	if c.loading || c.running {
		c.mu.Unlock()
		return errBusy
	}
	if c.series == nil {
		c.mu.Unlock()
		return errors.New("load a series first")
	}
	if c.settings.Language != c.loadedLanguage || c.settings.Scanlator != c.loadedScanlator {
		c.mu.Unlock()
		return errors.New("the language or scanlator changed: reload the series first")
	}
	// volume mode packs the Volumes box, which isn't saved with the options
	volumes := c.settings.Volumes != ""
	if volumes {
		c.settings.Volumes = c.runVolumes()
	}
	skip, err := core.Validate(c.settings)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	var chapters grabber.Filterables
	var numbers []float64
	var selected []bool
	for i, chap := range c.series.Chapters {
		numbers = append(numbers, chap.GetNumber())
		selected = append(selected, c.selected[i])
		if c.selected[i] {
			chapters = append(chapters, chap)
		}
	}
	if len(chapters) == 0 {
		c.mu.Unlock()
		return errors.New("select at least one chapter")
	}
	rows := len(chapters)
	if c.settings.Bundle {
		rows = 1
	}
	if volumes {
		// grouped here too, so a site without volumes is refused before the
		// run starts rather than failing it
		spec, _ := core.ParseVolumes(c.settings.Volumes)
		groups, loose, err := core.GroupVolumes(chapters, spec)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		rows = len(groups) + len(loose)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.running = true
	c.rng = bundleRange(numbers, selected)
	c.runinfo = RunInfo{Chapters: len(chapters), Bundle: c.settings.Bundle, Volumes: volumes, Rows: rows}
	c.cancelling = false
	c.jobs = nil
	c.byChap = map[*grabber.Chapter]*Job{}
	c.failedC = map[*grabber.Chapter]bool{}
	c.byVolume = map[*core.Volume]*Job{}
	c.listedVol = map[grabber.Filterable]*Job{}
	c.chapVol = map[*grabber.Chapter]*Job{}
	c.bundle = nil
	c.stats = runStats{}
	c.summary = nil
	if c.settings.Bundle {
		c.bundle = &Job{Title: "Bundle: chapters " + c.rng, Status: "Gathering info..."}
		c.jobs = append(c.jobs, c.bundle)
	}
	series := c.series
	settings := c.settings
	rng := c.rng
	c.appendLogLocked(LogInfo, fmt.Sprintf("Downloading %d chapter(s) of %s", len(chapters), series.Title))
	c.mu.Unlock()
	c.emit()

	// the browser's visibility is read when it starts, so it must follow the
	// option as of this run, not the one in force when the series was loaded
	browser.SetVisible(settings.BrowserVisible)
	// Hold keeps the browser open; releasing closes it when the download is done
	release := browser.Hold()
	go func() {
		defer release()
		c.run(ctx, series, settings, chapters, skip, rng)
	}()
	return nil
}

// run is the download goroutine
func (c *Controller) run(ctx context.Context, series *core.Series, settings *grabber.Settings, chapters grabber.Filterables, skip []ranges.Range, rng string) {
	file, err := series.Download(ctx, chapters, skip, rng, events{c})

	c.mu.Lock()
	defer func() {
		c.mu.Unlock()
		c.emit()
	}()

	s := &Summary{
		Dir:      settings.OutputDir,
		Bundled:  c.bundle != nil,
		Saved:    c.stats.saved,
		Volumes:  c.stats.volumes,
		Skipped:  c.stats.skipped,
		Failures: c.stats.failures,
	}
	switch {
	case err != nil && c.runinfo.Volumes && !errors.Is(err, context.Canceled):
		// only the volumes check Start already ran can fail a volume run
		s.Err = err
		s.Failures = append(s.Failures, "volumes: "+err.Error())
		c.appendLogLocked(LogError, "volumes: "+err.Error())
	case errors.Is(err, context.Canceled):
		// a cancelled bundle is never packed: the chapters it never started
		// would be silently missing from it
		s.Cancelled = true
	case err != nil:
		s.Err = err
		s.Failures = append(s.Failures, "packing bundle: "+err.Error())
		c.appendLogLocked(LogError, "packing bundle: "+err.Error())
	case c.bundle != nil && c.stats.saved == 0:
		// core still packs an empty bundle when every chapter failed (as the
		// CLI does), so the file it leaves behind is not a result
		s.Err = errors.New("no chapter could be downloaded")
		s.Failures = append(s.Failures, "packing bundle: no chapter could be downloaded")
		c.appendLogLocked(LogError, "packing bundle: no chapter could be downloaded")
	default:
		s.Bundle = file
	}
	if c.bundle != nil {
		if s.Bundle == "" {
			c.bundle.State = JobFailed
		} else {
			c.bundle.State = JobDone
			c.bundle.Done = c.bundle.Total
		}
		c.bundle.Status = ""
	}
	c.summary = s
	c.running = false
	// released under the lock, in the same step that clears running: once
	// running is false a new Start may replace c.cancel
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

// Cancel stops the run from starting new chapters. Chapters already in
// flight finish. It's a no-op when nothing runs or a cancel already happened.
func (c *Controller) Cancel() {
	c.mu.Lock()
	cancel := c.cancel
	first := cancel != nil && !c.cancelling
	if first {
		c.cancelling = true
		c.appendLogLocked(LogInfo, "Cancelling: chapters already downloading will finish, no new ones start")
	}
	c.mu.Unlock()
	if first {
		cancel()
		c.emit()
	}
}

// shutdownWait bounds how long quitting waits for a run to wind down and for
// the browser to close, so a stuck render can't hang the window's exit
const shutdownWait = 3 * time.Second

// Shutdown cancels any run and closes the browser, so Chrome doesn't outlive
// the app. It waits at most shutdownWait for each step. Safe to call more
// than once.
func (c *Controller) Shutdown() {
	c.Cancel()
	deadline := time.Now().Add(shutdownWait)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() {
		browser.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
	}
}

// events maps core.Events onto the controller's state. Each method takes the
// lock itself; core calls them from several goroutines at once.
type events struct{ c *Controller }

var _ core.Events = events{}

// jobFor returns the row of a chapter, nil when it has none (bundle mode has
// a single row, and a chapter whose fetch failed never got one).
// Callers must hold mu.
func (c *Controller) jobFor(ch *grabber.Chapter) *Job {
	if c.bundle != nil {
		return c.bundle
	}
	if j := c.chapVol[ch]; j != nil {
		return j
	}
	return c.byChap[ch]
}

// live reports whether a chapter still accepts page progress: once it failed,
// pages still in flight finish but must not move its bar. Callers hold mu.
func (c *Controller) live(ch *grabber.Chapter) bool {
	return !c.failedC[ch]
}

// advance moves a row into the archiving phase at its first archived step,
// the point the CLI's bar switches its label: the downloads fill the bar up
// to the download share and no further. Callers hold mu.
func advance(j *Job) {
	if j.State == JobDownloading && j.Done > j.Total*core.DownloadWeight/(core.DownloadWeight+core.ArchiveWeight) {
		j.State = JobArchiving
	}
}

func (e events) BundleStart(pages int64) {
	c := e.c
	c.mu.Lock()
	if c.bundle != nil {
		c.bundle.Total = pages * (core.DownloadWeight + core.ArchiveWeight)
		c.bundle.Status = "Downloading..."
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) FetchFailed(listed grabber.Filterable, err error) {
	c := e.c
	c.mu.Lock()
	msg := fmt.Sprintf("%s: %s", listed.GetTitle(), err)
	c.stats.failures = append(c.stats.failures, msg)
	if c.bundle == nil && c.listedVol[listed] == nil {
		c.jobs = append(c.jobs, &Job{Title: listed.GetTitle(), Number: listed.GetNumber(), State: JobFailed, Reason: err.Error()})
	}
	c.appendLogLocked(LogError, "error fetching chapter "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Skipped(ch *grabber.Chapter) {
	c := e.c
	c.mu.Lock()
	c.stats.skipped++
	if c.bundle == nil && c.chapVol[ch] == nil {
		c.jobs = append(c.jobs, &Job{Title: ch.GetTitle(), Number: ch.GetNumber(), State: JobSkipped, Reason: "every page excluded by --skip-pages"})
	}
	c.appendLogLocked(LogWarn, fmt.Sprintf("skipping %s: every page is excluded by --skip-pages", ch.GetTitle()))
	c.mu.Unlock()
	c.emit()
}

func (e events) FilenameFailed(ch *grabber.Chapter, err error) {
	c := e.c
	c.mu.Lock()
	msg := fmt.Sprintf("%s: filename: %s", ch.GetTitle(), err)
	c.stats.failures = append(c.stats.failures, msg)
	if c.bundle == nil {
		j := &Job{Title: ch.GetTitle(), Number: ch.GetNumber(), State: JobFailed, Reason: "filename: " + err.Error()}
		c.jobs = append(c.jobs, j)
	}
	c.appendLogLocked(LogError, "error creating filename for "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Started(ch *grabber.Chapter) {
	c := e.c
	c.mu.Lock()
	if c.bundle != nil {
		c.bundle.Status = "Downloading " + ch.GetTitle()
	} else if j := c.chapVol[ch]; j != nil {
		// its row is too narrow for the chapter's title: the phase and
		// percentage say enough. The bar moves with VolumeProgress.
		j.Status = ""
	} else {
		// the CLI's bar: each page's download and archive steps
		j := &Job{Title: ch.GetTitle(), Number: ch.GetNumber(), Total: ch.PagesCount * (core.DownloadWeight + core.ArchiveWeight), State: JobDownloading}
		c.jobs = append(c.jobs, j)
		c.byChap[ch] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Page(ch *grabber.Chapter, idx int, err error) {
	c := e.c
	c.mu.Lock()
	if err != nil {
		c.appendLogLocked(LogError, fmt.Sprintf("error downloading page %d of %s: %s", idx+1, ch.GetTitle(), err))
	} else if j := c.jobFor(ch); j != nil && !j.Volume && c.live(ch) {
		// a volume's row moves with VolumeProgress instead
		j.Done += core.DownloadWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) DownloadFailed(ch *grabber.Chapter, err error) {
	c := e.c
	c.mu.Lock()
	c.failedC[ch] = true
	msg := fmt.Sprintf("%s: %s", ch.GetTitle(), err)
	c.stats.failures = append(c.stats.failures, msg)
	// a volume goes on with its other chapters
	if j := c.byChap[ch]; j != nil && c.bundle == nil {
		j.State = JobFailed
		j.Reason = "download: " + err.Error()
	}
	c.appendLogLocked(LogError, "error downloading chapter "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Archived(ch *grabber.Chapter) {
	c := e.c
	c.mu.Lock()
	if j := c.byChap[ch]; j != nil && c.bundle == nil && j.State != JobFailed {
		j.Done += core.ArchiveWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Packed(ch *grabber.Chapter, name string, err error) {
	c := e.c
	c.mu.Lock()
	j := c.byChap[ch]
	if err != nil {
		msg := fmt.Sprintf("%s: packing: %s", ch.GetTitle(), err)
		c.stats.failures = append(c.stats.failures, msg)
		if j != nil {
			j.State = JobFailed
			j.Reason = "packing: " + err.Error()
		}
		c.appendLogLocked(LogError, msg)
	} else {
		c.stats.saved++
		if j != nil {
			j.State = JobDone
			// the skipped pages were never counted, so the bar is exact only now
			j.Done = j.Total
		}
		c.appendLogLocked(LogInfo, "saved "+name)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Downloaded(ch *grabber.Chapter) {
	c := e.c
	c.mu.Lock()
	// the pages were counted as they came in; this only tallies the chapter
	c.stats.saved++
	c.mu.Unlock()
	c.emit()
}

func (e events) BundlePacking() {
	c := e.c
	c.mu.Lock()
	if c.bundle != nil {
		c.bundle.Status = "Creating bundle for chapters " + c.rng
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) BundleArchived(n int) {
	c := e.c
	c.mu.Lock()
	if c.bundle != nil {
		c.bundle.Done += int64(n) * core.ArchiveWeight
	}
	c.mu.Unlock()
	c.emit()
}

// volumeTitle is the row title of a volume: "Vol. 01 · chapters 1–7"
func volumeTitle(v *core.Volume) string {
	first, last := formatNumber(v.FirstNumber()), formatNumber(v.LastNumber())
	if first == last {
		return fmt.Sprintf("%s · chapter %s", core.VolumeLabel(v.Number), first)
	}
	return fmt.Sprintf("%s · chapters %s–%s", core.VolumeLabel(v.Number), first, last)
}

func (e events) VolumeStart(v *core.Volume) {
	c := e.c
	c.mu.Lock()
	j := &Job{Title: volumeTitle(v), Number: v.FirstNumber(), Total: volumeScale, State: JobDownloading, Status: "Waiting...", Volume: true}
	c.jobs = append(c.jobs, j)
	c.byVolume[v] = j
	for _, chap := range v.Chapters {
		c.listedVol[chap] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumeChapter(v *core.Volume, ch *grabber.Chapter) {
	c := e.c
	c.mu.Lock()
	if j := c.byVolume[v]; j != nil {
		c.chapVol[ch] = j
	}
	c.mu.Unlock()
}

func (e events) VolumePacking(v *core.Volume) {
	c := e.c
	c.mu.Lock()
	if j := c.byVolume[v]; j != nil {
		j.State = JobArchiving
		j.Status = "Packing..."
	}
	c.mu.Unlock()
	c.emit()
}

// volumeScale is the Total of a volume's row, whose Done is VolumeProgress's
// fraction of it
const volumeScale = 10000

func (e events) VolumeProgress(v *core.Volume, fraction float64) {
	c := e.c
	c.mu.Lock()
	// core never reports a volume going back; max keeps the row honest even
	// so, and leaves a row that already ended alone
	if j := c.byVolume[v]; j != nil && j.State != JobDone && j.State != JobFailed && j.State != JobSkipped {
		j.Done = max(j.Done, int64(fraction*volumeScale))
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumePacked(v *core.Volume, name string, err error) {
	c := e.c
	c.mu.Lock()
	j := c.byVolume[v]
	label := core.VolumeLabel(v.Number)
	switch {
	case errors.Is(err, context.Canceled):
		// a cancelled run says so in its summary; the row only stops
		if j != nil {
			j.State = JobSkipped
			j.Reason = "cancelled before all of its chapters started"
		}
	case errors.Is(err, core.ErrSkippedVolume):
		// its chapters were each counted as skipped already
		if j != nil {
			j.State = JobSkipped
			j.Reason = err.Error()
		}
	case err != nil:
		msg := fmt.Sprintf("%s: %s", label, err)
		c.stats.failures = append(c.stats.failures, msg)
		if j != nil {
			j.State = JobFailed
			j.Reason = err.Error()
		}
		c.appendLogLocked(LogError, "error packing "+msg)
	default:
		c.stats.volumes++
		if j != nil {
			j.State = JobDone
			j.Done = j.Total
		}
		c.appendLogLocked(LogInfo, "saved "+name)
	}
	if j != nil {
		j.Status = ""
	}
	c.mu.Unlock()
	c.emit()
}
