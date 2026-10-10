// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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
	progressHint = "Select chapters and press Download to follow them here."
	rangeHint    = "A range like 1-10,12 adds those chapters to the selection."
)

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
	// notice is the line under the header: a load error, or the warning that
	// the list no longer matches its filters
	notice *widget.Label
	// noticeBox is the tinted banner around notice, noticeIcon its glyph
	noticeBox  *surface
	noticeIcon *widget.Icon
	// loadErr is the last load error, which notice shows ahead of anything else
	loadErr string

	title        *widget.Label
	count        *widget.Label
	countPill    *pill
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
	status     *widget.Label
	statusPill *pill
	// actionErr is the last Start refusal, cleared by the next attempt
	actionErr string

	progressHead *widget.Label
	// overallBar is the whole run at a glance, above the per-chapter rows
	overallBar *thinBar
	jobList    *widget.List
	jobsNone   fyne.CanvasObject
	jobs       []Job
	details    *widget.Label
	detailsBox fyne.CanvasObject

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

// The layout, top to bottom: a header band with the icon and the series
// title, the URL and output rows, a split body (chapters on the left,
// options/progress/log tabs on the right) and a fixed action bar, so nothing
// a run changes can push the buttons around.
func (v *view) build() {
	header := v.buildHeader()
	chapters := v.buildChapters()

	v.optionsTab = container.NewTabItemWithIcon("Options", theme.SettingsIcon(), v.buildOptions())
	v.progressTab = container.NewTabItemWithIcon("Progress", theme.DownloadIcon(), v.buildProgress())
	v.logTab = container.NewTabItemWithIcon("Log", theme.DocumentIcon(), v.buildLog())
	v.tabs = container.NewAppTabs(v.optionsTab, v.progressTab, v.logTab)

	body := container.NewHSplit(padded(chapters), v.tabs)
	body.Offset = 0.5

	v.content = container.NewBorder(header, v.buildActions(), nil, nil, body)
}

func (v *view) buildHeader() fyne.CanvasObject {
	// the brand band: the app's icon and name, and the series once loaded,
	// which is the one thing the whole window is about
	icon := canvas.NewImageFromResource(appIcon)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSquareSize(44))
	appName := mutedText("MANGA DOWNLOADER", theme.SizeNameCaptionText, true)
	v.title = widget.NewLabelWithStyle(noSeries, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.title.SizeName = theme.SizeNameSubHeadingText
	v.title.Truncation = fyne.TextTruncateEllipsis
	names := container.New(layout.NewCustomPaddedVBoxLayout(-2*theme.Padding()), appName, v.title)
	brand := newSurface(container.NewBorder(nil, nil, container.NewCenter(icon), nil, names),
		theme.ColorNameHeaderBackground, 0)
	brand.inset = 2

	v.urlEntry = widget.NewEntry()
	v.urlEntry.SetPlaceHolder("Series URL, e.g. https://…")
	v.urlEntry.OnSubmitted = func(string) { v.load() }
	v.loadBtn = widget.NewButtonWithIcon("Load", theme.SearchIcon(), v.load)
	v.loadBtn.Importance = widget.HighImportance
	v.spinner = widget.NewActivity()
	labels := fieldLabels("Series", "Save to")
	urlRow := container.NewBorder(nil, nil, labels[0], container.NewHBox(v.spinner, v.loadBtn), v.urlEntry)

	// the output folder is what people look for first, so it's always in view
	v.outputEntry = widget.NewEntry()
	v.outputEntry.OnChanged = func(s string) { v.setOption(func(o *grabber.Settings) { o.OutputDir = s }) }
	v.browseBtn = widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), v.browse)
	outputRow := container.NewBorder(nil, nil, labels[1], v.browseBtn, v.outputEntry)

	v.notice = widget.NewLabel("")
	v.notice.Wrapping = fyne.TextWrapWord
	v.noticeIcon = widget.NewIcon(nil)
	v.noticeBox = newSurface(container.NewBorder(nil, nil, container.NewCenter(v.noticeIcon), nil, v.notice),
		theme.ColorNameWarning, tintSoft)
	v.noticeBox.radius = theme.InputRadiusSize()
	v.noticeBox.Hide()

	fields := padded(container.NewVBox(urlRow, outputRow, v.noticeBox))
	return container.NewVBox(brand, widget.NewSeparator(), fields, widget.NewSeparator())
}

func (v *view) buildChapters() fyne.CanvasObject {
	v.count = widget.NewLabel("")
	v.count.SizeName = theme.SizeNameCaptionText
	v.count.TextStyle = fyne.TextStyle{Bold: true}
	v.countPill = newPill(v.count)

	// the selection helpers are secondary to the list itself, so they're
	// flat; Add range keeps a face because it acts on what's typed next to it
	selAll := widget.NewButton("Select all", func() { v.ctrl.SetAll(true) })
	selAll.Importance = widget.LowImportance
	selNone := widget.NewButton("Select none", func() { v.ctrl.SetAll(false) })
	selNone.Importance = widget.LowImportance
	v.rangeEntry = widget.NewEntry()
	v.rangeEntry.SetPlaceHolder("e.g. 1-10,12")
	v.rangeEntry.OnSubmitted = func(string) { v.selectRange() }
	addRange := widget.NewButtonWithIcon("Add range", theme.ContentAddIcon(), v.selectRange)
	v.selButtons = []fyne.Disableable{selAll, selNone, addRange, v.rangeEntry}
	// the hint and a bad range share one line, so an error doesn't move the list
	v.rangeMsg = widget.NewLabel(rangeHint)
	v.rangeMsg.Truncation = fyne.TextTruncateEllipsis
	v.rangeMsg.SizeName = theme.SizeNameCaptionText

	v.chapterList = widget.NewList(
		func() int { return len(v.chapters) },
		func() fyne.CanvasObject { return newChapterRow() },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			i := int(id)
			o.(*chapterRow).set(v.chapters[i].GetTitle(), v.chapterLanguage(v.chapters[i]), v.ctrl.IsSelected(i),
				func(on bool) { v.ctrl.SetSelected(i, on) })
		},
	)
	logo := canvas.NewImageFromResource(appIcon)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSquareSize(72))
	v.chaptersNone = emptyState(logo, "Start with a series", chaptersHint)

	head := container.NewVBox(
		container.NewBorder(nil, nil, sectionLabel("Chapters"), v.countPill),
		container.NewBorder(nil, nil, container.NewHBox(selAll, selNone), addRange, v.rangeEntry),
		v.rangeMsg,
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
	// the one primary action of this tab; it's only enabled when there's
	// something to apply
	v.applyBtn = widget.NewButtonWithIcon("Apply and reload", theme.ViewRefreshIcon(), v.reloadIfStale)
	v.applyBtn.Importance = widget.HighImportance

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
	filterNote := mutedText("These filter the chapter list, so changing them reloads it.", theme.SizeNameCaptionText, false)
	filterNote.Wrapping = fyne.TextWrapWord

	return container.NewVScroll(padded(container.NewVBox(
		sectionLabel("Download"),
		output,
		gap(),
		sectionLabel("Chapter list"),
		filters,
		container.NewBorder(nil, nil, nil, v.applyBtn, filterNote),
	)))
}

func (v *view) buildProgress() fyne.CanvasObject {
	v.progressHead = widget.NewLabelWithStyle("Progress", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.progressHead.Truncation = fyne.TextTruncateEllipsis
	v.overallBar = newThinBar(overallHeight)
	head := container.NewVBox(v.progressHead, padH(v.overallBar), gap())

	v.jobList = widget.NewList(
		func() int { return len(v.jobs) },
		func() fyne.CanvasObject { return newJobRow() },
		func(id widget.ListItemID, o fyne.CanvasObject) { o.(*jobRow).set(v.jobs[id]) },
	)
	// the bars already divide the rows
	v.jobList.HideSeparators = true
	v.jobsNone = emptyState(container.NewGridWrap(fyne.NewSquareSize(56),
		widget.NewIcon(theme.NewColoredResource(theme.DownloadIcon(), theme.ColorNamePlaceHolder))),
		"No downloads yet", progressHint)

	// the full summary, failures included, selectable so a reason can be
	// copied into a bug report
	v.details = widget.NewLabel("")
	v.details.Wrapping = fyne.TextWrapWord
	v.details.Selectable = true
	detailsScroll := container.NewVScroll(v.details)
	detailsScroll.SetMinSize(fyne.NewSize(0, 96))
	summary := newSurface(container.NewBorder(sectionLabel("Summary"), nil, nil, nil, detailsScroll),
		theme.ColorNameHeaderBackground, 0)
	summary.radius = theme.InputRadiusSize()
	v.detailsBox = container.NewVBox(gap(), summary)

	return padded(container.NewBorder(head, v.detailsBox, nil, nil, container.NewStack(v.jobList, v.jobsNone)))
}

func (v *view) buildLog() fyne.CanvasObject {
	v.logLabel = widget.NewLabel("")
	v.logLabel.Wrapping = fyne.TextWrapWord
	v.logLabel.Selectable = true
	// a log reads as one: fixed-width lines line up their prefixes
	v.logLabel.TextStyle = fyne.TextStyle{Monospace: true}
	v.logLabel.SizeName = theme.SizeNameCaptionText
	v.logScroll = container.NewVScroll(v.logLabel)
	return padded(v.logScroll)
}

func (v *view) buildActions() fyne.CanvasObject {
	v.downloadBtn = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), v.start)
	v.downloadBtn.Importance = widget.HighImportance
	v.cancelBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), v.ctrl.Cancel)
	v.openBtn = widget.NewButtonWithIcon("Open folder", theme.FolderOpenIcon(), v.openFolder)
	v.status = widget.NewLabel("")
	v.status.Truncation = fyne.TextTruncateEllipsis
	v.status.TextStyle = fyne.TextStyle{Bold: true}
	v.statusPill = newPill(v.status)

	bar := container.NewBorder(nil, nil, container.NewHBox(v.downloadBtn, v.cancelBtn), v.openBtn, padH(v.statusPill))
	strip := newSurface(bar, theme.ColorNameHeaderBackground, 0)
	strip.inset = 1.5
	return container.NewVBox(widget.NewSeparator(), strip)
}

// sectionLabel is a muted, upper-case section title: it orders the window
// without competing with the content under it
func sectionLabel(text string) *widget.RichText {
	return mutedText(strings.ToUpper(text), theme.SizeNameCaptionText, true)
}

// fieldLabels are the labels in front of the header rows' entries, all as
// wide as the widest so the entries after them start on one line
func fieldLabels(texts ...string) []fyne.CanvasObject {
	labels := make([]*widget.Label, len(texts))
	w := float32(0)
	for i, t := range texts {
		labels[i] = widget.NewLabelWithStyle(t, fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
		w = max(w, labels[i].MinSize().Width)
	}
	out := make([]fyne.CanvasObject, len(labels))
	for i, l := range labels {
		strut := canvas.NewRectangle(color.Transparent)
		strut.SetMinSize(fyne.NewSize(w, 0))
		out[i] = container.NewStack(strut, l)
	}
	return out
}

// mutedText is secondary text in the theme's placeholder colour. A Label's
// low importance would use the disabled colour, which is close to invisible
// on the dark background.
func mutedText(text string, size fyne.ThemeSizeName, bold bool) *widget.RichText {
	return widget.NewRichText(&widget.TextSegment{Text: text, Style: widget.RichTextStyle{
		ColorName: theme.ColorNamePlaceHolder,
		SizeName:  size,
		TextStyle: fyne.TextStyle{Bold: bold},
		Inline:    true,
	}})
}

// padded insets o by the theme padding on every side
func padded(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewPadded(o)
}

// padH insets o horizontally by the text's inner padding, so a bar or badge
// lines up with the label text around it
func padH(o fyne.CanvasObject) fyne.CanvasObject {
	p := theme.InnerPadding()
	return container.New(layout.NewCustomPaddedLayout(0, 0, p, p), o)
}

// gap is a blank padding-high space between groups
func gap() fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, theme.Padding()))
	return r
}

// emptyState stands in for an empty list: a picture, a title and a hint,
// centred in the space the list would take
func emptyState(picture fyne.CanvasObject, title, hint string) fyne.CanvasObject {
	t := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	t.SizeName = theme.SizeNameSubHeadingText
	h := widget.NewRichText(&widget.TextSegment{Text: hint, Style: widget.RichTextStyle{
		ColorName: theme.ColorNamePlaceHolder,
		Alignment: fyne.TextAlignCenter,
	}})
	h.Wrapping = fyne.TextWrapWord
	return container.NewVBox(layout.NewSpacer(), container.NewCenter(picture), t, h, layout.NewSpacer())
}

// chapterRow is a chapter list row: its checkbox and title, and a language
// badge when the list needs one to tell chapters apart
type chapterRow struct {
	widget.BaseWidget
	check *widget.Check
	lang  *widget.Label
	badge *pill
}

func newChapterRow() *chapterRow {
	r := &chapterRow{check: widget.NewCheck("", nil), lang: widget.NewLabel("")}
	r.lang.SizeName = theme.SizeNameCaptionText
	r.lang.TextStyle = fyne.TextStyle{Bold: true}
	r.lang.Importance = widget.HighImportance
	r.lang.Alignment = fyne.TextAlignCenter
	r.badge = newPill(r.lang)
	r.ExtendBaseWidget(r)
	return r
}

// set fills the row; onChange is attached after the check is set, so filling
// a recycled row never toggles a chapter
func (r *chapterRow) set(title, lang string, checked bool, onChange func(bool)) {
	r.check.OnChanged = nil
	// SetText, not Text: SetChecked doesn't redraw when the state is
	// unchanged, which left a recycled row showing its previous title
	r.check.SetText(title)
	r.check.SetChecked(checked)
	r.check.OnChanged = onChange
	if r.lang.Text != lang {
		r.lang.Text = lang
		r.badge.Refresh()
	}
	setShown(lang != "", r.badge)
}

func (r *chapterRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, nil, padH(r.badge), r.check))
}

// jobRow is a progress row: a state icon, the chapter and its status on one
// line, and a slim bar under them
type jobRow struct {
	widget.BaseWidget
	icon          *widget.Icon
	title, status *widget.Label
	bar           *thinBar
}

func newJobRow() *jobRow {
	r := &jobRow{icon: widget.NewIcon(nil), title: widget.NewLabel(""), status: widget.NewLabel(""), bar: newThinBar(barHeight)}
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.status.Truncation = fyne.TextTruncateEllipsis
	r.status.Alignment = fyne.TextAlignTrailing
	r.ExtendBaseWidget(r)
	return r
}

func (r *jobRow) CreateRenderer() fyne.WidgetRenderer {
	p := theme.InnerPadding()
	bar := container.New(layout.NewCustomPaddedLayout(-theme.Padding(), p/2, p, p), r.bar)
	line := container.NewBorder(nil, nil, padH(r.icon), nil, container.NewGridWithColumns(2, r.title, r.status))
	return widget.NewSimpleRenderer(container.NewBorder(nil, bar, nil, nil, line))
}

func (r *jobRow) set(j Job) {
	r.title.SetText(j.Title)
	imp, colour, icon := jobLook(j.State)
	r.status.Importance = imp
	r.status.Text = jobStatus(j)
	frac := 0.0
	if j.Total > 0 {
		frac = float64(j.Done) / float64(j.Total)
	}
	// a running row says how far it's got, which the bar alone can't say
	// precisely
	if (j.State == JobDownloading || j.State == JobArchiving) && j.Total > 0 {
		r.status.Text = fmt.Sprintf("%s  %d%%", r.status.Text, int(frac*100))
	}
	r.status.Refresh()
	r.icon.SetResource(icon)

	// a failed row is filled red, so it can't pass for one still waiting
	if j.State == JobDone || j.State == JobFailed {
		frac = 1
	}
	r.bar.set(frac, colour)
}

// jobLook is the status colour and icon of a row in each state
func jobLook(s JobState) (widget.Importance, fyne.ThemeColorName, fyne.Resource) {
	switch s {
	case JobDone:
		return widget.SuccessImportance, theme.ColorNameSuccess, theme.NewSuccessThemedResource(theme.ConfirmIcon())
	case JobFailed:
		return widget.DangerImportance, theme.ColorNameError, theme.NewErrorThemedResource(theme.ErrorIcon())
	case JobSkipped:
		return widget.WarningImportance, theme.ColorNameWarning, theme.NewWarningThemedResource(theme.MediaSkipNextIcon())
	case JobArchiving:
		return widget.HighImportance, theme.ColorNamePrimary, theme.NewPrimaryThemedResource(theme.StorageIcon())
	}
	return widget.HighImportance, theme.ColorNamePrimary, theme.NewPrimaryThemedResource(theme.DownloadIcon())
}

// chapterLanguage is the language badge of a chapter's row: only shown when
// the list repeats numbers across languages (see
// core.HasDuplicateChapterNumbers), where it tells the copies apart
func (v *view) chapterLanguage(ch grabber.Filterable) string {
	if !v.ctrl.HasDuplicates() {
		return ""
	}
	if l, ok := ch.(interface{ GetLanguage() string }); ok {
		return l.GetLanguage()
	}
	return ""
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
	v.count.Text = fmt.Sprintf("%d of %d selected", selected, total)
	v.count.Importance = widget.MediumImportance
	if selected > 0 {
		v.count.Importance = widget.HighImportance
	}
	v.countPill.Refresh()
	setShown(loaded, v.countPill)
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
	switch v.notice.Importance {
	case widget.DangerImportance:
		v.noticeBox.setTint(theme.ColorNameError, tintSoft)
		v.noticeIcon.SetResource(theme.NewErrorThemedResource(theme.ErrorIcon()))
	default:
		v.noticeBox.setTint(theme.ColorNameWarning, tintSoft)
		v.noticeIcon.SetResource(theme.NewWarningThemedResource(theme.WarningIcon()))
	}
	// an empty banner would still take a line
	setShown(v.notice.Text != "", v.noticeBox)
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
	setShown(run.Chapters > 0, v.overallBar)
	if run.Bundle {
		v.progressHead.SetText(fmt.Sprintf("Bundle of %s", plural(run.Chapters, "chapter", "chapters")))
		// the bundle's single row already counts its pages; the overall bar
		// mirrors it
		if len(v.jobs) > 0 {
			j := v.jobs[0]
			frac := 0.0
			if j.Total > 0 {
				frac = float64(j.Done) / float64(j.Total)
			}
			if j.State == JobDone || j.State == JobFailed {
				frac = 1
			}
			_, colour, _ := jobLook(j.State)
			v.overallBar.set(frac, colour)
		}
		return 0
	}
	// the overall bar stacks saved, skipped and failed chapters, then the
	// pages of the ones still going, so it creeps forward page by page and a
	// failure shows the moment it happens
	done, skipped, failed, partial := 0, 0, 0, 0.0
	for _, j := range v.jobs {
		switch j.State {
		case JobDone:
			done++
		case JobSkipped:
			skipped++
		case JobFailed:
			failed++
		default:
			if j.Total > 0 {
				partial += float64(j.Done) / float64(j.Total)
			}
		}
	}
	finished := done + skipped + failed
	if run.Chapters == 0 {
		v.progressHead.SetText("Progress")
		return 0
	}
	v.progressHead.SetText(fmt.Sprintf("%d of %d chapters finished", finished, run.Chapters))
	n := float64(run.Chapters)
	v.overallBar.setSegments(
		barSegment{float64(done) / n, theme.ColorNameSuccess},
		barSegment{float64(skipped) / n, theme.ColorNameWarning},
		barSegment{float64(failed) / n, theme.ColorNameError},
		barSegment{partial / n, theme.ColorNamePrimary},
	)
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
		setNotice(v.status, "Downloading the bundle…", widget.HighImportance)
	case running:
		setNotice(v.status, fmt.Sprintf("Downloading: %d of %d chapters finished", finished, run.Chapters), widget.HighImportance)
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
	v.statusPill.Refresh()
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
	text, icon := "Log", theme.DocumentIcon()
	if errs > 0 {
		text = fmt.Sprintf("Log (%s)", plural(errs, "error", "errors"))
		icon = theme.NewErrorThemedResource(theme.ErrorIcon())
	}
	if v.logTab.Text != text {
		v.logTab.Text = text
		v.logTab.Icon = icon
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
	// BeginLoad runs here, not in the goroutine, so the controls are disabled
	// before a second click or Enter can start another load
	if err := v.ctrl.BeginLoad(u); err != nil {
		v.loadErr = loadMessage(err)
		v.refresh()
		return
	}
	go func() {
		err := v.ctrl.FinishLoad(u)
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
