// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
// the whole bundle, whose Total counts every page twice (download + archive).
// The CLI's bar also adds each kept chapter's pages a second time, so it runs
// past its total; the GUI counts each page once per phase.
type Job struct {
	Title  string
	Number float64
	Total  int64
	Done   int64
	State  JobState
	Reason string
	// Status is the bundle's phase ("Downloading <chapter>", "Creating
	// bundle..."); unused for single chapters
	Status string
}

// RunInfo describes the run in progress, or the last one
type RunInfo struct {
	// Chapters is how many chapters the run was started with
	Chapters int
	Bundle   bool
}

// Summary is what a finished run produced
type Summary struct {
	Dir       string
	Cancelled bool
	// Saved counts chapters written to disk (or, in bundle mode, kept for the bundle)
	Saved int
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

	// loadErr is the last load's error, which the view shows under the URL
	loadErr error
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
	stats   runStats
	summary *Summary
	log     []LogLine
}

// NewController returns a controller that works on its own copy of settings
// and routes the library's warnings into its log.
func NewController(settings *grabber.Settings, notifyFn func()) *Controller {
	if notifyFn == nil {
		notifyFn = func() {}
	}
	c := &Controller{
		notify:   notifyFn,
		identify: core.Identify,
		settings: settings,
	}
	notice.SetHandler(func(level notice.Level, msg string) {
		lvl := LogInfo
		if level == notice.LevelWarn {
			lvl = LogWarn
		}
		c.addLog(lvl, msg)
	})
	return c
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
	site, siteErrs := c.identify(url, s)
	for _, err := range siteErrs {
		c.addLog(LogWarn, "site check: "+err.Error())
	}
	if site == nil {
		return errors.New("site not recognised")
	}

	series, err := core.Open(site, s)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.series = series
	c.selGen++
	c.selected = make([]bool, len(series.Chapters))
	c.hasDuplicates = core.HasDuplicateChapterNumbers(series.Chapters)
	c.loadedLanguage = s.Language
	c.loadedScanlator = s.Scanlator
	// the previous series' results would otherwise show next to this title
	c.jobs, c.bundle, c.byChap, c.failedC = nil, nil, nil, nil
	c.summary = nil
	c.runinfo = RunInfo{}
	c.stats = runStats{}
	c.appendLogLocked(LogInfo, fmt.Sprintf("Loaded %s: %d chapters", series.Title, len(series.Chapters)))
	return nil
}

// Stale reports whether the language or scanlator filter changed since the
// loaded list was fetched, which means the list must be reloaded to match.
func (c *Controller) Stale() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil && (c.settings.Language != c.loadedLanguage || c.settings.Scanlator != c.loadedScanlator)
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

// SetSelected selects or deselects chapter i
func (c *Controller) SetSelected(i int, on bool) {
	c.mu.Lock()
	if i < 0 || i >= len(c.selected) {
		c.mu.Unlock()
		return
	}
	c.selected[i] = on
	c.selGen++
	c.mu.Unlock()
	c.emit()
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

// SetAll selects or deselects every chapter
func (c *Controller) SetAll(on bool) {
	c.mu.Lock()
	for i := range c.selected {
		c.selected[i] = on
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
}

// SelectRange selects the chapters whose number falls in the range text
// (e.g. "1-10,12"), leaving the rest of the selection as it is. It returns how
// many chapters the range matched.
func (c *Controller) SelectRange(text string) (int, error) {
	rngs, err := ranges.Parse(text)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
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

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.running = true
	c.rng = bundleRange(numbers, selected)
	c.runinfo = RunInfo{Chapters: len(chapters), Bundle: c.settings.Bundle}
	c.cancelling = false
	c.jobs = nil
	c.byChap = map[*grabber.Chapter]*Job{}
	c.failedC = map[*grabber.Chapter]bool{}
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
	go c.run(ctx, series, settings, chapters, skip, rng)
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
		Skipped:  c.stats.skipped,
		Failures: c.stats.failures,
	}
	switch {
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
	return c.byChap[ch]
}

// live reports whether a chapter still accepts page progress: once it failed,
// pages still in flight finish but must not move its bar. Callers hold mu.
func (c *Controller) live(ch *grabber.Chapter) bool {
	return !c.failedC[ch]
}

// advance moves a row into the archiving phase once every page is in, the
// same point the CLI's bar switches its label. Callers hold mu.
func advance(j *Job) {
	if j.State == JobDownloading && j.Done >= j.Total/2 {
		j.State = JobArchiving
	}
}

func (e events) BundleStart(pages int64) {
	c := e.c
	c.mu.Lock()
	if c.bundle != nil {
		c.bundle.Total = pages * 2
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
	if c.bundle == nil {
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
	if c.bundle == nil {
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
	} else {
		// the CLI's bar: pages to download plus pages to archive
		j := &Job{Title: ch.GetTitle(), Number: ch.GetNumber(), Total: ch.PagesCount * 2, State: JobDownloading}
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
	} else if j := c.jobFor(ch); j != nil && c.live(ch) {
		j.Done++
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
		j.Done++
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
		c.bundle.Done += int64(n)
	}
	c.mu.Unlock()
	c.emit()
}
