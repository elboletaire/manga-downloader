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
type bundleDownloadedMsg struct{ pages int }
type bundlePackingMsg struct{}
type bundleArchivedMsg struct{ n int }
type downloadDoneMsg struct{}

// ── Per-chapter bar state ───────────────────────────────────────────────────

type barState struct {
	rawTitle string // untruncated title; truncated to titleW at render time
	current  int64
	total    int64
	status   string          // "downloading" | "archiving" | "done" | "failed"
	bar      *progress.Model // brand-coloured bar (violet gradient while active, green when done)
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

	// Bundle bar row (full): phase(30) + " " + bar(barW) + " " + counter(9)
	btPhaseW       = 30
	btBundleGapsW  = 2
	btBundleFixedW = btPhaseW + btCounterW + btBundleGapsW // 41
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

// btBundleLayout returns phaseW, barW, and counterW for a bundle row.
// It degrades gracefully as the terminal narrows:
//   - Full  (width ≥ 56): phase(30) + " " + bar(width-41) + " " + counter(9)
//   - Shrunk (width ≥ 26): phase(width-26) + " " + bar(15) + " " + counter(9)
//   - No-phase (width ≥ 11): bar(width-10) + " " + counter(9)
//   - Tiny (width < 11): bar(width) only
func btBundleLayout(width int) (phaseW, barW, counterW int) {
	if width <= 0 {
		width = 80
	}
	switch {
	case width >= btBundleFixedW+btMinBarW: // ≥ 56
		return btPhaseW, width - btBundleFixedW, btCounterW
	case width >= btMinBarW+1+btCounterW+1: // ≥ 26: shrink phase
		phaseW = width - btMinBarW - 1 - btCounterW - 1
		return phaseW, btMinBarW, btCounterW
	case width >= btMinBarW+1: // ≥ 16: drop phase, keep counter
		return 0, width - btCounterW - 1, btCounterW
	default: // < 16: bar only
		return 0, max(1, width), 0
	}
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
	hasDarkBg   bool
	width       int
	done        bool // set true on downloadDoneMsg; View renders full bars immediately

	// non-bundle
	chapters []*grabber.Chapter
	states   map[*grabber.Chapter]*barState

	// bundle
	bundlePhase   string
	bundleCurrent int64
	bundleTotal   int64
	bundleBar     *progress.Model
}

var _ tea.Model = progressModel{}

func newProgressModel(seriesTitle, bundleRange string, bundle, showLang, hasDarkBg bool) progressModel {
	return progressModel{
		seriesTitle: seriesTitle,
		bundle:      bundle,
		bundleRange: bundleRange,
		showLang:    showLang,
		hasDarkBg:   hasDarkBg,
		width:       80,
		states:      make(map[*grabber.Chapter]*barState),
	}
}

// makeBar creates a progress.Model with brand gradient colours.
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
	)
	return &b
}

// makeDoneBar creates a progress.Model in the mint/done colour.
func (m progressModel) makeDoneBar(width int) *progress.Model {
	var c string
	if m.hasDarkBg {
		c = "#4fd8b0" // mint
	} else {
		c = "#1b7a5e" // mint-deep
	}
	b := progress.New(
		progress.WithoutPercentage(),
		progress.WithWidth(width),
		progress.WithColors(lipgloss.Color(c), lipgloss.Color(c)),
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
	)
	return &b
}

func (m progressModel) Init() tea.Cmd { return nil }

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

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
		_, bBarW, _ := btBundleLayout(m.width)
		if m.bundleBar != nil {
			m.bundleBar.SetWidth(bBarW)
		}

	case bundleStartMsg:
		m.bundleTotal = msg.total * 2
		_, bBarW, _ := btBundleLayout(m.width)
		bar := m.makeBar(bBarW)
		m.bundleBar = bar
		m.bundlePhase = "Gathering info..."

	case chapterStartedMsg:
		if m.bundle {
			m.bundlePhase = "Downloading " + truncateString(msg.chapter.GetTitle(), btPhaseW-12)
			return m, nil
		}
		_, barW, _ := btLayout(m.width)
		bar := m.makeBar(barW)
		m.chapters = append(m.chapters, msg.chapter)
		// Compute the raw (untruncated) title here so it adapts when the terminal
		// is resized; renderChapterRow truncates it to titleW at render time.
		rawTitle := chapterRawTitle(m.seriesTitle, msg.chapter, m.showLang)
		m.states[msg.chapter] = &barState{
			rawTitle: rawTitle,
			total:    msg.total,
			status:   "downloading",
			bar:      bar,
		}
		return m, nil

	case pageSuccessMsg:
		if m.bundle {
			m.bundleCurrent++
			if m.bundleBar != nil {
				return m, m.bundleBar.SetPercent(safePercent(m.bundleCurrent, m.bundleTotal))
			}
			return m, nil
		}
		s := m.states[msg.chapter]
		if s == nil {
			return m, nil
		}
		s.current++
		if s.current > s.total/2 {
			s.status = "archiving"
		}
		return m, s.bar.SetPercent(safePercent(s.current, s.total))

	case downloadFailedMsg:
		if !m.bundle {
			if s := m.states[msg.chapter]; s != nil {
				s.status = "failed"
			}
		}

	case archivedMsg:
		if m.bundle {
			return m, nil
		}
		s := m.states[msg.chapter]
		if s == nil {
			return m, nil
		}
		s.current++
		s.status = "archiving"
		return m, s.bar.SetPercent(safePercent(s.current, s.total))

	case packedMsg:
		if m.bundle {
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

	case bundleDownloadedMsg:
		m.bundleCurrent += int64(msg.pages)
		if m.bundleBar != nil {
			return m, m.bundleBar.SetPercent(safePercent(m.bundleCurrent, m.bundleTotal))
		}

	case bundlePackingMsg:
		m.bundlePhase = truncateString(
			fmt.Sprintf("Creating bundle for chapters %s", m.bundleRange), btPhaseW)

	case bundleArchivedMsg:
		m.bundleCurrent += int64(msg.n)
		if m.bundleBar != nil {
			return m, m.bundleBar.SetPercent(safePercent(m.bundleCurrent, m.bundleTotal))
		}

	case downloadDoneMsg:
		// Mark done so View() renders all non-failed bars at 100% in the final
		// frame, bypassing the spring animation that would freeze mid-flight on Quit.
		m.done = true
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
		for _, c := range m.chapters {
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
	default:
		if m.hasDarkBg {
			fgHex = "#a993ff" // violet-soft
		} else {
			fgHex = "#5b3fc4" // violet-deep
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fgHex)).Render(text)
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
		counter := fmt.Sprintf("%9s", fmt.Sprintf("%d/%d", s.current, s.total))
		row = title + " " + barStr + " " + counter + " " + m.statusStyle(s.status)
	}
	return clipRow(row, m.width)
}

func (m progressModel) renderBundleRow() string {
	phaseW, barW, counterW := btBundleLayout(m.width)

	if m.bundleBar != nil && m.bundleBar.Width() != barW {
		m.bundleBar.SetWidth(barW)
	}

	var barStr string
	switch {
	case m.bundleBar == nil:
		barStr = strings.Repeat("░", barW)
	case m.done:
		barStr = m.bundleBar.ViewAs(1.0)
	default:
		barStr = m.bundleBar.View()
	}

	var row string
	switch {
	case phaseW > 0 && counterW > 0:
		phase := fixedW(m.bundlePhase, phaseW)
		counter := fmt.Sprintf("%9s", fmt.Sprintf("%d/%d", m.bundleCurrent, m.bundleTotal))
		row = phase + " " + barStr + " " + counter
	case counterW > 0:
		counter := fmt.Sprintf("%9s", fmt.Sprintf("%d/%d", m.bundleCurrent, m.bundleTotal))
		row = barStr + " " + counter
	default:
		row = barStr
	}
	return clipRow(row, m.width)
}

// ── bubbleEvents implements core.Events by sending to a tea.Program ─────────

type bubbleEvents struct {
	prog *tea.Program
	done <-chan struct{} // closed after prog.Run() returns; used to avoid deadlocking Println
}

var _ core.Events = (*bubbleEvents)(nil)

// safePrintln prints a line above the TUI. After the program has stopped,
// it falls back to stderr. prog.Println blocks if the event loop is gone,
// so we run it in a goroutine and check the done channel; a leaked goroutine
// is harmless because the process exits shortly after done is closed.
func (e *bubbleEvents) safePrintln(s string) {
	if interrupted.Load() != 0 {
		return
	}
	go func() {
		select {
		case <-e.done:
			fmt.Fprintln(os.Stderr, s)
		default:
			e.prog.Println(s)
		}
	}()
}

func (e *bubbleEvents) BundleStart(pages int64) {
	e.prog.Send(bundleStartMsg{total: pages})
}

func (e *bubbleEvents) FetchFailed(listed grabber.Filterable, err error) {
	e.safePrintln(color.RedString("- error fetching chapter %s: %s", listed.GetTitle(), err.Error()))
}

func (e *bubbleEvents) Skipped(c *grabber.Chapter) {
	e.safePrintln(color.YellowString(
		"- skipping chapter %s: --skip-pages excludes all of its pages", c.GetTitle()))
}

func (e *bubbleEvents) FilenameFailed(c *grabber.Chapter, err error) {
	e.safePrintln(color.RedString(
		"- error creating filename for chapter %s: %s", c.GetTitle(), err.Error()))
}

func (e *bubbleEvents) Started(c *grabber.Chapter) {
	e.prog.Send(chapterStartedMsg{chapter: c, total: int64(c.PagesCount) * 2})
}

func (e *bubbleEvents) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		e.safePrintln(color.RedString("- error downloading page %d: %s", idx+1, err.Error()))
		return
	}
	e.prog.Send(pageSuccessMsg{chapter: c})
}

func (e *bubbleEvents) DownloadFailed(c *grabber.Chapter, err error) {
	e.safePrintln(color.RedString(
		"- error downloading chapter %s: %s", c.GetTitle(), err.Error()))
	e.prog.Send(downloadFailedMsg{chapter: c})
}

func (e *bubbleEvents) Archived(c *grabber.Chapter) {
	e.prog.Send(archivedMsg{chapter: c})
}

func (e *bubbleEvents) Packed(c *grabber.Chapter, _ string, err error) {
	if err != nil {
		e.safePrintln(color.RedString(err.Error()))
	}
	e.prog.Send(packedMsg{chapter: c, failed: err != nil})
}

func (e *bubbleEvents) Downloaded(c *grabber.Chapter) {
	e.prog.Send(bundleDownloadedMsg{pages: int(c.PagesCount)})
}

func (e *bubbleEvents) BundlePacking() {
	e.prog.Send(bundlePackingMsg{})
}

func (e *bubbleEvents) BundleArchived(n int) {
	e.prog.Send(bundleArchivedMsg{n: n})
}

// Volume event stubs: volumes are not yet rendered in the TUI; they compile
// and satisfy core.Events so the rebase builds while full support is added.
func (e *bubbleEvents) VolumeStart(v *core.Volume) {}
func (e *bubbleEvents) VolumeChapter(v *core.Volume, c *grabber.Chapter) {}
func (e *bubbleEvents) VolumePacking(v *core.Volume) {}
func (e *bubbleEvents) VolumeProgress(v *core.Volume, fraction float64) {}
func (e *bubbleEvents) VolumePacked(v *core.Volume, name string, err error) {
	if errors.Is(err, core.ErrSkippedVolume) {
		return
	}
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

// Volume event stubs for plainEvents
func (plainEvents) VolumeStart(v *core.Volume) {}
func (plainEvents) VolumeChapter(v *core.Volume, c *grabber.Chapter) {}
func (plainEvents) VolumePacking(v *core.Volume) {}
func (plainEvents) VolumeProgress(v *core.Volume, fraction float64) {}
func (plainEvents) VolumePacked(v *core.Volume, name string, err error) {}

// ── Helpers ─────────────────────────────────────────────────────────────────

// chapterRawTitle builds an untruncated "Series - Chapter [lang]" title for a
// chapter. The caller passes showLanguage (true when the series has chapters in
// more than one language). The result is stored in barState.rawTitle and
// truncated to titleW at render time so resizing the terminal shows more text.
func chapterRawTitle(seriesTitle string, c *grabber.Chapter, showLanguage bool) string {
	chapTitle := c.GetTitle()
	if showLanguage && c.Language != "" {
		chapTitle += " [" + c.Language + "]"
	}
	if seriesTitle == "" {
		return chapTitle
	}
	return seriesTitle + " - " + chapTitle
}

// teaNoticeHandler returns a notice handler that routes output through the TUI.
// It respects the interrupted mute flag and falls back to stderr when the
// program has stopped (done channel closed).
func teaNoticeHandler(prog *tea.Program, done <-chan struct{}) func(notice.Level, string) {
	return func(level notice.Level, msg string) {
		if interrupted.Load() != 0 {
			return
		}
		var s string
		if level == notice.LevelWarn {
			s = color.YellowString(msg)
		} else {
			s = color.BlueString(msg)
		}
		go func() {
			select {
			case <-done:
				fmt.Fprintln(os.Stderr, s)
			default:
				prog.Println(s)
			}
		}()
	}
}

// teaAttentionHandler returns a browser attention handler that routes output
// through the TUI, with the same done-channel fallback as teaNoticeHandler.
func teaAttentionHandler(prog *tea.Program, done <-chan struct{}) func(browser.Attention) {
	return func(a browser.Attention) {
		if !a.Active || interrupted.Load() != 0 {
			return
		}
		s := attentionColor.Sprint(a.Message())
		go func() {
			select {
			case <-done:
				attentionColor.Fprintln(os.Stderr, a.Message())
			default:
				prog.Println(s)
			}
		}()
	}
}
