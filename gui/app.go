// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// refreshInterval bounds how often a "state" event goes out while events
// stream in (at most 10 a second). The last change always goes out too.
const refreshInterval = 100 * time.Millisecond

// logTail is how many log lines a push or State carries; the controller keeps
// more, but the window only needs the recent ones
const logTail = 500

// sink is what the App needs from the Wails runtime. The real one is
// wailsRuntime; tests use a fake that records calls.
type sink interface {
	// Emit sends an event to the frontend. It must not block for long.
	Emit(event string, data any)
	// ChooseDirectory shows the native folder picker, "" when cancelled
	ChooseDirectory(defaultDir string) (string, error)
	// Close makes every later Emit a no-op; the window is going away
	Close()
}

// App is the value bound to the frontend. It's a thin adapter over the
// Controller: it turns the controller's state into DTOs, pushes changes to the
// frontend, and does the few things that need the OS.
//
// Wails runs each bound call on its own goroutine, so App's methods may run
// concurrently. Everything they touch is either the Controller (locked) or
// immutable after construction.
type App struct {
	ctrl *Controller
	sink sink
	// prefsPath is the gui.json the options are saved to, "" when there's no
	// config dir to save them in (the options then last for this run only)
	prefsPath string
	// tag is the build's release tag, "" for a dev build, which skips the
	// update check
	tag     string
	version string
	// updateCtx bounds the update check; cancelUpdate ends it at shutdown
	updateCtx    context.Context
	cancelUpdate context.CancelFunc
	interval     time.Duration

	// saveMu serialises writes to gui.json, so two saves can't race a rename
	saveMu sync.Mutex
	// dirty is a one-slot signal: a pending signal already covers any number
	// of further changes, so notify never blocks
	dirty    chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
}

// newApp wires an App to its controller and sink. The controller's notify
// must be app.notify, which is how the App learns about changes; launch starts
// the pushes once the window exists.
func newApp(ctrl *Controller, s sink, prefsPath, tag, version string) *App {
	updateCtx, cancelUpdate := context.WithCancel(context.Background())
	return &App{
		ctrl:         ctrl,
		sink:         s,
		prefsPath:    prefsPath,
		tag:          tag,
		version:      version,
		updateCtx:    updateCtx,
		cancelUpdate: cancelUpdate,
		interval:     refreshInterval,
		dirty:        make(chan struct{}, 1),
		stop:         make(chan struct{}),
	}
}

// notify is the controller's change signal. It's called from any goroutine.
func (a *App) notify() {
	select {
	case a.dirty <- struct{}{}:
	default:
	}
}

// launch starts pushing state to the frontend. Call it once the window is up.
func (a *App) launch() {
	go a.pump()
}

// pump emits a "state" event after each change, at most once per interval.
// The selection and the log are only sent when they changed since the last
// push, because they can be thousands of entries long while the rest of the
// state is small.
func (a *App) pump() {
	var sentSel, sentLog uint64
	for {
		select {
		case <-a.dirty:
		case <-a.stop:
			return
		}

		a.sink.Emit("state", a.State())
		// read each generation before its content, so a change that lands in
		// between is sent again on the next push rather than lost
		if g := a.ctrl.SelectionGen(); g != sentSel {
			sentSel = g
			a.sink.Emit("selection", a.Selection())
		}
		if g := a.ctrl.LogGen(); g != sentLog {
			sentLog = g
			a.sink.Emit("log", a.Log())
		}

		select {
		case <-time.After(a.interval):
		case <-a.stop:
			return
		}
	}
}

// startup is called by Wails once the window exists, with the context the
// runtime calls need.
func (a *App) startup(ctx context.Context, rt *wailsRuntime) {
	rt.set(ctx)
	a.launch()
	// release builds only: a dev build has no release to compare with. The
	// check runs behind the window, which is up already.
	if a.tag != "" {
		go a.ctrl.CheckUpdate(a.updateCtx, a.tag, latestReleaseAPI, updateClient)
	}
}

// shutdown stops the pushes before the window goes away, then cancels any run
// and closes the browser, as the controller's Shutdown does
func (a *App) shutdown(context.Context) {
	a.stopOnce.Do(func() { close(a.stop) })
	a.cancelUpdate()
	// events still in flight (a load finishing, a push) must not reach a
	// window that's going away
	a.sink.Close()
	a.ctrl.Shutdown()
}

// State is everything the window shows except the chapter list, the
// selection and the log, which have their own calls and events
type State struct {
	URL           string `json:"url"`
	Loading       bool   `json:"loading"`
	Loaded        bool   `json:"loaded"`
	LoadError     string `json:"loadError"`
	Title         string `json:"title"`
	ChapterCount  int    `json:"chapterCount"`
	HasDuplicates bool   `json:"hasDuplicates"`
	// HasGroupDuplicates is set when the list repeats a chapter number in one
	// language with the groups telling the copies apart (see ChapterDTO.Group)
	HasGroupDuplicates bool `json:"hasGroupDuplicates"`
	Stale              bool `json:"stale"`
	// LoadedLanguage and LoadedScanlator are the filters the list was fetched
	// with, so the window can tell which of the two went stale
	LoadedLanguage  string      `json:"loadedLanguage"`
	LoadedScanlator string      `json:"loadedScanlator"`
	SelectedCount   int         `json:"selectedCount"`
	Running         bool        `json:"running"`
	Cancelling      bool        `json:"cancelling"`
	Run             RunDTO      `json:"run"`
	Jobs            []JobDTO    `json:"jobs"`
	Summary         *SummaryDTO `json:"summary"`
	Options         OptionsDTO  `json:"options"`
	// Volumes is the Volumes box, used when the options pack as volumes
	Volumes VolumesDTO `json:"volumes"`
	// Languages and Scanlators are what the two filter selectors offer, null
	// for a filter the site doesn't list
	Languages  *FilterDTO `json:"languages"`
	Scanlators *FilterDTO `json:"scanlators"`
	Version    string     `json:"version"`
	// ReleaseURL is the GitHub release of this build, "" for a dev build
	ReleaseURL string `json:"releaseURL"`
	// Update is the newer release the update check found, null when there's none
	Update *UpdateDTO `json:"update"`
	// BrowserVisibleAuto is set when the loaded site opens the browser
	// visible on its own, whatever Options.BrowserVisible says
	BrowserVisibleAuto bool `json:"browserVisibleAuto"`
	// BrowserAlert is the message of a browser window waiting for the user,
	// "" when none is
	BrowserAlert string `json:"browserAlert"`
}

// VolumesDTO is the Volumes box: the volumes a run packs, in --volumes'
// syntax ("" for the site's own), and the site's volumes in the same syntax,
// "" when the site doesn't say
type VolumesDTO struct {
	Text string `json:"text"`
	Site string `json:"site"`
}

// UpdateDTO is a newer release than the running build
type UpdateDTO struct {
	Latest string `json:"latest"`
	URL    string `json:"url"`
}

// FilterDTO is what a filter's selector offers: the values, whether the site
// also takes "all", and the value the loaded list shows
type FilterDTO struct {
	Values   []string `json:"values"`
	All      bool     `json:"all"`
	Selected string   `json:"selected"`
}

// RunDTO describes the run in progress, or the last one
type RunDTO struct {
	Chapters int  `json:"chapters"`
	Bundle   bool `json:"bundle"`
	Volumes  bool `json:"volumes"`
	// Rows is how many progress rows the run ends up with
	Rows int `json:"rows"`
}

// JobDTO is the progress of one chapter (or of the whole bundle)
type JobDTO struct {
	Title  string  `json:"title"`
	Number float64 `json:"number"`
	// Total and Done count pages: each page is downloaded and then archived,
	// so Total is twice the page count. A volume's row, whose pages aren't
	// known until it's packed, counts its progress out of a fixed Total.
	Total  int64  `json:"total"`
	Done   int64  `json:"done"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	// Status is the bundle's or volume's current phase; empty for single chapters
	Status string `json:"status"`
	// Volume is set on the row of a volume
	Volume bool `json:"volume"`
}

// SummaryDTO is the outcome of a finished run
type SummaryDTO struct {
	Dir       string   `json:"dir"`
	Cancelled bool     `json:"cancelled"`
	Saved     int      `json:"saved"`
	Volumes   int      `json:"volumes"`
	Bundled   bool     `json:"bundled"`
	Bundle    string   `json:"bundle"`
	Skipped   int      `json:"skipped"`
	Failures  []string `json:"failures"`
	// Error is set when a bundle couldn't be written
	Error string `json:"error"`
}

// ChapterDTO is one row of the chapter list. Index is the position in the
// list the frontend got, which is what Selection and SetSelected use.
type ChapterDTO struct {
	Index    int     `json:"index"`
	Number   float64 `json:"number"`
	Title    string  `json:"title"`
	Language string  `json:"language"`
	// Group is the scanlation group that made the release. It's only sent when
	// the list has copies of one number in a language (see State.HasGroupDuplicates)
	Group string `json:"group"`
	// Volume is the volume the site files the chapter under, 0 when it doesn't say
	Volume int `json:"volume"`
}

// LogDTO is one line of the log
type LogDTO struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// State returns the window's state
func (a *App) State() State {
	c := a.ctrl
	st := State{
		URL:                c.URL(),
		Loading:            c.Loading(),
		Loaded:             c.Loaded(),
		Title:              c.Title(),
		ChapterCount:       len(c.Chapters()),
		HasDuplicates:      c.HasDuplicates(),
		HasGroupDuplicates: c.HasGroupDuplicates(),
		Stale:              c.Stale(),
		SelectedCount:      c.SelectedCount(),
		Running:            c.Running(),
		Cancelling:         c.Cancelling(),
		Run:                runDTO(c.Run()),
		Jobs:               []JobDTO{},
		Summary:            summaryDTO(c.Summary()),
		Options:            optionsOf(c.Settings()),
		Version:            a.version,
		ReleaseURL:         releaseURL(a.tag),
		Update:             updateDTO(c.Update()),

		BrowserVisibleAuto: c.BrowserVisibleAuto(),
		BrowserAlert:       c.BrowserAlert(),
	}
	st.LoadedLanguage, st.LoadedScanlator = c.LoadedFilters()
	st.Volumes.Text, st.Volumes.Site = c.Volumes()
	language, scanlator := c.Filters()
	st.Languages, st.Scanlators = filterDTO(language), filterDTO(scanlator)
	if err := c.LoadError(); err != nil {
		st.LoadError = loadMessage(err)
	}
	for _, j := range c.Jobs() {
		st.Jobs = append(st.Jobs, JobDTO{
			Title:  j.Title,
			Number: j.Number,
			Total:  j.Total,
			Done:   j.Done,
			State:  jobStateName(j.State),
			Reason: j.Reason,
			Status: j.Status,
			Volume: j.Volume,
		})
	}

	return st
}

// Chapters returns the loaded chapter list, sorted by number
func (a *App) Chapters() []ChapterDTO {
	list := a.ctrl.Chapters()
	out := make([]ChapterDTO, 0, len(list))
	dupes := a.ctrl.HasDuplicates()
	groupDupes := a.ctrl.HasGroupDuplicates()
	for i, ch := range list {
		row := ChapterDTO{Index: i, Number: ch.GetNumber(), Title: ch.GetTitle(), Volume: grabber.VolumeOf(ch)}
		// the language only tells copies of the same number apart, so it's
		// only sent when the list has such copies (as the CLI shows it)
		if l, ok := ch.(interface{ GetLanguage() string }); ok && dupes {
			row.Language = l.GetLanguage()
		}
		if g, ok := ch.(interface{ GetGroup() string }); ok && groupDupes {
			row.Group = g.GetGroup()
		}
		out = append(out, row)
	}

	return out
}

// Selection returns the indices of the selected chapters, in list order
func (a *App) Selection() []int {
	return a.ctrl.SelectedIndices()
}

// Log returns the most recent log lines
func (a *App) Log() []LogDTO {
	lines := a.ctrl.Log()
	if len(lines) > logTail {
		lines = lines[len(lines)-logTail:]
	}
	out := make([]LogDTO, 0, len(lines))
	for _, l := range lines {
		out = append(out, LogDTO{Level: logLevelName(l.Level), Text: l.Text})
	}

	return out
}

// Clear forgets the loaded series (see Controller.Clear) and empties the
// chapter list the window holds.
func (a *App) Clear() error {
	if err := a.ctrl.Clear(); err != nil {
		return err
	}
	a.sink.Emit("chapters", a.Chapters())
	return nil
}

// Load identifies the site for url and fetches its title and chapters. It
// returns once loading has started; the outcome arrives in the state (loading,
// loadError) and the "chapters" event.
func (a *App) Load(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("enter the URL of a series first")
	}
	if err := a.ctrl.BeginLoad(url); err != nil {
		return err
	}
	a.finishLoad(url)

	return nil
}

// SetOptionsAndLoad applies the options and reloads url with them, as a filter
// picked from a selector does: SetOptions then Load, in one step, so the window
// never shows the picked filter as not applied. It's refused as SetOptions and
// Load are.
func (a *App) SetOptionsAndLoad(o OptionsDTO, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("enter the URL of a series first")
	}
	if err := a.ctrl.ConfigureLoad(func(s *grabber.Settings) { applyOptions(s, o) }, url); err != nil {
		return err
	}
	a.save()
	a.finishLoad(url)

	return nil
}

// finishLoad does the load BeginLoad or ConfigureLoad started, on its own
// goroutine, and reports its chapters when it's done
func (a *App) finishLoad(url string) {
	go func() {
		// a panic in a site's parser must not take the window down with it,
		// and must not leave the controls locked behind a load that never ends
		defer func() {
			if r := recover(); r != nil {
				a.ctrl.abortLoad(fmt.Errorf("the site's parser failed: %v", r))
			}
			a.sink.Emit("chapters", a.Chapters())
		}()
		// a filter the series doesn't have may have been reset to its fallback,
		// which the saved options should show too
		before := optionsOf(a.ctrl.Settings())
		// the error is kept by the controller and reported through the state
		_ = a.ctrl.FinishLoad(url)
		if optionsOf(a.ctrl.Settings()) != before {
			a.save()
		}
	}()
}

// SetSelected selects or deselects the chapter at index. It's refused while a
// series loads or a download runs.
func (a *App) SetSelected(index int, on bool) error {
	return a.ctrl.SetSelected(index, on)
}

// SetSelectedMany selects or deselects the chapters at indices in one step. An
// index out of range rejects the whole call.
func (a *App) SetSelectedMany(indices []int, on bool) error {
	return a.ctrl.SetSelectedMany(indices, on)
}

// SetAll selects or deselects every chapter. It's refused while a series
// loads or a download runs.
func (a *App) SetAll(on bool) error {
	return a.ctrl.SetAll(on)
}

// SelectRange selects the chapters whose number is in the range text
// ("1-10,12") and returns how many matched
func (a *App) SelectRange(text string) (int, error) {
	return a.ctrl.SelectRange(text)
}

// SetVolumes sets the Volumes box ("1-7/8-16", "" for the site's volumes).
// A value that doesn't parse is refused, as is any change while busy.
func (a *App) SetVolumes(text string) error {
	return a.ctrl.SetVolumes(text)
}

// Options returns the current options
func (a *App) Options() OptionsDTO {
	return optionsOf(a.ctrl.Settings())
}

// SetOptions applies the options and saves them for the next run. It's
// refused while a series loads or a download runs.
func (a *App) SetOptions(o OptionsDTO) error {
	if err := a.ctrl.Configure(func(s *grabber.Settings) { applyOptions(s, o) }); err != nil {
		return err
	}
	a.save()

	return nil
}

// save writes the current options to gui.json. A failure is logged rather than
// returned: the options still apply for this run.
func (a *App) save() {
	if a.prefsPath == "" {
		return
	}
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	if err := saveOptions(a.prefsPath, optionsOf(a.ctrl.Settings())); err != nil {
		a.ctrl.addLog(LogWarn, "could not save the options: "+err.Error())
	}
}

// ChooseFolder asks for the output folder with the native picker, starting at
// the current one. A picked folder becomes the output folder; "" means the
// picker was cancelled.
func (a *App) ChooseFolder() (string, error) {
	if a.ctrl.Loading() || a.ctrl.Running() {
		return "", errBusy
	}
	current := optionsOf(a.ctrl.Settings()).OutputDir
	dir, err := a.sink.ChooseDirectory(existingDir(current))
	if err != nil || dir == "" {
		return "", err
	}
	o := optionsOf(a.ctrl.Settings())
	o.OutputDir = dir
	if err := a.SetOptions(o); err != nil {
		return "", err
	}

	return dir, nil
}

// existingDir returns dir when it exists as a folder, the home folder
// otherwise, because the picker can refuse a starting folder that's gone
func existingDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			return abs
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return ""
}

// OpenFolder opens the folder of the last run's output, or the output folder
// when no run has finished, in the system's file manager
func (a *App) OpenFolder() error {
	dir := optionsOf(a.ctrl.Settings()).OutputDir
	if s := a.ctrl.Summary(); s != nil && s.Dir != "" {
		dir = s.Dir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return fmt.Errorf("the folder %s doesn't exist", abs)
	}

	return openInFileManager(abs)
}

// openInFileManager starts the platform's file manager on dir without waiting
// for it, since some of them (explorer) never exit with a clean status
func openInFileManager(dir string) error {
	name := "xdg-open"
	switch runtime.GOOS {
	case "windows":
		name = "explorer"
	case "darwin":
		name = "open"
	}
	cmd := exec.Command(name, dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	// waiting in the background reaps the child, so it doesn't linger as a zombie
	go func() { _ = cmd.Wait() }()

	return nil
}

// Start downloads the selected chapters in the background. Refusals (nothing
// selected, invalid options, a run already going) are returned here.
func (a *App) Start() error {
	return a.ctrl.Start()
}

// Cancel stops the run from starting new chapters
func (a *App) Cancel() {
	a.ctrl.Cancel()
}

// ShowBrowser brings the browser window to the front again
func (a *App) ShowBrowser() {
	a.ctrl.ShowBrowser()
}

// DismissBrowserAlert hides the alert about the browser window; the window
// and the load waiting for it go on
func (a *App) DismissBrowserAlert() {
	a.ctrl.DismissBrowserAlert()
}

// Version returns the release tag, or the build's version when it has none
func (a *App) Version() string {
	return a.version
}

// wailsRuntime is the sink on top of Wails' runtime. Its context is set in
// startup; before that (and after shutdown) it drops events instead of calling
// into a window that isn't there.
type wailsRuntime struct {
	mu  sync.Mutex
	ctx context.Context
}

func (r *wailsRuntime) set(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
}

// Close implements sink: the context is dropped, after which events are discarded
func (r *wailsRuntime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = nil
}

func (r *wailsRuntime) context() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx
}

// Emit implements sink
func (r *wailsRuntime) Emit(event string, data any) {
	if ctx := r.context(); ctx != nil {
		wruntime.EventsEmit(ctx, event, data)
	}
}

// ChooseDirectory implements sink
func (r *wailsRuntime) ChooseDirectory(defaultDir string) (string, error) {
	ctx := r.context()
	if ctx == nil {
		return "", errors.New("the window isn't ready yet")
	}

	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title:                "Choose the output folder",
		DefaultDirectory:     defaultDir,
		CanCreateDirectories: true,
	})
}

// loadMessage turns a Load error into the line shown under the URL
func loadMessage(err error) string {
	var titleErr *core.TitleError
	var chaptersErr *core.ChaptersError
	var noChapters *core.NoChaptersError
	switch {
	case errors.As(err, &titleErr):
		return "Couldn't fetch the series title: " + titleErr.Err.Error()
	case errors.As(err, &chaptersErr):
		return "Couldn't fetch the chapter list: " + chaptersErr.Error()
	case errors.As(err, &noChapters):
		return noChapters.Error()
	}

	return err.Error()
}

// updateDTO converts an available update, nil when there's none
func updateDTO(u *Update) *UpdateDTO {
	if u == nil {
		return nil
	}

	return &UpdateDTO{Latest: u.Latest, URL: latestReleasePage}
}

// filterDTO converts a filter's selector, nil when the site doesn't list it
func filterDTO(f *Filter) *FilterDTO {
	if f == nil {
		return nil
	}

	return &FilterDTO{Values: f.Values, All: f.All, Selected: f.Selected}
}

func runDTO(r RunInfo) RunDTO {
	return RunDTO{Chapters: r.Chapters, Bundle: r.Bundle, Volumes: r.Volumes, Rows: r.Rows}
}

func summaryDTO(s *Summary) *SummaryDTO {
	if s == nil {
		return nil
	}
	out := &SummaryDTO{
		Dir:       s.Dir,
		Cancelled: s.Cancelled,
		Saved:     s.Saved,
		Volumes:   s.Volumes,
		Bundled:   s.Bundled,
		Bundle:    s.Bundle,
		Skipped:   s.Skipped,
		Failures:  append([]string{}, s.Failures...),
	}
	if s.Err != nil {
		out.Error = s.Err.Error()
	}

	return out
}

// jobStateName is the frontend's name for a JobState
func jobStateName(s JobState) string {
	switch s {
	case JobDownloading:
		return "downloading"
	case JobArchiving:
		return "archiving"
	case JobDone:
		return "done"
	case JobFailed:
		return "failed"
	case JobSkipped:
		return "skipped"
	}

	return ""
}

// logLevelName is the frontend's name for a LogLevel
func logLevelName(l LogLevel) string {
	switch l {
	case LogWarn:
		return "warn"
	case LogError:
		return "error"
	}

	return "info"
}
