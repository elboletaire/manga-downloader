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
	"sync/atomic"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
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
	// ChooseDirectory shows the native folder picker titled title, "" when cancelled
	ChooseDirectory(title, defaultDir string) (string, error)
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
	// releaseAPI is where the update check asks for the latest release; a test seam
	releaseAPI string
	// updateCtx bounds the update check; cancelUpdate ends it at shutdown
	updateCtx    context.Context
	cancelUpdate context.CancelFunc
	interval     time.Duration
	// installed is whether this exe is an installed copy (the Windows
	// installer's, the Linux install.sh's), which can update itself; the
	// other fields of the update are test seams
	installed bool
	updating  atomic.Bool
	fetch     func(ctx context.Context, pkg, sums *Asset) (string, error)
	install   func(string) error
	quit      func()

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
		releaseAPI:   latestReleaseAPI,
		updateCtx:    updateCtx,
		cancelUpdate: cancelUpdate,
		installed:    installedCopy(),
		fetch: func(ctx context.Context, pkg, sums *Asset) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, updateTimeout)
			defer cancel()

			return downloadInstaller(ctx, newDownloadClient(), pkg, sums)
		},
		install:  runInstaller,
		interval: refreshInterval,
		dirty:    make(chan struct{}, 1),
		stop:     make(chan struct{}),
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
	a.quit = func() { wruntime.Quit(ctx) }
	go sweepUpdateDirs(os.TempDir())
	a.launch()
	// the check runs behind the window, which is up already
	a.checkUpdates()
}

// checkUpdates starts the update check in the background, unless one is running, when
// shouldCheckUpdates allows it
func (a *App) checkUpdates() {
	if !shouldCheckUpdates(a.tag, a.ctrl.Prefs()) || !a.ctrl.claimUpdateCheck() {
		return
	}
	go a.ctrl.CheckUpdate(a.updateCtx, a.tag, a.releaseAPI, updateClient)
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
	URL           string   `json:"url"`
	Loading       bool     `json:"loading"`
	Loaded        bool     `json:"loaded"`
	LoadError     string   `json:"loadError"`
	LoadErrorMsg  *Message `json:"loadErrorMsg,omitempty"`
	Title         string   `json:"title"`
	ChapterCount  int      `json:"chapterCount"`
	HasDuplicates bool     `json:"hasDuplicates"`
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
	// Prefs are the app-only preferences, which aren't options (see PrefsDTO)
	Prefs PrefsDTO `json:"prefs"`
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
	// BrowserAlertMsg is BrowserAlert as a message, when there's an alert
	BrowserAlertMsg *Message `json:"browserAlertMsg,omitempty"`
	// FilenameTemplateDefault is the template a blank option stands for, and
	// FilenameTemplateWarning what is wrong with the current one, "" when nothing
	FilenameTemplateDefault string `json:"filenameTemplateDefault"`
	FilenameTemplateWarning string `json:"filenameTemplateWarning"`
}

// VolumesDTO is the Volumes box ("" for the site's own volumes), the site's
// volumes as ranges ("" when they can't be written so) and how many it lists
type VolumesDTO struct {
	Text   string `json:"text"`
	Site   string `json:"site"`
	Listed int    `json:"listed"`
}

// UpdateDTO is a newer release than the running build
type UpdateDTO struct {
	Latest string `json:"latest"`
	URL    string `json:"url"`
	// OneClick is whether Update can install it: an installed copy and its
	// package and checksums on the release
	OneClick bool `json:"oneClick"`
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
	Title string `json:"title"`
	// TitleMsg, ReasonMsg and StatusMsg are the texts the GUI wrote, as
	// messages for the window; Title, Reason and Status stay in English
	TitleMsg *Message `json:"titleMsg,omitempty"`
	// Tags is the release's language and group, shown apart from the title
	Tags   string  `json:"tags"`
	Number float64 `json:"number"`
	// Total and Done count pages: each page is downloaded and then archived,
	// so Total is twice the page count. A volume's row, whose pages aren't
	// known until it's packed, counts its progress out of a fixed Total.
	Total     int64    `json:"total"`
	Done      int64    `json:"done"`
	State     string   `json:"state"`
	Reason    string   `json:"reason"`
	ReasonMsg *Message `json:"reasonMsg,omitempty"`
	// Status is the bundle's or volume's current phase; empty for single chapters
	Status    string   `json:"status"`
	StatusMsg *Message `json:"statusMsg,omitempty"`
	// Volume is set on the row of a volume
	Volume bool `json:"volume"`
}

// FailureDTO is a line of the run summary's failures
type FailureDTO struct {
	Label     string   `json:"label"`
	Reason    string   `json:"reason"`
	ReasonMsg *Message `json:"reasonMsg,omitempty"`
}

// SummaryDTO is the outcome of a finished run
type SummaryDTO struct {
	Dir       string `json:"dir"`
	Cancelled bool   `json:"cancelled"`
	Saved     int    `json:"saved"`
	Volumes   int    `json:"volumes"`
	// Parts is set when the volume files are chapters with their parts merged
	Parts    bool         `json:"parts"`
	Bundled  bool         `json:"bundled"`
	Bundle   string       `json:"bundle"`
	Skipped  int          `json:"skipped"`
	Failures []FailureDTO `json:"failures"`
	// Error is set when a bundle couldn't be written
	Error string `json:"error"`
	// ErrorMsg is Error as a message, when the window translates it
	ErrorMsg *Message `json:"errorMsg,omitempty"`
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
	// Locked is set when the listing marks the chapter as locked: it can't be
	// selected (see Controller.SetSelected) nor downloaded
	Locked bool `json:"locked"`
	// Part is the chapter's part as its label writes it ("Part 2"), and
	// WholeTitle its title without it; both empty for a chapter without parts
	Part       string `json:"part,omitempty"`
	WholeTitle string `json:"wholeTitle,omitempty"`
	// Unnumbered is set for a chapter the site lists without a number: it
	// is grouped with no other chapter in a parts run
	Unnumbered bool `json:"unnumbered,omitempty"`
}

// LogDTO is one line of the log. Msg is set when the window translates the
// line; Text stays the English text (for copying the log).
type LogDTO struct {
	Level string   `json:"level"`
	Text  string   `json:"text"`
	Msg   *Message `json:"msg,omitempty"`
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
		Prefs:              c.Prefs(),
		Version:            a.version,
		ReleaseURL:         releaseURL(a.tag),
		Update:             updateDTO(c.Update(), a.installed),

		BrowserVisibleAuto: c.BrowserVisibleAuto(),

		FilenameTemplateDefault: packer.FilenameTemplateDefault,
	}
	// a bundle's name is always distinct, and a volume template needs .Volume;
	// a parts run names its files as chapters
	if st.Options.PackAs == PackChapters || st.Options.PackAs == PackParts {
		st.FilenameTemplateWarning = packer.TemplateWarning(st.Options.FilenameTemplate)
	}
	st.LoadedLanguage, st.LoadedScanlator = c.LoadedFilters()
	st.Volumes.Text, st.Volumes.Site, st.Volumes.Listed = c.Volumes()
	language, scanlator := c.Filters()
	st.Languages, st.Scanlators = filterDTO(language), filterDTO(scanlator)
	if err := c.LoadError(); err != nil {
		st.LoadError, st.LoadErrorMsg = loadMessage(err)
	}
	if alert := c.BrowserAlert(); alert != nil {
		st.BrowserAlert, st.BrowserAlertMsg = alert.Text, alert
	}
	for _, j := range c.Jobs() {
		st.Jobs = append(st.Jobs, JobDTO{
			Title:     j.Title,
			TitleMsg:  j.TitleMsg,
			Tags:      j.Tags,
			Number:    j.Number,
			Total:     j.Total,
			Done:      j.Done,
			State:     jobStateName(j.State),
			Reason:    j.Reason,
			ReasonMsg: j.ReasonMsg,
			Status:    j.Status,
			StatusMsg: j.StatusMsg,
			Volume:    j.Volume,
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
		row := ChapterDTO{Index: i, Number: ch.GetNumber(), Title: grabber.ChapterLabel(ch), Volume: grabber.VolumeOf(ch), Locked: grabber.LockedOf(ch)}
		row.Part, row.WholeTitle = grabber.PartOf(ch)
		row.Unnumbered = grabber.Unnumbered(ch)
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
		out = append(out, LogDTO{Level: logLevelName(l.Level), Text: l.Text, Msg: l.Msg})
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
		return uiErr("errors.enterURL", nil, nil)
	}
	if err := a.ctrl.BeginLoad(url); err != nil {
		return err
	}
	a.finishLoad(url)

	return nil
}

// SetOptionsAndLoad applies the options and reloads url in one step, so a filter picked
// from a selector is never shown as not applied. It's refused as SetOptions and Load are.
func (a *App) SetOptionsAndLoad(o OptionsDTO, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return uiErr("errors.enterURL", nil, nil)
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
				a.ctrl.abortLoad(uiErr("errors.parserFailed", map[string]any{"reason": fmt.Sprint(r)}, nil))
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

// SetUILanguage sets the window's language: "" follows the system, else en, es
// or ca. It's never refused while busy, and is saved for the next run.
func (a *App) SetUILanguage(lang string) error {
	if !validUILanguage(lang) {
		return uiErr("errors.unknownLanguage", map[string]any{"language": lang}, nil)
	}
	a.ctrl.SetPrefs(func(p *PrefsDTO) { p.UILanguage = lang })
	a.save()

	return nil
}

// SetSimpleStart sets whether the window shows the simple view while nothing is
// loaded. It's never refused while busy; a load started keeps the full view.
func (a *App) SetSimpleStart(on bool) error {
	a.ctrl.SetPrefs(func(p *PrefsDTO) { p.SimpleStart = on })
	a.save()

	return nil
}

// SetCheckUpdates turns the update check at startup on or off. Turning it off
// forgets the update found; turning it on checks now, unless a check is still
// running. It's never refused while busy.
func (a *App) SetCheckUpdates(on bool) error {
	a.ctrl.SetPrefs(func(p *PrefsDTO) { p.CheckUpdates = on })
	if on {
		a.checkUpdates()
	} else {
		a.ctrl.ClearUpdate()
	}
	a.save()

	return nil
}

// save writes the current options and preferences to gui.json. Both go in every
// write, so one never erases the other. A failure is logged rather than
// returned: the options still apply for this run.
func (a *App) save() {
	if a.prefsPath == "" {
		return
	}
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	if err := saveOptions(a.prefsPath, optionsOf(a.ctrl.Settings()), a.ctrl.Prefs()); err != nil {
		a.ctrl.addMsg(LogWarn, msg("log.saveFailed", map[string]any{"reason": err.Error()}))
	}
}

// ChooseFolder asks for the output folder with the native picker, titled title
// and starting at the current one. A picked folder becomes the output folder; "" means the
// picker was cancelled.
func (a *App) ChooseFolder(title string) (string, error) {
	if a.ctrl.Loading() || a.ctrl.Running() {
		return "", errBusy
	}
	current := optionsOf(a.ctrl.Settings()).OutputDir
	dir, err := a.sink.ChooseDirectory(title, existingDir(current))
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
		return uiErr("errors.folderMissing", map[string]any{"path": abs}, nil)
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

// Update downloads the newer release's package, checks it against the release's
// checksums, runs its installer (which relaunches the app) and quits. Any
// failure is logged and returned; the release link stays the fallback.
func (a *App) Update() error {
	u := a.ctrl.Update()
	if !a.installed || u == nil || u.Installer == nil || u.Sums == nil {
		return uiErr("errors.updateUnavailable", nil, nil)
	}
	if !a.updating.CompareAndSwap(false, true) {
		return uiErr("errors.updateBusy", nil, nil)
	}
	defer a.updating.Store(false)
	if a.ctrl.Running() {
		return a.failUpdate(uiErr("errors.updateWaitDownload", nil, nil))
	}

	a.ctrl.addMsg(LogInfo, msg("log.downloadingUpdate", map[string]any{"version": u.Latest}))
	path, err := a.fetch(a.updateCtx, u.Installer, u.Sums)
	if err != nil {
		return a.failUpdate(uiErr("errors.updateDownload", map[string]any{"reason": err.Error()}, err))
	}
	if a.ctrl.Running() {
		_ = removeUpdateDir(filepath.Dir(path))
		return a.failUpdate(uiErr("errors.updateMeanwhile", nil, nil))
	}
	if err := a.install(path); errors.Is(err, errNotRestarted) {
		// installed already, so the files stay; only the restart is left to the user
		err = a.failUpdate(uiErr("errors.updateRestart", map[string]any{"version": u.Latest}, nil))
		if a.quit != nil {
			a.quit()
		}

		return err
	} else if err != nil {
		_ = removeUpdateDir(filepath.Dir(path))
		return a.failUpdate(uiErr("errors.updateInstaller", map[string]any{"reason": err.Error()}, err))
	}
	if a.quit != nil {
		a.quit()
	}

	return nil
}

// failUpdate logs an update error in the window's language and returns it
func (a *App) failUpdate(err error) error {
	var ue *uiError
	if errors.As(err, &ue) {
		a.ctrl.addMsg(LogError, ue.Message)
	} else {
		a.ctrl.addLog(LogError, err.Error())
	}

	return err
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
func (r *wailsRuntime) ChooseDirectory(title, defaultDir string) (string, error) {
	ctx := r.context()
	if ctx == nil {
		return "", uiErr("errors.notReady", nil, nil)
	}

	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title:                title,
		DefaultDirectory:     defaultDir,
		CanCreateDirectories: true,
	})
}

// loadMessage turns a Load error into the line shown under the URL: its text,
// and its message when the GUI wrote the error
func loadMessage(err error) (string, *Message) {
	var ue *uiError
	var titleErr *core.TitleError
	var chaptersErr *core.ChaptersError
	var noChapters *core.NoChaptersError
	switch {
	case errors.As(err, &ue):
		m := ue.Message
		return m.Text, &m
	case errors.As(err, &titleErr):
		m := msg("errors.loadTitle", map[string]any{"reason": titleErr.Err.Error()})
		return m.Text, &m
	case errors.As(err, &chaptersErr):
		m := msg("errors.loadChapters", map[string]any{"reason": chaptersErr.Error()})
		return m.Text, &m
	case errors.As(err, &noChapters):
		return noChapters.Error(), nil
	}

	return err.Error(), nil
}

// updateDTO converts an available update, nil when there's none
func updateDTO(u *Update, installed bool) *UpdateDTO {
	if u == nil {
		return nil
	}

	return &UpdateDTO{Latest: u.Latest, URL: latestReleasePage, OneClick: installed && u.Installer != nil && u.Sums != nil}
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
		Parts:     s.Parts,
		Bundled:   s.Bundled,
		Bundle:    s.Bundle,
		Skipped:   s.Skipped,
		Failures:  []FailureDTO{},
	}
	for _, f := range s.Failures {
		out.Failures = append(out.Failures, FailureDTO{Label: f.Label, Reason: f.Reason, ReasonMsg: f.ReasonMsg})
	}
	if s.Err != nil {
		out.Error = s.Err.Error()
		out.ErrorMsg = messageOf(s.Err)
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
	case JobFetching:
		return "fetching"
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
