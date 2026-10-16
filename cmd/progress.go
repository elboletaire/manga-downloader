// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
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

// chapterFetchingMsg adds a chapter whose page list is being fetched, by its
// listing entry; chapterFetchedMsg re-keys it to the fetched chapter
type chapterFetchingMsg struct{ listed grabber.Filterable }
type chapterFetchedMsg struct {
	listed  grabber.Filterable
	chapter *grabber.Chapter
}
type pageSuccessMsg struct{ chapter *grabber.Chapter }
type downloadFailedMsg struct{ chapter *grabber.Chapter }

// chapterEndedMsg reports a chapter that is no longer pending: downloaded,
// skipped by --skip-pages, or failed to fetch (then it's the listing entry)
type chapterEndedMsg struct {
	chapter grabber.Filterable
	// status is the row's final status when it ends without a download:
	// "skipped" or "failed", empty when it downloaded
	status string
}

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
	chCount int // len(v.Chapters), shown in the counter column as "x/N ch."
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
	tags     string // release tags (" [en] [Group]"), kept whole while the row fits
	current  int64
	total    int64
	status   string          // "fetching" | "downloading" | "archiving" | "done" | "failed" | "skipped"
	bar      *progress.Model // brand-coloured bar (violet gradient while active, green when done)
	failBar  *progress.Model // danger-coloured bar, created lazily on failure
}

// ── Per-volume bar state ────────────────────────────────────────────────────

type volumeBarState struct {
	rawTitle string                      // "Series - Vol. 01", untruncated
	chCount  int                         // number of chapters, the N in the counter's "x/N ch."
	finished map[grabber.Filterable]bool // chapters no longer pending, the x; keyed once per chapter
	fraction float64                     // 0→1, set from VolumeProgress (never decreasing)
	status   string                      // "downloading" | "archiving" | "done" | "failed" | "skipped"
	bar      *progress.Model             // brand-coloured bar
	failBar  *progress.Model             // danger-coloured bar, created lazily on failure
}

// ── Layout constants ────────────────────────────────────────────────────────

const (
	// Chapter bar row (full): title(titleW) + " " + bar(barW) + " " + counter(9) + " " + status(12)
	btCounterW  = 9                                // "9999/9999"
	btStatusW   = 12                               // " downloading"
	btGapsW     = 3                                // spaces between title, bar, counter, status
	btFixedW    = btCounterW + btStatusW + btGapsW // 24
	btMinBarW   = 15
	btMinTitleW = 40 // so an 80-column row keeps a readable title
	btMaxTitleW = 70
)

// btLayout splits a chapter row between title and bar. The counter and status
// are dropped when narrow is set.
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
	titleW = min(btMaxTitleW, max(btMinTitleW, avail*2/3), avail-btMinBarW)
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

// fitTitle renders title and tags in exactly w visible cells. The tags stay whole and
// pinned to the right edge, so releases of one chapter differ even when the row is
// narrow; only the title is padded or truncated. Tags wider than w are truncated.
func fitTitle(title, tags string, w int) string {
	if w <= 0 || tags == "" {
		return fixedW(title, w)
	}
	tagsW := runewidth.StringWidth(tags)
	if tagsW >= w {
		return fixedW(tags, w)
	}
	return fixedW(title, w-tagsW) + tags
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
	// chapters and states are keyed by the listing entry while the page list
	// is fetched, then by the fetched chapter
	chapters []grabber.Filterable
	states   map[grabber.Filterable]*barState

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
		states:        make(map[grabber.Filterable]*barState),
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

// setPercent animates bar towards p through a copy: the frame tick SetPercent
// schedules reads the bar's tag later, from another goroutine.
func setPercent(bar *progress.Model, p float64) tea.Cmd {
	b := *bar
	cmd := b.SetPercent(p)
	*bar = b
	return cmd
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

	case chapterFetchingMsg:
		// bundle and volume rows already say they're waiting
		if m.bundle || m.volumeOf(msg.listed) != nil {
			return m, nil
		}
		_, barW, _ := btLayout(m.width)
		m.chapters = append(m.chapters, msg.listed)
		rawTitle, tags := chapterTitle(m.seriesTitle, msg.listed, m.showLang, m.showGroup)
		m.states[msg.listed] = &barState{rawTitle: rawTitle, tags: tags, status: "fetching", bar: m.makeBar(barW)}
		return m, nil

	case chapterFetchedMsg:
		if s := m.states[msg.listed]; s != nil {
			delete(m.states, msg.listed)
			m.states[msg.chapter] = s
			if i := slices.Index(m.chapters, msg.listed); i >= 0 {
				m.chapters[i] = msg.chapter
			}
			// the fetched chapter's title can differ from the listing's
			s.rawTitle, s.tags = chapterTitle(m.seriesTitle, msg.chapter, m.showLang, m.showGroup)
		}
		return m, nil

	case chapterStartedMsg:
		if m.bundle {
			return m, nil
		}
		// Skip chapters that belong to a volume; volumes have their own bars.
		if _, inVolume := m.chapterVolume[msg.chapter]; inVolume {
			return m, nil
		}
		if s := m.states[msg.chapter]; s != nil {
			// the row was added while fetching: it only gets its total and status
			s.total = msg.total
			s.status = "downloading"
			return m, nil
		}
		_, barW, _ := btLayout(m.width)
		bar := m.makeBar(barW)
		m.chapters = append(m.chapters, msg.chapter)
		// Compute the raw (untruncated) title here so it adapts when the terminal
		// is resized; renderChapterRow truncates it to titleW at render time.
		rawTitle, tags := chapterTitle(m.seriesTitle, msg.chapter, m.showLang, m.showGroup)
		m.states[msg.chapter] = &barState{
			rawTitle: rawTitle,
			tags:     tags,
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
				return m, setPercent(m.bundleBar, safePercent(m.bundleCurrent, m.bundleTotal))
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
		return m, setPercent(s.bar, safePercent(s.current, s.total))

	case chapterEndedMsg:
		m.countVolumeChapter(msg.chapter)
		if s := m.states[msg.chapter]; s != nil && msg.status != "" {
			s.status = msg.status
		}

	case downloadFailedMsg:
		m.countVolumeChapter(msg.chapter)
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
		return m, setPercent(s.bar, safePercent(s.current, s.total))

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
		return m, setPercent(s.bar, 1.0)

	case bundlePackingMsg:
		m.bundleStatus = "archiving"
		m.bundlePacking = true

	case bundleArchivedMsg:
		m.bundleCurrent += int64(msg.n) * core.ArchiveWeight
		m.bundleArchived += int64(msg.n)
		if m.bundleBar != nil {
			return m, setPercent(m.bundleBar, safePercent(m.bundleCurrent, m.bundleTotal))
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
			finished: make(map[grabber.Filterable]bool),
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
		return m, setPercent(vs.bar, msg.fraction)

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
		return m, setPercent(vs.bar, 1.0)

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
			if ch, ok := c.(*grabber.Chapter); ok {
				if _, inVolume := m.chapterVolume[ch]; inVolume {
					continue
				}
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
	title := fitTitle(s.rawTitle, s.tags, titleW)

	var barStr string
	switch {
	case s.status == "fetching" || s.status == "skipped":
		barStr = s.bar.ViewAs(0)
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
		counter := strings.Repeat(" ", btCounterW)
		if s.status != "fetching" {
			counter = fmt.Sprintf("%9s", pageCounter(s.current, s.total))
		}
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
	// Counter column shows "finished/N ch." instead of "current/total".
	counter := fmt.Sprintf("%9s", fmt.Sprintf("%d/%d ch.", len(vs.finished), vs.chCount))
	row := title + " " + barStr + " " + counter + " " + m.statusStyle(vs.status)
	return clipRow(row, m.width)
}

// countVolumeChapter counts a chapter that's no longer pending towards its
// volume's counter. c is the fetched chapter, or its listing entry when the
// fetch failed; a map keyed by it counts a chapter reported twice once.
func (m progressModel) countVolumeChapter(c grabber.Filterable) {
	if vs := m.volStates[m.volumeOf(c)]; vs != nil {
		vs.finished[c] = true
	}
}

// volumeOf returns the volume c belongs to, nil outside every volume. A
// failed fetch has no VolumeChapter, so its listing entry is looked up instead.
func (m progressModel) volumeOf(c grabber.Filterable) *core.Volume {
	if ch, ok := c.(*grabber.Chapter); ok {
		if v := m.chapterVolume[ch]; v != nil {
			return v
		}
	}
	for _, v := range m.volumes {
		if slices.Contains(v.Chapters, c) {
			return v
		}
	}
	return nil
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
	// showLang and showGroup name the release in messages, as in the bars
	showLang, showGroup bool
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

func (e *bubbleEvents) Fetching(listed grabber.Filterable) {
	e.prog.Send(chapterFetchingMsg{listed: listed})
}

func (e *bubbleEvents) Fetched(listed grabber.Filterable, c *grabber.Chapter) {
	e.prog.Send(chapterFetchedMsg{listed: listed, chapter: c})
}

func (e *bubbleEvents) FetchFailed(listed grabber.Filterable, err error) {
	e.println(color.RedString("- error fetching %s: %s",
		core.ReleaseLabel(listed, e.showLang, e.showGroup), err.Error()))
	e.prog.Send(chapterEndedMsg{chapter: listed, status: "failed"})
}

func (e *bubbleEvents) Skipped(c *grabber.Chapter) {
	e.println(color.YellowString(
		"- skipping %s: --skip-pages excludes all of its pages", core.ReleaseLabel(c, e.showLang, e.showGroup)))
	e.prog.Send(chapterEndedMsg{chapter: c, status: "skipped"})
}

func (e *bubbleEvents) FilenameFailed(c *grabber.Chapter, err error) {
	e.println(color.RedString(
		"- error creating filename for %s: %s", core.ReleaseLabel(c, e.showLang, e.showGroup), err.Error()))
	e.prog.Send(chapterEndedMsg{chapter: c, status: "failed"})
}

func (e *bubbleEvents) Started(c *grabber.Chapter) {
	e.prog.Send(chapterStartedMsg{chapter: c, total: int64(c.PagesCount) * (core.DownloadWeight + core.ArchiveWeight)})
}

func (e *bubbleEvents) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		e.println(color.RedString("- error downloading page %d of %s: %s",
			idx+1, core.ReleaseLabel(c, e.showLang, e.showGroup), err.Error()))
		return
	}
	e.prog.Send(pageSuccessMsg{chapter: c})
}

func (e *bubbleEvents) DownloadFailed(c *grabber.Chapter, err error) {
	e.println(color.RedString(
		"- error downloading %s: %s", core.ReleaseLabel(c, e.showLang, e.showGroup), err.Error()))
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

// Downloaded sends no page events: the pages were counted as they came in, and
// adding them again here overshot the bundle's total
func (e *bubbleEvents) Downloaded(c *grabber.Chapter) {
	e.prog.Send(chapterEndedMsg{chapter: c})
}

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

type plainEvents struct {
	showLang, showGroup bool
}

var _ core.Events = plainEvents{}

func (plainEvents) BundleStart(int64)           {}
func (plainEvents) Downloaded(*grabber.Chapter) {}
func (plainEvents) BundlePacking()              {}
func (plainEvents) BundleArchived(int)          {}
func (plainEvents) Archived(*grabber.Chapter)   {}

func (p plainEvents) Fetching(listed grabber.Filterable) {
	fmt.Fprintf(color.Output, "fetching pages of %s...\n", core.ReleaseLabel(listed, p.showLang, p.showGroup))
}

func (plainEvents) Fetched(grabber.Filterable, *grabber.Chapter) {}

func (p plainEvents) FetchFailed(listed grabber.Filterable, err error) {
	red("- error fetching %s: %s", core.ReleaseLabel(listed, p.showLang, p.showGroup), err.Error())
}

func (p plainEvents) Skipped(c *grabber.Chapter) {
	yellow("- skipping %s: --skip-pages excludes all of its pages", core.ReleaseLabel(c, p.showLang, p.showGroup))
}

func (p plainEvents) FilenameFailed(c *grabber.Chapter, err error) {
	red("- error creating filename for %s: %s", core.ReleaseLabel(c, p.showLang, p.showGroup), err.Error())
}

func (p plainEvents) Started(c *grabber.Chapter) {
	fmt.Fprintf(color.Output, "downloading %s...\n", core.ReleaseLabel(c, p.showLang, p.showGroup))
}

func (p plainEvents) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		red("- error downloading page %d of %s: %s",
			idx+1, core.ReleaseLabel(c, p.showLang, p.showGroup), err.Error())
	}
}

func (p plainEvents) DownloadFailed(c *grabber.Chapter, err error) {
	red("- error downloading %s: %s", core.ReleaseLabel(c, p.showLang, p.showGroup), err.Error())
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

// chapterTitle builds an untruncated "Series - Chapter" title, with the release
// tags " [lang] [group]" kept apart so they can stay whole.
func chapterTitle(seriesTitle string, c grabber.Filterable, showLanguage, showGroup bool) (title, tags string) {
	title = grabber.ChapterLabel(c)
	if seriesTitle != "" {
		title = seriesTitle + " - " + title
	}
	return title, core.ReleaseTags(c, showLanguage, showGroup)
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
