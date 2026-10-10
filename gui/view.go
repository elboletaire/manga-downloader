// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// refreshEvery throttles redraws while events stream in: a page event per
// image would otherwise queue a redraw each
const refreshEvery = 100 * time.Millisecond

// noSeries is the title shown until a series is loaded
const noSeries = "Paste a series URL above and press Load"

// view is the one window. It only reads the controller's state and calls its
// actions; it never holds state the controller doesn't.
type view struct {
	app   fyne.App
	win   fyne.Window
	ctrl  *Controller
	prefs fyne.Preferences

	content fyne.CanvasObject

	// syncing is set while the option widgets are filled from the settings,
	// so their change handlers don't write back
	syncing bool

	urlEntry *widget.Entry
	loadBtn  *widget.Button
	spinner  *widget.Activity
	loadErr  *widget.Label
	staleMsg *widget.Label

	title       *widget.Label
	count       *widget.Label
	selButtons  []fyne.Disableable
	rangeEntry  *widget.Entry
	rangeErr    *widget.Label
	chapterList *widget.List
	chapters    grabber.Filterables

	outputEntry   *widget.Entry
	browseBtn     *widget.Button
	formatSelect  *widget.Select
	convertSelect *widget.Select
	bundleCheck   *widget.Check
	languageEntry *widget.Entry
	scanEntry     *widget.Entry
	skipEntry     *widget.Entry
	templateEntry *widget.Entry
	browserCheck  *widget.Check
	optionWidgets []fyne.Disableable

	downloadBtn  *widget.Button
	cancelBtn    *widget.Button
	actionErr    *widget.Label
	jobList      *widget.List
	jobs         []Job
	progressHead *widget.Label
	summary      *widget.Label
	openBtn      *widget.Button
	options      *widget.Accordion
	logAcc       *widget.Accordion
	logLabel     *widget.Label
	logCount     int
}

// newView builds the window's content. changed, when non-nil, is what the
// controller signals on; the view then redraws from it, throttled, on the
// Fyne goroutine.
func newView(app fyne.App, win fyne.Window, ctrl *Controller, changed <-chan struct{}) *view {
	v := &view{app: app, win: win, ctrl: ctrl, prefs: app.Preferences()}
	v.build()
	v.syncOptions()
	v.refresh()
	if changed != nil {
		go v.watch(changed)
	}
	return v
}

// watch redraws whenever the controller signals a change. The channel holds
// one pending signal, so a burst collapses into a redraw that reads the state
// after the throttle.
func (v *view) watch(changed <-chan struct{}) {
	for range changed {
		time.Sleep(refreshEvery)
		fyne.Do(v.refresh)
	}
}

func (v *view) build() {
	// --- series: URL and load -------------------------------------------
	v.urlEntry = widget.NewEntry()
	v.urlEntry.SetPlaceHolder("Series URL, e.g. https://…")
	v.urlEntry.OnSubmitted = func(string) { v.load() }
	v.loadBtn = widget.NewButton("Load", v.load)
	v.loadBtn.Importance = widget.HighImportance
	v.spinner = widget.NewActivity()
	v.loadErr = widget.NewLabel("")
	v.loadErr.Importance = widget.DangerImportance
	v.loadErr.Wrapping = fyne.TextWrapWord
	v.staleMsg = widget.NewLabel("")
	v.staleMsg.Importance = widget.WarningImportance
	v.staleMsg.Wrapping = fyne.TextWrapWord
	urlRow := container.NewBorder(nil, nil, nil, container.NewHBox(v.spinner, v.loadBtn), v.urlEntry)

	// --- chapters -------------------------------------------------------
	v.title = widget.NewLabelWithStyle(noSeries, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.count = widget.NewLabel("")
	selAll := widget.NewButton("Select all", func() { v.ctrl.SetAll(true) })
	selNone := widget.NewButton("Select none", func() { v.ctrl.SetAll(false) })
	v.rangeEntry = widget.NewEntry()
	v.rangeEntry.SetPlaceHolder("Range, e.g. 1-10,12")
	v.rangeEntry.OnSubmitted = func(string) { v.selectRange() }
	selRange := widget.NewButton("Select range", v.selectRange)
	v.rangeErr = widget.NewLabel("")
	v.rangeErr.Importance = widget.DangerImportance
	v.selButtons = []fyne.Disableable{selAll, selNone, selRange, v.rangeEntry}

	v.chapterList = widget.NewList(
		func() int { return len(v.chapters) },
		func() fyne.CanvasObject { return widget.NewCheck("", nil) },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			check := o.(*widget.Check)
			i := int(id)
			check.OnChanged = nil
			check.Text = v.chapterLabel(v.chapters[i])
			check.SetChecked(v.ctrl.IsSelected(i))
			check.OnChanged = func(on bool) { v.ctrl.SetSelected(i, on) }
			check.Refresh()
		},
	)

	selectionBar := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(selAll, selNone), container.NewHBox(v.title, v.count)),
		container.NewBorder(nil, nil, nil, selRange, v.rangeEntry),
		v.rangeErr,
	)
	chapterPane := container.NewBorder(selectionBar, nil, nil, nil, v.chapterList)

	// --- options --------------------------------------------------------
	v.outputEntry = widget.NewEntry()
	v.outputEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.OutputDir = s }) }
	v.browseBtn = widget.NewButton("Browse…", v.browse)
	v.formatSelect = widget.NewSelect([]string{packer.FormatCBZ, packer.FormatRaw}, func(s string) {
		v.setOption(func(o *grabber.Settings) { o.Format = s })
	})
	v.convertSelect = widget.NewSelect([]string{"avif", "avif,webp", "none"}, func(s string) {
		v.setOption(func(o *grabber.Settings) { o.ConvertImages = s })
	})
	v.bundleCheck = widget.NewCheck("Bundle all chapters into one file", func(on bool) {
		v.setOption(func(o *grabber.Settings) { o.Bundle = on })
	})
	// language and scanlator filter the list itself, so they reload it on
	// Enter instead of on every keystroke
	v.languageEntry = widget.NewEntry()
	v.languageEntry.SetPlaceHolder("e.g. en, es-la (empty: all)")
	v.languageEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.Language = s }) }
	v.languageEntry.OnSubmitted = func(string) { v.reloadIfStale() }
	v.scanEntry = widget.NewEntry()
	v.scanEntry.SetPlaceHolder("group name, or all (empty: default)")
	v.scanEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.Scanlator = s }) }
	v.scanEntry.OnSubmitted = func(string) { v.reloadIfStale() }
	v.skipEntry = widget.NewEntry()
	v.skipEntry.SetPlaceHolder("e.g. 2 or 2,-1 (-1 is the last page)")
	v.skipEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.SkipPages = s }) }
	v.templateEntry = widget.NewEntry()
	v.templateEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.FilenameTemplate = s }) }
	v.browserCheck = widget.NewCheck("Show the browser window", func(on bool) {
		v.setOption(func(o *grabber.Settings) { o.BrowserVisible = on })
	})
	v.optionWidgets = []fyne.Disableable{v.outputEntry, v.browseBtn, v.formatSelect, v.convertSelect,
		v.bundleCheck, v.languageEntry, v.scanEntry, v.skipEntry, v.templateEntry, v.browserCheck}

	// the output folder is what people look for first, so it stays out of the
	// accordion
	outputRow := container.NewBorder(nil, nil, widget.NewLabel("Save to"), v.browseBtn, v.outputEntry)
	form := widget.NewForm(
		widget.NewFormItem("Format", v.formatSelect),
		widget.NewFormItem("Bundle", v.bundleCheck),
		widget.NewFormItem("Language", v.languageEntry),
		widget.NewFormItem("Scanlator", v.scanEntry),
		widget.NewFormItem("Convert images", v.convertSelect),
		widget.NewFormItem("Skip pages", v.skipEntry),
		widget.NewFormItem("Filename template", v.templateEntry),
		widget.NewFormItem("Browser", v.browserCheck),
	)
	v.options = widget.NewAccordion(widget.NewAccordionItem("Options", container.NewVBox(
		form,
		widget.NewLabel("Language and scanlator filter the chapter list: press Enter in them to reload it."),
	)))

	top := container.NewVBox(urlRow, v.loadErr, v.staleMsg, outputRow, v.options)

	// --- download -------------------------------------------------------
	v.downloadBtn = widget.NewButton("Download", v.start)
	v.downloadBtn.Importance = widget.HighImportance
	v.cancelBtn = widget.NewButton("Cancel", v.ctrl.Cancel)
	v.actionErr = widget.NewLabel("")
	v.actionErr.Importance = widget.DangerImportance
	v.actionErr.Wrapping = fyne.TextWrapWord

	v.jobList = widget.NewList(
		func() int { return len(v.jobs) },
		func() fyne.CanvasObject {
			return container.NewVBox(widget.NewLabel(""), widget.NewProgressBar())
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			j := v.jobs[id]
			row := o.(*fyne.Container)
			row.Objects[0].(*widget.Label).SetText(j.Title + " - " + jobStatus(j))
			bar := row.Objects[1].(*widget.ProgressBar)
			bar.Max = float64(j.Total)
			if j.Total == 0 {
				bar.Max = 1
			}
			bar.SetValue(float64(j.Done))
		},
	)
	v.summary = widget.NewLabel("")
	v.summary.Wrapping = fyne.TextWrapWord
	v.openBtn = widget.NewButton("Open folder", v.openFolder)

	actions := container.NewBorder(nil, nil, container.NewHBox(v.downloadBtn, v.cancelBtn), nil, v.actionErr)

	v.logLabel = widget.NewLabel("")
	v.logLabel.Wrapping = fyne.TextWrapWord
	logScroll := container.NewVScroll(v.logLabel)
	logScroll.SetMinSize(fyne.NewSize(0, 160))
	v.logAcc = widget.NewAccordion(widget.NewAccordionItem("Log", logScroll))

	v.progressHead = widget.NewLabelWithStyle("Progress", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	progressPane := container.NewBorder(v.progressHead, nil, nil, nil, v.jobList)
	split := container.NewVSplit(chapterPane, progressPane)
	split.Offset = 0.5

	bottom := container.NewVBox(actions, v.summary, container.NewHBox(v.openBtn), v.logAcc)

	v.content = container.NewBorder(top, bottom, nil, nil, split)
}

// chapterLabel is a chapter's list label, with its language when the list
// repeats numbers across languages (see core.HasDuplicateChapterNumbers)
func (v *view) chapterLabel(ch grabber.Filterable) string {
	title := ch.GetTitle()
	if !v.ctrl.HasDuplicates() {
		return title
	}
	if l, ok := ch.(interface{ GetLanguage() string }); ok && l.GetLanguage() != "" {
		return fmt.Sprintf("%s [%s]", title, l.GetLanguage())
	}
	return title
}

// jobStatus is the line under a progress row
func jobStatus(j Job) string {
	switch j.State {
	case JobDownloading:
		if j.Status != "" {
			return j.Status
		}
		return "downloading"
	case JobArchiving:
		if j.Status != "" {
			return j.Status
		}
		return "archiving"
	case JobDone:
		return "done"
	case JobFailed:
		return "failed: " + j.Reason
	case JobSkipped:
		return "skipped: " + j.Reason
	}
	return ""
}

// refresh redraws everything from the controller. It runs on the Fyne
// goroutine only.
func (v *view) refresh() {
	loading := v.ctrl.Loading()
	running := v.ctrl.Running()
	loaded := v.ctrl.Loaded()
	busy := loading || running

	if loading {
		v.spinner.Show()
		v.spinner.Start()
	} else {
		v.spinner.Stop()
		v.spinner.Hide()
	}
	setEnabled(!busy, v.urlEntry, v.loadBtn)
	setEnabled(!busy, v.optionWidgets...)
	setEnabled(loaded && !busy, v.selButtons...)
	setEnabled(loaded && !busy, v.downloadBtn)
	setEnabled(running, v.cancelBtn)

	if loaded {
		v.title.SetText(v.ctrl.Title())
	} else {
		v.title.SetText(noSeries)
	}
	total := 0
	if loaded {
		v.chapters = v.ctrl.Chapters()
		total = len(v.chapters)
		v.chapterList.Refresh()
	} else {
		v.chapters = nil
	}
	selected := v.ctrl.SelectedCount()
	v.count.SetText(fmt.Sprintf("%d of %d selected", selected, total))
	switch selected {
	case 0:
		v.downloadBtn.SetText("Download")
	case 1:
		v.downloadBtn.SetText("Download 1 chapter")
	default:
		v.downloadBtn.SetText(fmt.Sprintf("Download %d chapters", selected))
	}

	v.jobs = v.ctrl.Jobs()
	v.jobList.Refresh()
	v.refreshProgress()

	if v.ctrl.Stale() {
		v.staleMsg.SetText("The language or scanlator changed: press Enter in that field to reload the list.")
	} else {
		v.staleMsg.SetText("")
	}
	// empty labels still take a line, so the ones with nothing to say are hidden
	setShown(v.loadErr.Text != "", v.loadErr)
	setShown(v.staleMsg.Text != "", v.staleMsg)
	setShown(v.rangeErr.Text != "", v.rangeErr)
	setShown(v.actionErr.Text != "", v.actionErr)

	v.refreshSummary()
	v.refreshLog()
}

// refreshProgress says how far the run has got, and how many chapters it has
func (v *view) refreshProgress() {
	run := v.ctrl.Run()
	if run.Bundle {
		v.progressHead.SetText(fmt.Sprintf("Bundle of %s", plural(run.Chapters, "chapter", "chapters")))
		return
	}
	finished := 0
	for _, j := range v.jobs {
		if j.State == JobDone || j.State == JobFailed || j.State == JobSkipped {
			finished++
		}
	}
	if run.Chapters == 0 {
		v.progressHead.SetText("Progress")
		return
	}
	v.progressHead.SetText(fmt.Sprintf("Progress: %d of %d chapters finished", finished, run.Chapters))
}

func (v *view) refreshSummary() {
	s := v.ctrl.Summary()
	if s == nil {
		v.summary.SetText("")
		v.openBtn.Disable()
		return
	}
	v.summary.SetText(summaryText(s))
	v.openBtn.Enable()
}

func (v *view) refreshLog() {
	lines := v.ctrl.Log()
	if len(lines) == v.logCount {
		return
	}
	v.logCount = len(lines)
	var b strings.Builder
	for _, l := range lines {
		switch l.Level {
		case LogWarn:
			b.WriteString("warning: ")
		case LogError:
			b.WriteString("error: ")
		}
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	v.logLabel.SetText(strings.TrimSuffix(b.String(), "\n"))
}

// summaryText says what a finished run produced, in plain words
func summaryText(s *Summary) string {
	var parts []string
	switch {
	case s.Cancelled && s.Bundled:
		parts = append(parts, "Cancelled. No bundle was written.")
	case s.Cancelled && s.Saved == 0:
		parts = append(parts, "Cancelled before anything was saved.")
	case s.Cancelled:
		parts = append(parts, fmt.Sprintf("Cancelled. %s saved before it stopped, in %s.", plural(s.Saved, "chapter", "chapters"), s.Dir))
	case s.Err != nil:
		parts = append(parts, "The bundle could not be written.")
	case s.Bundled && s.Bundle != "":
		parts = append(parts, fmt.Sprintf("Saved %s to %s (%s).", s.Bundle, s.Dir, plural(s.Saved, "chapter", "chapters")))
	case s.Saved > 0:
		parts = append(parts, fmt.Sprintf("Saved %s to %s.", plural(s.Saved, "chapter", "chapters"), s.Dir))
	default:
		parts = append(parts, "Nothing was saved.")
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%s skipped: --skip-pages excluded every page.", plural(s.Skipped, "chapter", "chapters")))
	}
	if len(s.Failures) > 0 {
		parts = append(parts, fmt.Sprintf("%d failed:", len(s.Failures)))
		for _, f := range s.Failures {
			parts = append(parts, "  - "+f)
		}
	}
	return strings.Join(parts, "\n")
}

// load runs Load for the URL in the entry, off the Fyne goroutine
func (v *view) load() {
	u := strings.TrimSpace(v.urlEntry.Text)
	if u == "" {
		v.loadErr.SetText("Enter the URL of a series first.")
		return
	}
	v.loadErr.SetText("")
	v.loadURL(u)
}

func (v *view) loadURL(u string) {
	go func() {
		err := v.ctrl.Load(u)
		fyne.Do(func() {
			v.loadErr.SetText(loadMessage(err))
			v.refresh()
		})
	}()
	v.refresh()
}

// loadMessage turns a Load error into the line shown under the URL
func loadMessage(err error) string {
	if err == nil {
		return ""
	}
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

func (v *view) reloadIfStale() {
	if v.ctrl.Loaded() && v.ctrl.Stale() && !v.ctrl.Loading() && !v.ctrl.Running() {
		v.loadURL(v.ctrl.URL())
	}
}

func (v *view) selectRange() {
	v.rangeErr.SetText("")
	n, err := v.ctrl.SelectRange(v.rangeEntry.Text)
	if err != nil {
		v.rangeErr.SetText("Invalid range: " + err.Error())
		return
	}
	if n == 0 {
		v.rangeErr.SetText("No chapter matches that range.")
	}
}

// start validates through the controller and starts the download. A refusal
// is shown next to the buttons.
func (v *view) start() {
	v.actionErr.SetText("")
	if err := v.ctrl.Start(); err != nil {
		v.actionErr.SetText(err.Error())
	}
	v.refresh()
}

// setOption applies one option change and remembers it for next time
func (v *view) setOption(fn func(*grabber.Settings)) {
	if v.syncing {
		return
	}
	if err := v.ctrl.Configure(fn); err != nil {
		return
	}
	saveOptions(v.prefs, v.ctrl.Settings())
	v.refresh()
}

// syncOptions fills the option widgets from the settings
func (v *view) syncOptions() {
	s := v.ctrl.Settings()
	v.syncing = true
	defer func() { v.syncing = false }()
	v.outputEntry.SetText(s.OutputDir)
	v.formatSelect.SetSelected(s.Format)
	v.convertSelect.SetSelected(s.ConvertImages)
	v.bundleCheck.SetChecked(s.Bundle)
	v.languageEntry.SetText(s.Language)
	v.scanEntry.SetText(s.Scanlator)
	v.skipEntry.SetText(s.SkipPages)
	v.templateEntry.SetText(s.FilenameTemplate)
	v.browserCheck.SetChecked(s.BrowserVisible)
}

// browse picks the output folder, starting from the current one
func (v *view) browse() {
	d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if u == nil {
			return
		}
		v.outputEntry.SetText(u.Path())
	}, v.win)
	if lister, err := storage.ListerForURI(storage.NewFileURI(v.ctrl.Settings().OutputDir)); err == nil {
		d.SetLocation(lister)
	}
	d.Show()
}

// openFolder opens the folder of the last summary in the file manager
func (v *view) openFolder() {
	s := v.ctrl.Summary()
	if s == nil {
		return
	}
	u, err := url.Parse(storage.NewFileURI(s.Dir).String())
	if err != nil {
		dialog.ShowError(err, v.win)
		return
	}
	if err := v.app.OpenURL(u); err != nil {
		dialog.ShowError(err, v.win)
	}
}

// setShown shows or hides w
func setShown(on bool, w fyne.CanvasObject) {
	if on {
		w.Show()
	} else {
		w.Hide()
	}
}

// setEnabled enables or disables every widget in ws
func setEnabled(on bool, ws ...fyne.Disableable) {
	for _, w := range ws {
		if on {
			w.Enable()
		} else {
			w.Disable()
		}
	}
}
