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
	version   string
	interval  time.Duration

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
func newApp(ctrl *Controller, s sink, prefsPath, version string) *App {
	return &App{
		ctrl:      ctrl,
		sink:      s,
		prefsPath: prefsPath,
		version:   version,
		interval:  refreshInterval,
		dirty:     make(chan struct{}, 1),
		stop:      make(chan struct{}),
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
}

// shutdown stops the pushes before the window goes away, then cancels any run
// and closes the browser, as the controller's Shutdown does
func (a *App) shutdown(context.Context) {
	a.stopOnce.Do(func() { close(a.stop) })
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
	Stale         bool   `json:"stale"`
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
	Version         string      `json:"version"`
}

// RunDTO describes the run in progress, or the last one
type RunDTO struct {
	Chapters int  `json:"chapters"`
	Bundle   bool `json:"bundle"`
}

// JobDTO is the progress of one chapter (or of the whole bundle)
type JobDTO struct {
	Title  string  `json:"title"`
	Number float64 `json:"number"`
	// Total and Done count pages: each page is downloaded and then archived,
	// so Total is twice the page count
	Total  int64  `json:"total"`
	Done   int64  `json:"done"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	// Status is the bundle's current phase; empty for single chapters
	Status string `json:"status"`
}

// SummaryDTO is the outcome of a finished run
type SummaryDTO struct {
	Dir       string   `json:"dir"`
	Cancelled bool     `json:"cancelled"`
	Saved     int      `json:"saved"`
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
		URL:           c.URL(),
		Loading:       c.Loading(),
		Loaded:        c.Loaded(),
		Title:         c.Title(),
		ChapterCount:  len(c.Chapters()),
		HasDuplicates: c.HasDuplicates(),
		Stale:         c.Stale(),
		SelectedCount: c.SelectedCount(),
		Running:       c.Running(),
		Cancelling:    c.Cancelling(),
		Run:           runDTO(c.Run()),
		Jobs:          []JobDTO{},
		Summary:       summaryDTO(c.Summary()),
		Options:       optionsOf(c.Settings()),
		Version:       a.version,
	}
	st.LoadedLanguage, st.LoadedScanlator = c.LoadedFilters()
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
		})
	}

	return st
}

// Chapters returns the loaded chapter list, sorted by number
func (a *App) Chapters() []ChapterDTO {
	list := a.ctrl.Chapters()
	out := make([]ChapterDTO, 0, len(list))
	dupes := a.ctrl.HasDuplicates()
	for i, ch := range list {
		row := ChapterDTO{Index: i, Number: ch.GetNumber(), Title: ch.GetTitle()}
		// the language only tells copies of the same number apart, so it's
		// only sent when the list has such copies (as the CLI shows it)
		if l, ok := ch.(interface{ GetLanguage() string }); ok && dupes {
			row.Language = l.GetLanguage()
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
	go func() {
		// a panic in a site's parser must not take the window down with it,
		// and must not leave the controls locked behind a load that never ends
		defer func() {
			if r := recover(); r != nil {
				a.ctrl.abortLoad(fmt.Errorf("the site's parser failed: %v", r))
			}
			a.sink.Emit("chapters", a.Chapters())
		}()
		// the error is kept by the controller and reported through the state
		_ = a.ctrl.FinishLoad(url)
	}()

	return nil
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

func runDTO(r RunInfo) RunDTO {
	return RunDTO{Chapters: r.Chapters, Bundle: r.Bundle}
}

func summaryDTO(s *Summary) *SummaryDTO {
	if s == nil {
		return nil
	}
	out := &SummaryDTO{
		Dir:       s.Dir,
		Cancelled: s.Cancelled,
		Saved:     s.Saved,
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
