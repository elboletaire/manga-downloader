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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// refreshEvery throttles redraws while events stream in: a page event per
// image would otherwise queue a redraw each
const refreshEvery = 100 * time.Millisecond

// noSeries is the chapter pane's title until a series is loaded
const noSeries = "No series loaded"

// Hints shown where a list would be, while it has nothing to show
const (
	chaptersHint = "Paste a series URL above and press Load to list its chapters."
	progressHint = "Nothing downloaded yet. Select chapters and press Download."
	rangeHint    = "A range like 1-10,12 adds those chapters to the selection."
)

// view is the one window. It only reads the controller's state and calls its
// actions; it never holds state the controller doesn't.
//
// The layout is a fixed header (URL, output folder), a split body (chapters
// on the left, options/progress/log tabs on the right) and a fixed action
// bar, so nothing a run changes can push the buttons around.
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
	// notice is the line under the header: a load error, or the warning that
	// the list no longer matches its filters
	notice *widget.Label
	// loadErr is the last load error, which notice shows ahead of anything else
	loadErr string

	title        *widget.Label
	count        *widget.Label
	selButtons   []fyne.Disableable
	rangeEntry   *widget.Entry
	rangeMsg     *widget.Label
	chapterList  *widget.List
	chaptersNone fyne.CanvasObject
	chapters     grabber.Filterables

	outputEntry   *widget.Entry
	browseBtn     *widget.Button
	formatSelect  *widget.Select
	convertSelect *widget.Select
	bundleCheck   *widget.Check
	languageEntry *widget.Entry
	scanEntry     *widget.Entry
	applyBtn      *widget.Button
	skipEntry     *widget.Entry
	templateEntry *widget.Entry
	browserCheck  *widget.Check
	optionWidgets []fyne.Disableable

	tabs        *container.AppTabs
	optionsTab  *container.TabItem
	progressTab *container.TabItem
	logTab      *container.TabItem

	downloadBtn *widget.Button
	cancelBtn   *widget.Button
	openBtn     *widget.Button
	// status is the one line next to the buttons: a Start refusal, the run's
	// progress, or the last run's headline. It truncates rather than wraps, so
	// it never changes the bar's height.
	status *widget.Label
	// actionErr is the last Start refusal, cleared by the next attempt
	actionErr string

	progressHead *widget.Label
	jobList      *widget.List
	jobsNone     fyne.CanvasObject
	jobs         []Job
	details      *widget.Label
	detailsBox   fyne.CanvasObject

	logLabel  *widget.Label
	logScroll *container.Scroll
	logCount  int
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
	header := v.buildHeader()
	chapters := v.buildChapters()

	v.optionsTab = container.NewTabItem("Options", v.buildOptions())
	v.progressTab = container.NewTabItem("Progress", v.buildProgress())
	v.logTab = container.NewTabItem("Log", v.buildLog())
	v.tabs = container.NewAppTabs(v.optionsTab, v.progressTab, v.logTab)

	body := container.NewHSplit(chapters, v.tabs)
	body.Offset = 0.5

	v.content = container.NewBorder(header, v.buildActions(), nil, nil, body)
}

func (v *view) buildHeader() fyne.CanvasObject {
	v.urlEntry = widget.NewEntry()
	v.urlEntry.SetPlaceHolder("Series URL, e.g. https://…")
	v.urlEntry.OnSubmitted = func(string) { v.load() }
	v.loadBtn = widget.NewButton("Load", v.load)
	v.loadBtn.Importance = widget.HighImportance
	v.spinner = widget.NewActivity()
	urlRow := container.NewBorder(nil, nil, nil, container.NewHBox(v.spinner, v.loadBtn), v.urlEntry)

	// the output folder is what people look for first, so it's always in view
	v.outputEntry = widget.NewEntry()
	v.outputEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.OutputDir = s }) }
	v.browseBtn = widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), v.browse)
	outputRow := container.NewBorder(nil, nil, widget.NewLabel("Save to"), v.browseBtn, v.outputEntry)

	v.notice = widget.NewLabel("")
	v.notice.Wrapping = fyne.TextWrapWord

	return container.NewVBox(urlRow, outputRow, v.notice)
}

func (v *view) buildChapters() fyne.CanvasObject {
	v.title = widget.NewLabelWithStyle(noSeries, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.title.Truncation = fyne.TextTruncateEllipsis
	v.count = widget.NewLabel("")

	selAll := widget.NewButton("Select all", func() { v.ctrl.SetAll(true) })
	selNone := widget.NewButton("Select none", func() { v.ctrl.SetAll(false) })
	v.rangeEntry = widget.NewEntry()
	v.rangeEntry.SetPlaceHolder("e.g. 1-10,12")
	v.rangeEntry.OnSubmitted = func(string) { v.selectRange() }
	addRange := widget.NewButton("Add range", v.selectRange)
	v.selButtons = []fyne.Disableable{selAll, selNone, addRange, v.rangeEntry}
	// the hint and a bad range share one line, so an error doesn't move the list
	v.rangeMsg = widget.NewLabel(rangeHint)
	v.rangeMsg.Truncation = fyne.TextTruncateEllipsis
	v.rangeMsg.SizeName = theme.SizeNameCaptionText

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
	v.chaptersNone = emptyHint(chaptersHint)

	head := container.NewVBox(
		container.NewBorder(nil, nil, nil, v.count, v.title),
		container.NewBorder(nil, nil, container.NewHBox(selAll, selNone), addRange, v.rangeEntry),
		v.rangeMsg,
		widget.NewSeparator(),
	)
	return container.NewBorder(head, nil, nil, nil, container.NewStack(v.chapterList, v.chaptersNone))
}

func (v *view) buildOptions() fyne.CanvasObject {
	v.formatSelect = widget.NewSelect([]string{packer.FormatCBZ, packer.FormatRaw}, func(s string) {
		v.setOption(func(o *grabber.Settings) { o.Format = s })
	})
	v.convertSelect = widget.NewSelect([]string{"avif", "avif,webp", "none"}, func(s string) {
		v.setOption(func(o *grabber.Settings) { o.ConvertImages = s })
	})
	v.bundleCheck = widget.NewCheck("All chapters in one file", func(on bool) {
		v.setOption(func(o *grabber.Settings) { o.Bundle = on })
	})
	v.skipEntry = widget.NewEntry()
	v.skipEntry.SetPlaceHolder("e.g. 2 or 2,-1 (-1 is the last)")
	v.skipEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.SkipPages = s }) }
	v.templateEntry = widget.NewEntry()
	v.templateEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.FilenameTemplate = s }) }
	v.browserCheck = widget.NewCheck("Show the browser window", func(on bool) {
		v.setOption(func(o *grabber.Settings) { o.BrowserVisible = on })
	})

	// language and scanlator filter the list itself, so they reload it on an
	// explicit Apply (or Enter) instead of on every keystroke
	v.languageEntry = widget.NewEntry()
	v.languageEntry.SetPlaceHolder("e.g. en, es-la (empty: all)")
	v.languageEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.Language = s }) }
	v.languageEntry.OnSubmitted = func(string) { v.reloadIfStale() }
	v.scanEntry = widget.NewEntry()
	v.scanEntry.SetPlaceHolder("group name, or all (empty: default)")
	v.scanEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.Scanlator = s }) }
	v.scanEntry.OnSubmitted = func(string) { v.reloadIfStale() }
	v.applyBtn = widget.NewButtonWithIcon("Apply and reload", theme.ViewRefreshIcon(), v.reloadIfStale)

	v.optionWidgets = []fyne.Disableable{v.outputEntry, v.browseBtn, v.formatSelect, v.convertSelect,
		v.bundleCheck, v.languageEntry, v.scanEntry, v.skipEntry, v.templateEntry, v.browserCheck}

	output := widget.NewForm(
		widget.NewFormItem("Format", v.formatSelect),
		widget.NewFormItem("Bundle", v.bundleCheck),
		widget.NewFormItem("Convert images", v.convertSelect),
		widget.NewFormItem("Skip pages", v.skipEntry),
		widget.NewFormItem("File name", v.templateEntry),
		widget.NewFormItem("Browser", v.browserCheck),
	)
	filters := widget.NewForm(
		widget.NewFormItem("Language", v.languageEntry),
		widget.NewFormItem("Scanlator", v.scanEntry),
	)
	filterNote := widget.NewLabel("These filter the chapter list, so changing them reloads it.")
	filterNote.Wrapping = fyne.TextWrapWord
	filterNote.SizeName = theme.SizeNameCaptionText

	return container.NewVScroll(container.NewVBox(
		heading("Download"),
		output,
		widget.NewSeparator(),
		heading("Chapter list"),
		filters,
		container.NewBorder(nil, nil, nil, v.applyBtn, filterNote),
	))
}

func (v *view) buildProgress() fyne.CanvasObject {
	v.progressHead = widget.NewLabelWithStyle("Progress", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.jobList = widget.NewList(
		func() int { return len(v.jobs) },
		newJobRow,
		func(id widget.ListItemID, o fyne.CanvasObject) { setJobRow(o, v.jobs[id]) },
	)
	// the bars already divide the rows
	v.jobList.HideSeparators = true
	v.jobsNone = emptyHint(progressHint)

	// the full summary, failures included, selectable so a reason can be
	// copied into a bug report
	v.details = widget.NewLabel("")
	v.details.Wrapping = fyne.TextWrapWord
	v.details.Selectable = true
	detailsScroll := container.NewVScroll(v.details)
	detailsScroll.SetMinSize(fyne.NewSize(0, 96))
	v.detailsBox = container.NewVBox(widget.NewSeparator(), detailsScroll)

	return container.NewBorder(v.progressHead, v.detailsBox, nil, nil, container.NewStack(v.jobList, v.jobsNone))
}

func (v *view) buildLog() fyne.CanvasObject {
	v.logLabel = widget.NewLabel("")
	v.logLabel.Wrapping = fyne.TextWrapWord
	v.logLabel.Selectable = true
	v.logScroll = container.NewVScroll(v.logLabel)
	return v.logScroll
}

func (v *view) buildActions() fyne.CanvasObject {
	v.downloadBtn = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), v.start)
	v.downloadBtn.Importance = widget.HighImportance
	v.cancelBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), v.ctrl.Cancel)
	v.openBtn = widget.NewButtonWithIcon("Open folder", theme.FolderOpenIcon(), v.openFolder)
	v.status = widget.NewLabel("")
	v.status.Truncation = fyne.TextTruncateEllipsis

	bar := container.NewBorder(nil, nil, container.NewHBox(v.downloadBtn, v.cancelBtn), v.openBtn, v.status)
	return container.NewVBox(widget.NewSeparator(), bar)
}

// heading is a section title inside a tab
func heading(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// emptyHint is the centred text that stands in for an empty list
func emptyHint(text string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return container.NewVBox(widget.NewLabel(""), l)
}

// newJobRow is a progress row: the chapter and its status on one line, a
// slim bar under them
func newJobRow() fyne.CanvasObject {
	title := widget.NewLabel("")
	title.Truncation = fyne.TextTruncateEllipsis
	status := widget.NewLabel("")
	status.Truncation = fyne.TextTruncateEllipsis
	status.Alignment = fyne.TextAlignTrailing
	return container.NewBorder(nil, newThinBar(), nil, nil, container.NewGridWithColumns(2, title, status))
}

func setJobRow(o fyne.CanvasObject, j Job) {
	row := o.(*fyne.Container)
	grid := row.Objects[0].(*fyne.Container)
	bar := row.Objects[1].(*thinBar)
	title := grid.Objects[0].(*widget.Label)
	status := grid.Objects[1].(*widget.Label)

	title.SetText(j.Title)
	imp, colour := jobLook(j.State)
	status.Importance = imp
	status.Text = jobStatus(j)
	status.Refresh()

	frac := 0.0
	if j.Total > 0 {
		frac = float64(j.Done) / float64(j.Total)
	}
	// a failed row is filled red, so it can't pass for one still waiting
	if j.State == JobDone || j.State == JobFailed {
		frac = 1
	}
	bar.set(frac, colour)
}

// jobLook is the status colour of a row in each state, for its text and bar
func jobLook(s JobState) (widget.Importance, fyne.ThemeColorName) {
	switch s {
	case JobDone:
		return widget.SuccessImportance, theme.ColorNameSuccess
	case JobFailed:
		return widget.DangerImportance, theme.ColorNameError
	case JobSkipped:
		return widget.WarningImportance, theme.ColorNameWarning
	}
	return widget.MediumImportance, theme.ColorNamePrimary
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

// jobStatus is the status text of a progress row
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
	stale := v.ctrl.Stale()
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
	setEnabled(loaded && stale && !busy, v.applyBtn)

	total := 0
	if loaded {
		v.title.SetText(v.ctrl.Title())
		v.chapters = v.ctrl.Chapters()
		total = len(v.chapters)
	} else {
		v.title.SetText(noSeries)
		v.chapters = nil
	}
	v.chapterList.Refresh()
	setShown(loaded, v.chapterList)
	setShown(!loaded, v.chaptersNone)

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

	v.refreshNotice(stale)

	v.jobs = v.ctrl.Jobs()
	v.jobList.Refresh()
	setShown(len(v.jobs) > 0, v.jobList)
	setShown(len(v.jobs) == 0, v.jobsNone)
	finished := v.refreshProgress()

	v.refreshSummary(running, finished)
	v.refreshLog()
}

// refreshNotice fills the line under the header. A load error wins over the
// stale warning, since the list it would reload isn't there.
func (v *view) refreshNotice(stale bool) {
	switch {
	case v.loadErr != "":
		setNotice(v.notice, v.loadErr, widget.DangerImportance)
	case stale:
		setNotice(v.notice, "The language or scanlator changed. Press Apply and reload in Options to update the chapter list.", widget.WarningImportance)
	default:
		setNotice(v.notice, "", widget.MediumImportance)
	}
	// an empty label still takes a line
	setShown(v.notice.Text != "", v.notice)
}

func setNotice(l *widget.Label, text string, imp widget.Importance) {
	if l.Text == text && l.Importance == imp {
		return
	}
	l.Importance = imp
	l.SetText(text)
}

// refreshProgress says how far the run has got, and how many chapters it has.
// It returns the number of finished chapters.
func (v *view) refreshProgress() int {
	run := v.ctrl.Run()
	if run.Bundle {
		v.progressHead.SetText(fmt.Sprintf("Bundle of %s", plural(run.Chapters, "chapter", "chapters")))
		return 0
	}
	finished := 0
	for _, j := range v.jobs {
		if j.State == JobDone || j.State == JobFailed || j.State == JobSkipped {
			finished++
		}
	}
	if run.Chapters == 0 {
		v.progressHead.SetText("Progress")
		return 0
	}
	v.progressHead.SetText(fmt.Sprintf("Progress: %d of %d chapters finished", finished, run.Chapters))
	return finished
}

// refreshSummary fills the status line next to the buttons and the details
// under the progress list
func (v *view) refreshSummary(running bool, finished int) {
	s := v.ctrl.Summary()
	setEnabled(s != nil && !running, v.openBtn)
	setShown(s != nil, v.detailsBox)
	if s != nil {
		v.details.SetText(summaryText(s))
	}

	run := v.ctrl.Run()
	switch {
	case v.actionErr != "":
		setNotice(v.status, v.actionErr, widget.DangerImportance)
	case running && run.Bundle:
		setNotice(v.status, "Downloading the bundle…", widget.MediumImportance)
	case running:
		setNotice(v.status, fmt.Sprintf("Downloading: %d of %d chapters finished", finished, run.Chapters), widget.MediumImportance)
	case s != nil:
		imp := widget.SuccessImportance
		if len(s.Failures) > 0 || s.Err != nil {
			imp = widget.DangerImportance
		} else if s.Cancelled || s.Skipped > 0 {
			imp = widget.WarningImportance
		}
		setNotice(v.status, summaryHeadline(s), imp)
	default:
		setNotice(v.status, "", widget.MediumImportance)
	}
}

func (v *view) refreshLog() {
	lines := v.ctrl.Log()
	if len(lines) == v.logCount {
		return
	}
	v.logCount = len(lines)
	var b strings.Builder
	errs := 0
	for _, l := range lines {
		// most library messages already open with "error ..."
		switch {
		case l.Level == LogWarn && !strings.HasPrefix(l.Text, "warning"):
			b.WriteString("warning: ")
		case l.Level == LogError && !strings.HasPrefix(l.Text, "error"):
			b.WriteString("error: ")
		}
		if l.Level == LogError {
			errs++
		}
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	v.logLabel.SetText(strings.TrimSuffix(b.String(), "\n"))
	v.logScroll.ScrollToBottom()

	// the error count on the tab is how a failure in the log gets noticed
	// without opening it
	text := "Log"
	if errs > 0 {
		text = fmt.Sprintf("Log (%s)", plural(errs, "error", "errors"))
	}
	if v.logTab.Text != text {
		v.logTab.Text = text
		v.tabs.Refresh()
	}
}

// summaryHeadline is the short outcome shown next to the buttons; the full
// story is summaryText
func summaryHeadline(s *Summary) string {
	switch {
	case s.Cancelled && s.Bundled:
		return "Cancelled, no bundle written"
	case s.Cancelled:
		return fmt.Sprintf("Cancelled, %d saved", s.Saved)
	case s.Err != nil:
		return "The bundle could not be written"
	case s.Bundled && s.Bundle != "":
		return "Saved " + s.Bundle
	}
	var parts []string
	if s.Saved > 0 {
		parts = append(parts, fmt.Sprintf("%d saved", s.Saved))
	}
	if len(s.Failures) > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", len(s.Failures)))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	if len(parts) == 0 {
		return "Nothing was saved"
	}
	return strings.Join(parts, ", ")
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
		v.loadErr = "Enter the URL of a series first."
		v.refresh()
		return
	}
	v.loadURL(u)
}

func (v *view) loadURL(u string) {
	v.loadErr = ""
	go func() {
		err := v.ctrl.Load(u)
		fyne.Do(func() {
			v.loadErr = loadMessage(err)
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
	n, err := v.ctrl.SelectRange(v.rangeEntry.Text)
	switch {
	case err != nil:
		setNotice(v.rangeMsg, "Invalid range: "+err.Error(), widget.DangerImportance)
	case n == 0:
		setNotice(v.rangeMsg, "No chapter matches that range.", widget.DangerImportance)
	default:
		setNotice(v.rangeMsg, fmt.Sprintf("Added %s to the selection.", plural(n, "chapter", "chapters")), widget.MediumImportance)
	}
}

// start validates through the controller and starts the download. A refusal
// is shown next to the buttons; a run brings its progress into view.
func (v *view) start() {
	v.actionErr = ""
	if err := v.ctrl.Start(); err != nil {
		v.actionErr = "Can't download: " + err.Error()
	} else {
		v.tabs.Select(v.progressTab)
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
