// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/fatih/color"
)

// ── Bubble Tea messages (sent from event callbacks into the model) ──────────

type bundleStartMsg struct{ total int64 }
type chapterStartedMsg struct {
	chapter *grabber.Chapter
	total   int64
}
type pageSuccessMsg struct{ chapter *grabber.Chapter }
type downloadFailedMsg struct{ chapter *grabber.Chapter }
type archivedMsg struct{ chapter *grabber.Chapter }
type packedMsg struct {
	chapter *grabber.Chapter
	failed  bool
}
type bundlePackingMsg struct{}
type bundleArchivedMsg struct{ n int }

// downloadDoneMsg ends the program once Download returns, with its error
type downloadDoneMsg struct{ err error }

// Volume messages
type volumeStartMsg struct {
	volume  *core.Volume
	chCount int // len(v.Chapters), shown in the counter column as "N ch."
}
type volumeChapterMsg struct {
	volume  *core.Volume
	chapter *grabber.Chapter
}
type volumeProgressMsg struct {
	volume   *core.Volume
	fraction float64
}
type volumePackingMsg struct {
	volume *core.Volume
}
type volumePackedMsg struct {
	volume *core.Volume
	err    error
}

// ── Per-chapter bar state ───────────────────────────────────────────────────

type barState struct {
	rawTitle string // untruncated title; truncated to titleW at render time
	current  int64
	total    int64
	status   string          // "downloading" | "archiving" | "done" | "failed"
	bar      *progress.Model // brand-coloured bar (violet gradient while active, green when done)
	failBar  *progress.Model // danger-coloured bar, created lazily on failure
}

// ── Per-volume bar state ────────────────────────────────────────────────────

type volumeBarState struct {
	rawTitle string          // "Series - Vol. 01", untruncated
	chCount  int             // number of chapters shown in counter column as "N ch."
	fraction float64         // 0→1, set from VolumeProgress (never decreasing)
	status   string          // "downloading" | "archiving" | "done" | "failed" | "skipped"
	bar      *progress.Model // brand-coloured bar
	failBar  *progress.Model // danger-coloured bar, created lazily on failure
}

// ── Layout constants ────────────────────────────────────────────────────────

const (
	// Chapter bar row (full): title(titleW) + " " + bar(barW) + " " + counter(9) + " " + status(12)
	btCounterW  = 9                                // "9999/9999"
	btStatusW   = 12                               // " downloading"
	btGapsW     = 3                                // spaces between title, bar, counter, status
	btFixedW    = btCounterW + btStatusW + btGapsW // 24
	btMinBarW   = 15
	btMaxTitleW = 40
)

// btLayout returns titleW, barW, and narrow for a chapter row.
// When narrow is false the full row is: title(titleW) + " " + bar(barW) + " " + counter(9) + " " + status(12)
// totalling exactly width. When narrow is true the counter and status are omitted:
// title(titleW) + " " + bar(barW) totalling exactly width.
func btLayout(width int) (titleW, barW int, narrow bool) {
	if width <= 0 {
		width = 80
	}
	avail := width - btFixedW
	if avail < btMinBarW+1 {
		// Too narrow for full layout; omit counter and status.
		narrow = true
		titleW = max(1, width/3)
		barW = width - titleW - 1 // 1 space between title and bar
		if barW < 1 {
			barW = 1
			titleW = max(1, width-2)
		}
		return
	}
	titleW = min(btMaxTitleW, avail-btMinBarW)
	if titleW < 1 {
		titleW = 1
	}
	barW = avail - titleW
	return
}

// fixedW renders s in exactly w visible cells: truncates with "…" or pads with spaces.
func fixedW(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.FillRight(runewidth.Truncate(s, w, "…"), w)
}

// clipRow clips a rendered row (which may contain ANSI codes) to width visible cells.
// It is called as a last-resort guard; when layout arithmetic is correct the clip is a no-op.
func clipRow(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// ── Bubble Tea model ────────────────────────────────────────────────────────

type progressModel struct {
	seriesTitle string
	bundle      bool
	bundleRange string
	showLang    bool
	showGroup   bool
	hasDarkBg   bool
	width       int
	done        bool // set true on downloadDoneMsg; View renders full bars immediately

	// non-bundle, loose chapters
	chapters []*grabber.Chapter
	states   map[*grabber.Chapter]*barState

	// volumes support
	volumes       []*core.Volume
	volStates     map[*core.Volume]*volumeBarState
	chapterVolume map[*grabber.Chapter]*core.Volume // which volume each chapter belongs to

	// bundle
	bundleStatus  string
	bundleStarted bool // set on BundleStart; until then the counter is blank
	bundleCurrent int64
	bundleTotal   int64
	// bundleDownloaded and bundleArchived are the pages the counter shows;
	// bundlePacking is set once the archive starts (bundlePackingMsg)
	bundleDownloaded int64
	bundleArchived   int64
	bundlePacking    bool
	bundleBar        *progress.Model // created with the model, empty until BundleStart
	bundleFailBar    *progress.Model // danger-coloured bar, created when Download fails
}

var _ tea.Model = progressModel{}

func newProgressModel(seriesTitle, bundleRange string, bundle, showLang, showGroup, hasDarkBg bool) progressModel {
	m := progressModel{
		seriesTitle:   seriesTitle,
		bundle:        bundle,
		bundleRange:   bundleRange,
		showLang:      showLang,
		showGroup:     showGroup,
		hasDarkBg:     hasDarkBg,
		width:         80,
		states:        make(map[*grabber.Chapter]*barState),
		volStates:     make(map[*core.Volume]*volumeBarState),
		chapterVolume: make(map[*grabber.Chapter]*core.Volume),
	}
	if bundle {
		// core fetches every chapter to total the pages before BundleStart;
		// the bar is drawn empty meanwhile, so it's made once here
		m.bundleStatus = "gathering"
		_, barW, _ := btLayout(m.width)
		m.bundleBar = m.makeBar(barW)
	}
	return m
}

// makeBar creates a progress.Model with brand gradient colours.
// The fill glyphs are '▇' (U+2587, lower seven-eighths block) for both the
// filled and empty portions, so each row occupies only 7/8 of the cell height
// and leaves a 1-pixel gap between stacked rows.
func (m progressModel) makeBar(width int) *progress.Model {
	var from, to string
	if m.hasDarkBg {
		from = "#5b3fc4" // violet-deep
		to = "#a993ff"   // violet-soft
	} else {
		from = "#6a4bd3" // accent-hover
		to = "#7c5ce0"   // violet
	}
	b := progress.New(
		progress.WithoutPercentage(),
		progress.WithWidth(width),
		progress.WithColors(lipgloss.Color(from), lipgloss.Color(to)),
		progress.WithFillCharacters('▇', '▇'),
	)
	return &b
}

// makeDangerBar creates a progress.Model in the danger colour for failed chapters.
func makeDangerBar(width int) *progress.Model {
	const danger = "#c0344b"
	b := progress.New(
		progress.WithoutPercentage(),
		progress.WithWidth(width),
		progress.WithColors(lipgloss.Color(danger), lipgloss.Color(danger)),
		progress.WithFillCharacters('▇', '▇'),
	)
	return &b
}

func (m progressModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.BackgroundColorMsg:
		m.hasDarkBg = msg.IsDark()
		// Recolour all existing bars with the now-known background.
		_, barW, _ := btLayout(m.width)
		for _, s := range m.states {
			newBar := m.makeBar(s.bar.Width())
			if s.bar.Width() == 0 {
				newBar = m.makeBar(barW)
			}
			*s.bar = *newBar
			// failBar uses danger colour (bg-independent); no rebuild needed
		}
		for _, vs := range m.volStates {
			newBar := m.makeBar(vs.bar.Width())
			if vs.bar.Width() == 0 {
				newBar = m.makeBar(barW)
			}
			*vs.bar = *newBar
		}
		if m.bundleBar != nil {
			newBar := m.makeBar(m.bundleBar.Width())
			if m.bundleBar.Width() == 0 {
				newBar = m.makeBar(barW)
			}
			*m.bundleBar = *newBar
		}

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Interrupt
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		_, barW, _ := btLayout(m.width)
		for _, s := range m.states {
			s.bar.SetWidth(barW)
			if s.failBar != nil {
				s.failBar.SetWidth(barW)
			}
		}
		for _, vs := range m.volStates {
			vs.bar.SetWidth(barW)
			if vs.failBar != nil {
				vs.failBar.SetWidth(barW)
			}
		}
		if m.bundleBar != nil {
			m.bundleBar.SetWidth(barW)
		}
		if m.bundleFailBar != nil {
			m.bundleFailBar.SetWidth(barW)
		}

	case bundleStartMsg:
		m.bundleTotal = msg.total * (core.DownloadWeight + core.ArchiveWeight)
		m.bundleStarted = true
		m.bundleStatus = "downloading"

	case chapterStartedMsg:
		if m.bundle {
			return m, nil
		}
		// Skip chapters that belong to a volume; volumes have their own bars.
		if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
			return m, nil
		}
		_, barW, _ := btLayout(m.width)
		bar := m.makeBar(barW)
		m.chapters = append(m.chapters, msg.chapter)
		// Compute the raw (untruncated) title here so it adapts when the terminal
		// is resized; renderChapterRow truncates it to titleW at render time.
		rawTitle := chapterRawTitle(m.seriesTitle, msg.chapter, m.showLang, m.showGroup)
		m.states[msg.chapter] = &barState{
			rawTitle: rawTitle,
			total:    msg.total,
			status:   "downloading",
			bar:      bar,
		}
		return m, nil

	case pageSuccessMsg:
		if m.bundle {
			m.bundleCurrent += core.DownloadWeight
			m.bundleDownloaded++
			if m.bundleBar != nil {
				return m, m.bundleBar.SetPercent(safePercent(m.bundleCurrent, m.bundleTotal))
			}
			return m, nil
		}
		// Skip page events for chapters inside a volume.
		if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
			return m, nil
		}
		s := m.states[msg.chapter]
		if s == nil {
			return m, nil
		}
		s.current += core.DownloadWeight
		return m, s.bar.SetPercent(safePercent(s.current, s.total))

	case downloadFailedMsg:
		if !m.bundle {
			if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
				return m, nil
			}
			if s := m.states[msg.chapter]; s != nil {
				s.status = "failed"
			}
		}

	case archivedMsg:
		if m.bundle {
			return m, nil
		}
		if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
			return m, nil
		}
		s := m.states[msg.chapter]
		if s == nil {
			return m, nil
		}
		s.current += core.ArchiveWeight
		s.status = "archiving"
		return m, s.bar.SetPercent(safePercent(s.current, s.total))

	case packedMsg:
		if m.bundle {
			return m, nil
		}
		if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
			return m, nil
		}
		s := m.states[msg.chapter]
		if s == nil {
			return m, nil
		}
		if msg.failed {
			// Leave s.current as-is; the danger bar renders at actual progress.
			s.status = "failed"
			return m, nil
		}
		s.status = "done"
		return m, s.bar.SetPercent(1.0)

	case bundlePackingMsg:
		m.bundleStatus = "archiving"
		m.bundlePacking = true

	case bundleArchivedMsg:
		m.bundleCurrent += int64(msg.n) * core.ArchiveWeight
		m.bundleArchived += int64(msg.n)
		if m.bundleBar != nil {
			return m, m.bundleBar.SetPercent(safePercent(m.bundleCurrent, m.bundleTotal))
		}

	case volumeStartMsg:
		if m.bundle {
			return m, nil // volumes don't run with --bundle
		}
		_, barW, _ := btLayout(m.width)
		bar := m.makeBar(barW)
		m.volumes = append(m.volumes, msg.volume)
		m.volStates[msg.volume] = &volumeBarState{
			rawTitle: m.seriesTitle + " - " + core.VolumeLabel(msg.volume.Number),
			chCount:  msg.chCount,
			status:   "downloading",
			bar:      bar,
		}

	case volumeChapterMsg:
		m.chapterVolume[msg.chapter] = msg.volume

	case volumeProgressMsg:
		vs := m.volStates[msg.volume]
		if vs == nil {
			return m, nil
		}
		vs.fraction = msg.fraction
		return m, vs.bar.SetPercent(msg.fraction)

	case volumePackingMsg:
		if vs := m.volStates[msg.volume]; vs != nil {
			vs.status = "archiving"
		}

	case volumePackedMsg:
		vs := m.volStates[msg.volume]
		if vs == nil {
			return m, nil
		}
		if errors.Is(msg.err, core.ErrSkippedVolume) {
			vs.status = "skipped"
			return m, nil
		}
		if msg.err != nil {
			vs.status = "failed"
			return m, nil
		}
		vs.status = "done"
		return m, vs.bar.SetPercent(1.0)

	case downloadDoneMsg:
		// Mark done so View() renders all non-failed bars at 100% in the final
		// frame, bypassing the spring animation that would freeze mid-flight on Quit.
		m.done = true
		if m.bundle {
			m.bundleStatus = "done"
			if msg.err != nil {
				m.bundleStatus = "failed"
				_, barW, _ := btLayout(m.width)
				m.bundleFailBar = makeDangerBar(barW)
			}
		}
		return m, tea.Quit

	case progress.FrameMsg:
		var cmds []tea.Cmd
		for _, s := range m.states {
			var cmd tea.Cmd
			*s.bar, cmd = s.bar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if s.failBar != nil {
				var fc tea.Cmd
				*s.failBar, fc = s.failBar.Update(msg)
				if fc != nil {
					cmds = append(cmds, fc)
				}
			}
		}
		for _, vs := range m.volStates {
			var cmd tea.Cmd
			*vs.bar, cmd = vs.bar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if vs.failBar != nil {
				var fc tea.Cmd
				*vs.failBar, fc = vs.failBar.Update(msg)
				if fc != nil {
					cmds = append(cmds, fc)
				}
			}
		}
		if m.bundleBar != nil {
			var cmd tea.Cmd
			*m.bundleBar, cmd = m.bundleBar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
	}

	return m, nil
}

// safePercent returns current/total clamped to [0,1], guarding division by zero.
func safePercent(current, total int64) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(current) / float64(total)
	if p > 1 {
		return 1
	}
	return p
}

func (m progressModel) View() tea.View {
	var sb strings.Builder

	if m.bundle {
		sb.WriteString(m.renderBundleRow())
		sb.WriteByte('\n')
	} else {
		// Volume rows first (VolumeStart precedes chapter events).
		for _, v := range m.volumes {
			vs := m.volStates[v]
			if vs == nil {
				continue
			}
			sb.WriteString(m.renderVolumeRow(vs))
			sb.WriteByte('\n')
		}
		// Loose chapter rows (chapters not in any volume).
		for _, c := range m.chapters {
			if _, inVolume := m.chapterVolume[c]; inVolume {
				continue
			}
			s := m.states[c]
			if s == nil {
				continue
			}
			sb.WriteString(m.renderChapterRow(s))
			sb.WriteByte('\n')
		}
	}

	v := tea.NewView(sb.String())
	v.WindowTitle = "manga-downloader"
	return v
}

func (m progressModel) statusStyle(status string) string {
	text := fmt.Sprintf("%-12s", status)
	var fgHex string
	switch status {
	case "done":
		if m.hasDarkBg {
			fgHex = "#4fd8b0" // mint
		} else {
			fgHex = "#1b7a5e" // mint-deep
		}
	case "failed":
		fgHex = "#c0344b" // danger
	case "skipped":
		if m.hasDarkBg {
			fgHex = "#f5c400" // yellow
		} else {
			fgHex = "#b28800" // yellow-deep
		}
	default: // "downloading", "archiving"
		if m.hasDarkBg {
			fgHex = "#a993ff" // violet-soft
		} else {
			fgHex = "#5b3fc4" // violet-deep
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fgHex)).Render(text)
}

// pageCounter shows a chapter's steps as pages out of its page count: the
// downloaded pages up to the download share (so 30/30 once they're all in),
// then the archived ones
func pageCounter(current, total int64) string {
	pages := total / (core.DownloadWeight + core.ArchiveWeight)
	downloadEnd := pages * core.DownloadWeight
	if current <= downloadEnd {
		return fmt.Sprintf("%d/%d", current/core.DownloadWeight, pages)
	}
	return fmt.Sprintf("%d/%d", (current-downloadEnd)/core.ArchiveWeight, pages)
}

// bundleCounter is the bundle's pages: the downloaded ones until its archive
// starts, then the archived ones, out of its page count
func (m progressModel) bundleCounter() string {
	done := m.bundleDownloaded
	if m.bundlePacking {
		done = m.bundleArchived
	}
	return fmt.Sprintf("%d/%d", done, m.bundleTotal/(core.DownloadWeight+core.ArchiveWeight))
}

func (m progressModel) renderChapterRow(s *barState) string {
	titleW, barW, narrow := btLayout(m.width)
	if s.bar.Width() != barW {
		s.bar.SetWidth(barW)
	}
	title := fixedW(s.rawTitle, titleW)

	var barStr string
	switch {
	case s.status == "failed":
		// Danger-coloured bar at actual progress reached, never 100%.
		if s.failBar == nil {
			s.failBar = makeDangerBar(barW)
		} else if s.failBar.Width() != barW {
			s.failBar.SetWidth(barW)
		}
		barStr = s.failBar.ViewAs(safePercent(s.current, s.total))
	case m.done || s.status == "done":
		// Render done bars at full immediately, bypassing the spring animation.
		barStr = s.bar.ViewAs(1.0)
	default:
		barStr = s.bar.View()
	}

	var row string
	if narrow {
		row = title + " " + barStr
	} else {
		counter := fmt.Sprintf("%9s", pageCounter(s.current, s.total))
		row = title + " " + barStr + " " + counter + " " + m.statusStyle(s.status)
	}
	return clipRow(row, m.width)
}

func (m progressModel) renderVolumeRow(vs *volumeBarState) string {
	titleW, barW, narrow := btLayout(m.width)
	if vs.bar.Width() != barW {
		vs.bar.SetWidth(barW)
	}
	title := fixedW(vs.rawTitle, titleW)

	var barStr string
	switch {
	case vs.status == "failed":
		// Danger-coloured bar at the reached fraction, never 100%.
		if vs.failBar == nil {
			vs.failBar = makeDangerBar(barW)
		} else if vs.failBar.Width() != barW {
			vs.failBar.SetWidth(barW)
		}
		barStr = vs.failBar.ViewAs(vs.fraction)
	case m.done || vs.status == "done":
		barStr = vs.bar.ViewAs(1.0)
	default:
		barStr = vs.bar.View()
	}

	if narrow {
		return clipRow(title+" "+barStr, m.width)
	}
	// Counter column shows "N ch." instead of "current/total".
	counter := fmt.Sprintf("%9s", fmt.Sprintf("%d ch.", vs.chCount))
	row := title + " " + barStr + " " + counter + " " + m.statusStyle(vs.status)
	return clipRow(row, m.width)
}

// bundleTitle is the bundle row's title: the series and the range it packs,
// like the chapter and volume rows' "Series - Chapter 1"
func (m progressModel) bundleTitle() string {
	if m.bundleRange == "" {
		return m.seriesTitle + " - all chapters"
	}
	return m.seriesTitle + " - chapters " + m.bundleRange
}

// renderBundleRow lays the bundle out like a chapter row: title, bar, the
// "current/total" pages (blank until BundleStart gives the total) and status
func (m progressModel) renderBundleRow() string {
	titleW, barW, narrow := btLayout(m.width)

	counter := strings.Repeat(" ", btCounterW)
	if m.bundleStarted {
		counter = fmt.Sprintf("%9s", m.bundleCounter())
	}
	if extra := len(counter) - btCounterW; !narrow && extra > 0 {
		// a total of 5000+ pages outgrows the column: take the room from the
		// bar, then the title, so clipRow never cuts the status off
		take := min(extra, barW-1)
		barW -= take
		titleW = max(1, titleW-(extra-take))
	}

	if m.bundleBar.Width() != barW {
		m.bundleBar.SetWidth(barW)
	}
	title := fixedW(m.bundleTitle(), titleW)

	var barStr string
	switch {
	case m.bundleFailBar != nil:
		// Danger-coloured bar at actual progress reached, never 100%.
		if m.bundleFailBar.Width() != barW {
			m.bundleFailBar.SetWidth(barW)
		}
		barStr = m.bundleFailBar.ViewAs(safePercent(m.bundleCurrent, m.bundleTotal))
	case m.done && m.bundleCurrent < m.bundleTotal:
		// core packs the bundle without the chapters that failed, so it's
		// done but short of the total: show how far it got, not a full bar
		barStr = m.bundleBar.ViewAs(safePercent(m.bundleCurrent, m.bundleTotal))
	case m.done:
		barStr = m.bundleBar.ViewAs(1.0)
	default:
		// empty until BundleStart, as nothing sets its percent before
		barStr = m.bundleBar.View()
	}

	if narrow {
		return clipRow(title+" "+barStr, m.width)
	}
	row := title + " " + barStr + " " + counter + " " + m.statusStyle(m.bundleStatus)
	return clipRow(row, m.width)
}

// ── bubbleEvents implements core.Events by sending to a tea.Program ─────────

type bubbleEvents struct {
	prog *tea.Program
	done <-chan struct{} // closed after prog.Run() returns
}

var _ core.Events = (*bubbleEvents)(nil)

// println prints a line above the TUI, or to stderr if the program has stopped.
// It uses prog.Send (which selects on ctx.Done) instead of prog.Println (which
// blocks forever on a closed channel), so it never hangs after Run() returns.
func (e *bubbleEvents) println(s string) {
	if interrupted.Load() != 0 {
		return
	}
	select {
	case <-e.done:
		fmt.Fprintln(os.Stderr, s)
	default:
		e.prog.Send(tea.Println(s)())
	}
}

func (e *bubbleEvents) BundleStart(pages int64) {
	e.prog.Send(bundleStartMsg{total: pages})
}

func (e *bubbleEvents) FetchFailed(listed grabber.Filterable, err error) {
	e.println(color.RedString("- error fetching chapter %s: %s", listed.GetTitle(), err.Error()))
}

func (e *bubbleEvents) Skipped(c *grabber.Chapter) {
	e.println(color.YellowString(
		"- skipping chapter %s: --skip-pages excludes all of its pages", c.GetTitle()))
}

func (e *bubbleEvents) FilenameFailed(c *grabber.Chapter, err error) {
	e.println(color.RedString(
		"- error creating filename for chapter %s: %s", c.GetTitle(), err.Error()))
}

func (e *bubbleEvents) Started(c *grabber.Chapter) {
	e.prog.Send(chapterStartedMsg{chapter: c, total: int64(c.PagesCount) * (core.DownloadWeight + core.ArchiveWeight)})
}

func (e *bubbleEvents) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		e.println(color.RedString("- error downloading page %d: %s", idx+1, err.Error()))
		return
	}
	e.prog.Send(pageSuccessMsg{chapter: c})
}

func (e *bubbleEvents) DownloadFailed(c *grabber.Chapter, err error) {
	e.println(color.RedString(
		"- error downloading chapter %s: %s", c.GetTitle(), err.Error()))
	e.prog.Send(downloadFailedMsg{chapter: c})
}

func (e *bubbleEvents) Archived(c *grabber.Chapter) {
	e.prog.Send(archivedMsg{chapter: c})
}

func (e *bubbleEvents) Packed(c *grabber.Chapter, _ string, err error) {
	if err != nil {
		e.println(color.RedString(err.Error()))
	}
	e.prog.Send(packedMsg{chapter: c, failed: err != nil})
}

// Downloaded sends nothing: the pages were counted as they came in, and
// adding them again here overshot the bundle's total
func (e *bubbleEvents) Downloaded(*grabber.Chapter) {}

func (e *bubbleEvents) BundlePacking() {
	e.prog.Send(bundlePackingMsg{})
}

func (e *bubbleEvents) BundleArchived(n int) {
	e.prog.Send(bundleArchivedMsg{n: n})
}

func (e *bubbleEvents) VolumeStart(v *core.Volume) {
	e.prog.Send(volumeStartMsg{volume: v, chCount: len(v.Chapters)})
}

func (e *bubbleEvents) VolumeChapter(v *core.Volume, c *grabber.Chapter) {
	e.prog.Send(volumeChapterMsg{volume: v, chapter: c})
}

func (e *bubbleEvents) VolumePacking(v *core.Volume) {
	e.prog.Send(volumePackingMsg{volume: v})
}

func (e *bubbleEvents) VolumeProgress(v *core.Volume, fraction float64) {
	e.prog.Send(volumeProgressMsg{volume: v, fraction: fraction})
}

func (e *bubbleEvents) VolumePacked(v *core.Volume, name string, err error) {
	if errors.Is(err, core.ErrSkippedVolume) {
		e.println(color.YellowString("- skipping %s: %s", core.VolumeLabel(v.Number), err.Error()))
	} else if err != nil {
		e.println(color.RedString("- error packing %s: %s", core.VolumeLabel(v.Number), err.Error()))
	}
	e.prog.Send(volumePackedMsg{volume: v, err: err})
}

// ── plainEvents implements core.Events for non-TTY output ───────────────────

type plainEvents struct{}

var _ core.Events = plainEvents{}

func (plainEvents) BundleStart(int64)           {}
func (plainEvents) Downloaded(*grabber.Chapter) {}
func (plainEvents) BundlePacking()              {}
func (plainEvents) BundleArchived(int)          {}
func (plainEvents) Archived(*grabber.Chapter)   {}

func (plainEvents) FetchFailed(listed grabber.Filterable, err error) {
	red("- error fetching chapter %s: %s", listed.GetTitle(), err.Error())
}

func (plainEvents) Skipped(c *grabber.Chapter) {
	yellow("- skipping chapter %s: --skip-pages excludes all of its pages", c.GetTitle())
}

func (plainEvents) FilenameFailed(c *grabber.Chapter, err error) {
	red("- error creating filename for chapter %s: %s", c.GetTitle(), err.Error())
}

func (plainEvents) Started(c *grabber.Chapter) {
	fmt.Fprintf(color.Output, "downloading %s...\n", c.GetTitle())
}

func (plainEvents) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		red("- error downloading page %d: %s", idx+1, err.Error())
	}
}

func (plainEvents) DownloadFailed(c *grabber.Chapter, err error) {
	red("- error downloading chapter %s: %s", c.GetTitle(), err.Error())
}

func (plainEvents) Packed(_ *grabber.Chapter, _ string, err error) {
	if err != nil {
		red(err.Error())
	}
}

func (plainEvents) VolumeStart(v *core.Volume) {
	fmt.Fprintf(color.Output, "downloading %s...\n", core.VolumeLabel(v.Number))
}

func (plainEvents) VolumeChapter(*core.Volume, *grabber.Chapter) {}

func (plainEvents) VolumePacking(v *core.Volume) {
	fmt.Fprintf(color.Output, "archiving %s...\n", core.VolumeLabel(v.Number))
}

func (plainEvents) VolumeProgress(*core.Volume, float64) {}

func (plainEvents) VolumePacked(v *core.Volume, _ string, err error) {
	if errors.Is(err, core.ErrSkippedVolume) {
		yellow("- skipping %s: %s", core.VolumeLabel(v.Number), err.Error())
		return
	}
	if err != nil {
		red("- error packing %s: %s", core.VolumeLabel(v.Number), err.Error())
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// chapterRawTitle builds an untruncated "Series - Chapter [lang] [group]" title
// for a chapter. The caller passes showLanguage (true when the series has
// chapters in more than one language) and showGroup (true when one number is
// listed more than once in a language, see core.HasGroupDuplicates). The result
// is stored in barState.rawTitle and truncated to titleW at render time so
// resizing the terminal shows more text.
func chapterRawTitle(seriesTitle string, c *grabber.Chapter, showLanguage, showGroup bool) string {
	chapTitle := c.GetTitle()
	if showLanguage && c.Language != "" {
		chapTitle += " [" + c.Language + "]"
	}
	if showGroup && c.Group != "" {
		chapTitle += " [" + c.Group + "]"
	}
	if seriesTitle == "" {
		return chapTitle
	}
	return seriesTitle + " - " + chapTitle
}

// teaNoticeHandler returns a notice handler that routes output through the TUI.
// It respects the interrupted mute flag and falls back to stderr when the
// program has stopped (done channel closed).
func teaNoticeHandler(e *bubbleEvents) func(notice.Level, string) {
	return func(level notice.Level, msg string) {
		if level == notice.LevelWarn {
			e.println(color.YellowString(msg))
		} else {
			e.println(color.BlueString(msg))
		}
	}
}

// teaAttentionHandler returns a browser attention handler that routes output
// through the TUI, with the same done-channel fallback as teaNoticeHandler.
func teaAttentionHandler(e *bubbleEvents) func(browser.Attention) {
	return func(a browser.Attention) {
		if !a.Active {
			return
		}
		e.println(attentionColor.Sprint(a.Message()))
	}
}
