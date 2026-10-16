// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

// steps is a page's total on a chapter's bar: its download and archive steps
const steps = core.DownloadWeight + core.ArchiveWeight

func TestTruncateStringCountsRunesNotBytes(t *testing.T) {
	// 16 runes but 22 bytes: must not be truncated at a 20 rune limit
	input := "Wǒ Kě'ài Dào Bào"
	if got := truncateString(input, 20); got != input {
		t.Errorf("expected %q untouched, got %q", input, got)
	}
}

func TestTruncateStringNeverSplitsRunes(t *testing.T) {
	// no spaces, so it hard-cuts at maxLength: must cut between runes
	input := "Wǒǒǒǒǒǒǒǒǒǒ"
	got := truncateString(input, 7)
	if !utf8.ValidString(got) {
		t.Errorf("truncation produced invalid UTF-8: %q", got)
	}
	if !strings.HasPrefix(got, "Wǒǒǒǒǒǒ") {
		t.Errorf("expected a clean 7-rune cut, got %q", got)
	}
}

func TestProgressModelBubbleTea(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:      "Chapter 0042 The Great Battle",
		PagesCount: 10,
	}

	// chapterStartedMsg should add a bar to the model.
	m := newProgressModel("My Hero Academia", "", false, false, false, true)
	m.width = 80

	next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: int64(chapter.PagesCount) * steps})
	m = next.(progressModel)

	if len(m.states) != 1 {
		t.Fatalf("expected 1 bar after chapterStartedMsg, got %d", len(m.states))
	}
	s := m.states[chapter]
	if s.status != "downloading" {
		t.Errorf("initial status = %q, want \"downloading\"", s.status)
	}
	if s.rawTitle == "" {
		t.Error("rawTitle should not be empty after chapterStartedMsg")
	}

	// pageSuccessMsg should add the page's download steps to current.
	next, _ = m.Update(pageSuccessMsg{chapter: chapter})
	m = next.(progressModel)
	s = m.states[chapter]
	if s.current != core.DownloadWeight {
		t.Errorf("current after 1 page = %d, want %d", s.current, core.DownloadWeight)
	}

	// WindowSizeMsg should update width (title truncates at render time, not here).
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(progressModel)
	if m.width != 120 {
		t.Errorf("width after resize = %d, want 120", m.width)
	}

	// downloadDoneMsg should set done=true and return Quit.
	next, cmd := m.Update(downloadDoneMsg{})
	if cmd == nil {
		t.Error("downloadDoneMsg returned nil cmd, want tea.Quit")
	}
	if !next.(progressModel).done {
		t.Error("progressModel.done should be true after downloadDoneMsg")
	}

	// Ctrl+C should return Interrupt.
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Error("ctrl+c returned nil cmd, want tea.Interrupt")
	}
}

func TestProgressModelBundleMode(t *testing.T) {
	m := newProgressModel("Series", "1-5", true, false, false, true)
	m.width = 80

	// BundleStart should create the bundle bar.
	next, _ := m.Update(bundleStartMsg{total: 50}) // 50 pages → total=250
	m = next.(progressModel)
	if m.bundleBar == nil {
		t.Fatal("bundleBar is nil after bundleStartMsg")
	}
	if m.bundleTotal != 50*steps {
		t.Errorf("bundleTotal = %d, want %d", m.bundleTotal, 50*steps)
	}
}

// A chapter's bar stays downloading until its first archived page, at 80% of
// its steps, and ends at 100% as the pages are archived.
func TestChapterBarSwitchesToArchivingAtDownloadShare(t *testing.T) {
	const pages = 10
	chapter := &grabber.Chapter{Title: "Chapter 0001", PagesCount: pages}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: pages * steps})
	m = next.(progressModel)

	for range pages {
		next, _ := m.Update(pageSuccessMsg{chapter: chapter})
		m = next.(progressModel)
	}
	if s := m.states[chapter]; s.status != "downloading" || s.current != s.total*core.DownloadWeight/steps {
		t.Errorf("after all downloads: status %q, current %d; want downloading at 80%% of %d", s.status, s.current, s.total)
	}

	next, _ = m.Update(archivedMsg{chapter: chapter})
	m = next.(progressModel)
	if s := m.states[chapter]; s.status != "archiving" {
		t.Errorf("status after the first archived page = %q, want archiving", s.status)
	}
	for range pages - 1 {
		next, _ := m.Update(archivedMsg{chapter: chapter})
		m = next.(progressModel)
	}
	if s := m.states[chapter]; s.current != s.total {
		t.Errorf("current after the archive = %d, want %d", s.current, s.total)
	}
}

// The counter shows pages, not steps: 30/30 downloading at the switch, then
// the archived pages once the archive starts.
func TestChapterCounterShowsPages(t *testing.T) {
	const pages = 30
	chapter := &grabber.Chapter{Title: "Chapter 0001", PagesCount: pages}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: pages * steps})
	m = next.(progressModel)
	row := func() string { return ansi.Strip(m.renderChapterRow(m.states[chapter])) }

	for range 7 {
		next, _ := m.Update(pageSuccessMsg{chapter: chapter})
		m = next.(progressModel)
	}
	if r := row(); !strings.Contains(r, " 7/30 ") {
		t.Errorf("row after 7 downloads %q, want 7/30", r)
	}
	for range pages - 7 {
		next, _ := m.Update(pageSuccessMsg{chapter: chapter})
		m = next.(progressModel)
	}
	if r := row(); !strings.Contains(r, " 30/30 ") || !strings.Contains(r, "downloading") {
		t.Errorf("row at the switch %q, want 30/30 downloading", r)
	}

	next, _ = m.Update(archivedMsg{chapter: chapter})
	m = next.(progressModel)
	if r := row(); !strings.Contains(r, " 1/30 ") || !strings.Contains(r, "archiving") {
		t.Errorf("row after one archived page %q, want 1/30 archiving", r)
	}
	next, _ = m.Update(archivedMsg{chapter: chapter})
	m = next.(progressModel)
	if r := row(); !strings.Contains(r, " 2/30 ") {
		t.Errorf("row after two archived pages %q, want 2/30", r)
	}
}

// A volume's counter is "finished/N ch.": its chapters no longer pending, each
// counted once, whether downloaded, failed to download or failed to fetch.
func TestVolumeCounterCountsFinishedChapters(t *testing.T) {
	listed := grabber.Filterables{
		&grabber.Chapter{Title: "Chapter 0001"},
		&grabber.Chapter{Title: "Chapter 0002"},
		&grabber.Chapter{Title: "Chapter 0003"},
	}
	v := &core.Volume{Number: 1, Chapters: listed}
	first := &grabber.Chapter{Title: "Chapter 0001", PagesCount: 1}
	second := &grabber.Chapter{Title: "Chapter 0002", PagesCount: 1}

	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	send := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}
	counter := func(want string) {
		t.Helper()
		if r := ansi.Strip(m.renderVolumeRow(m.volStates[v])); !strings.Contains(r, want) {
			t.Errorf("volume row %q, want counter %q", r, want)
		}
	}

	send(volumeStartMsg{volume: v, chCount: len(listed)})
	counter("0/3 ch.")
	send(volumeChapterMsg{volume: v, chapter: first})
	send(volumeChapterMsg{volume: v, chapter: second})

	send(chapterEndedMsg{chapter: first})
	counter("1/3 ch.")
	send(chapterEndedMsg{chapter: first}) // reported twice: still one
	counter("1/3 ch.")

	send(downloadFailedMsg{chapter: second})
	counter("2/3 ch.")

	// the third chapter's fetch failed, so it never got VolumeChapter
	send(chapterEndedMsg{chapter: listed[2]})
	counter("3/3 ch.")

	send(volumePackingMsg{volume: v})
	send(volumePackedMsg{volume: v})
	counter("3/3 ch.")
	if vs := m.volStates[v]; vs.status != "done" {
		t.Errorf("volume status = %q, want \"done\"", vs.status)
	}
}

// While core totals the pages, before BundleStart, the bundle row says so and
// draws an empty bar with the real bars' glyph, not a "0/0" over shading.
func TestBundleRowBeforeBundleStart(t *testing.T) {
	m := newProgressModel("Series", "1-5", true, false, false, true)
	m.width = 80

	row := ansi.Strip(m.renderBundleRow())
	if !strings.HasPrefix(row, "Series - chapters 1-5") || !strings.Contains(row, "gathering") {
		t.Errorf("row %q doesn't name the bundle or say it's gathering", row)
	}
	if strings.Contains(row, "░") || strings.Contains(row, "0/0") {
		t.Errorf("row %q has the placeholder bar or a 0/0 counter", row)
	}
	if !strings.Contains(row, "▇") {
		t.Errorf("row %q doesn't draw an empty bar", row)
	}
}

// The bundle row ends like a chapter row, saying done (or failed) in its own
// status column, instead of a "saved file" line printed below the bars.
func TestBundleRowEndsWithStatus(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, "done"},
		{fmt.Errorf("disk full"), "failed"},
	} {
		m := newProgressModel("Series", "", true, false, false, true)
		m.width = 80
		for _, msg := range []tea.Msg{bundleStartMsg{total: 5}, bundlePackingMsg{}, downloadDoneMsg{err: tc.err}} {
			next, _ := m.Update(msg)
			m = next.(progressModel)
		}
		row := ansi.Strip(m.renderBundleRow())
		if !strings.HasPrefix(row, "Series - all chapters") || !strings.HasSuffix(strings.TrimSpace(row), tc.want) {
			t.Errorf("err %v: row %q, want it titled and ending in %q", tc.err, row, tc.want)
		}
	}
}

// core drops a chapter that fails to download and still packs the bundle, so
// it ends done but short of its total: the bar shows how far it got, not full.
func TestBundleRowDoneShortOfTotal(t *testing.T) {
	m := newProgressModel("Series", "", true, false, false, true)
	m.width = 80
	msgs := []tea.Msg{bundleStartMsg{total: 10}} // 10 pages → total=50
	for range 5 {
		msgs = append(msgs, pageSuccessMsg{})
	}
	msgs = append(msgs, bundlePackingMsg{}, bundleArchivedMsg{n: 5}, downloadDoneMsg{})
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}
	row := m.renderBundleRow()
	if !strings.Contains(row, m.bundleBar.ViewAs(0.5)) {
		t.Errorf("row %q doesn't draw the bar at 25/50 steps", row)
	}
	if strings.Contains(row, m.bundleBar.ViewAs(1.0)) {
		t.Errorf("row %q draws a full bar for a bundle short of its total", row)
	}
	if plain := ansi.Strip(row); !strings.Contains(plain, " 5/10 ") || !strings.HasSuffix(strings.TrimSpace(plain), "done") {
		t.Errorf("row %q, want 5/10 archived and done", plain)
	}
}

// A failed bundle keeps its danger bar on the model, so a resize reaches it,
// instead of making a new one every frame.
func TestBundleFailBarPersists(t *testing.T) {
	m := newProgressModel("Series", "", true, false, false, true)
	m.width = 80
	for _, msg := range []tea.Msg{bundleStartMsg{total: 10}, pageSuccessMsg{}, downloadDoneMsg{err: fmt.Errorf("disk full")}} { // 1 of 10 pages → 4/50
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}
	if m.bundleFailBar == nil {
		t.Fatal("bundleFailBar is nil after a failed download")
	}
	failBar := m.bundleFailBar
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(progressModel)
	_, barW, _ := btLayout(120)
	if m.bundleFailBar != failBar || failBar.Width() != barW {
		t.Errorf("fail bar not resized in place: width %d, want %d", failBar.Width(), barW)
	}
	if row := m.renderBundleRow(); !strings.Contains(row, failBar.ViewAs(0.08)) {
		t.Errorf("row %q doesn't draw the fail bar at 4/50", row)
	}
}

// BundleStart(0), when every chapter failed to fetch, still shows its 0/0:
// only the gathering row leaves the counter blank.
func TestBundleCounterAfterEmptyStart(t *testing.T) {
	m := newProgressModel("Series", "", true, false, false, true)
	m.width = 80
	next, _ := m.Update(bundleStartMsg{total: 0})
	m = next.(progressModel)
	if row := ansi.Strip(m.renderBundleRow()); !strings.Contains(row, "0/0") {
		t.Errorf("row %q has no 0/0 counter after BundleStart(0)", row)
	}
}

// A bundle run ends its counter exactly at the total: each page counts once as
// it downloads and once as it's archived. Downloaded used to add the chapter's
// pages a third time, ending at 150/100.
func TestProgressModelBundleCountsEachPageOnce(t *testing.T) {
	chapter := &grabber.Chapter{Title: "Chapter 0001", PagesCount: 50}

	m := newProgressModel("Series", "1", true, false, false, true)
	m.width = 80
	msgs := []tea.Msg{bundleStartMsg{total: 50}, chapterStartedMsg{chapter: chapter, total: 100}}
	for range 50 {
		msgs = append(msgs, pageSuccessMsg{chapter: chapter})
	}
	// what Downloaded sends for a bundled chapter: it adds no pages
	msgs = append(msgs, chapterEndedMsg{chapter: chapter}, bundlePackingMsg{})
	for range 50 {
		msgs = append(msgs, bundleArchivedMsg{n: 1})
	}
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}
	if m.bundleCurrent != m.bundleTotal {
		t.Errorf("bundle ended at %d/%d", m.bundleCurrent, m.bundleTotal)
	}
}

// TestBubbleEventsBundleCountsEachPageOnce runs the same bundle through
// bubbleEvents into a real program, so a Downloaded that sends page events fails it.
func TestBubbleEventsBundleCountsEachPageOnce(t *testing.T) {
	chapter := &grabber.Chapter{Title: "Chapter 0001", PagesCount: 50}

	model := newProgressModel("Series", "1", true, false, false, true)
	prog := tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	done := make(chan struct{})
	ev := &bubbleEvents{prog: prog, done: done}

	go func() {
		ev.BundleStart(50)
		ev.Started(chapter)
		for i := range 50 {
			ev.Page(chapter, i, nil)
		}
		ev.Downloaded(chapter)
		ev.BundlePacking()
		for range 50 {
			ev.BundleArchived(1)
		}
		// messages are handled in order, so the model is final once quit is handled
		prog.Send(downloadDoneMsg{})
	}()
	final, err := prog.Run()
	close(done)
	if err != nil {
		t.Fatalf("program failed: %v", err)
	}

	m := final.(progressModel)
	if m.bundleDownloaded != 50 || m.bundleArchived != 50 {
		t.Errorf("counted %d downloaded and %d archived, want 50 each", m.bundleDownloaded, m.bundleArchived)
	}
	if m.bundleCurrent != m.bundleTotal {
		t.Errorf("bundle ended at %d/%d", m.bundleCurrent, m.bundleTotal)
	}
}

func TestFailedBarNotFull(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:      "Chapter 0001",
		PagesCount: 20, // total progress steps = 40
	}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80

	{
		next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: 20 * steps})
		m = next.(progressModel)
	}
	// simulate 10 of 20 pages downloaded: 40 of 100 steps
	for i := 0; i < 10; i++ {
		{
			next, _ := m.Update(pageSuccessMsg{chapter: chapter})
			m = next.(progressModel)
		}
	}
	// mark as failed via packedMsg
	{
		next, _ := m.Update(packedMsg{chapter: chapter, failed: true})
		m = next.(progressModel)
	}

	s := m.states[chapter]
	if s.status != "failed" {
		t.Fatalf("status = %q, want \"failed\"", s.status)
	}

	// The failBar is created lazily in renderChapterRow; after rendering it exists.
	row := m.renderChapterRow(s)
	if s.failBar == nil {
		t.Error("failBar should be non-nil after rendering a failed chapter")
	}

	// The failed bar renders at actual progress (40/100 = 40%), not full (100%).
	// Compare the rendered width against ViewAs(0.4) and ViewAs(1.0).
	failView40 := s.failBar.ViewAs(0.4)
	failView100 := s.failBar.ViewAs(1.0)
	if !strings.Contains(row, failView40) {
		t.Errorf("failed bar row does not contain 40%% bar\nrow:      %q\nexpected: %q", row, failView40)
	}
	if strings.Contains(row, failView100) && failView40 != failView100 {
		t.Errorf("failed bar row contains 100%% bar (should be at 25%%)\nrow: %q", row)
	}

	// A done chapter should use ViewAs(1.0) via the brand bar, not the failBar.
	chapter2 := &grabber.Chapter{Title: "Chapter 0002", PagesCount: 10}
	{
		next, _ := m.Update(chapterStartedMsg{chapter: chapter2, total: 10 * steps})
		m = next.(progressModel)
	}
	{
		next, _ := m.Update(packedMsg{chapter: chapter2, failed: false})
		m = next.(progressModel)
	}
	s2 := m.states[chapter2]
	if s2.status != "done" {
		t.Fatalf("status = %q, want \"done\"", s2.status)
	}
	if s2.failBar != nil {
		t.Error("done chapter should not have a failBar")
	}
}

func TestBtLayout(t *testing.T) {
	tests := []struct {
		width      int
		wantNarrow bool
	}{
		{80, false},
		{120, false},
		{200, false},
		{50, false},
		// btFixedW=24, btMinBarW=15: full layout requires width >= 24+15+1=40
		{40, false},
		{39, true},
		{20, true},
		{10, true},
	}
	for _, tc := range tests {
		titleW, barW, narrow := btLayout(tc.width)
		if narrow != tc.wantNarrow {
			t.Errorf("btLayout(%d): narrow=%v, want %v", tc.width, narrow, tc.wantNarrow)
		}
		if titleW < 1 {
			t.Errorf("btLayout(%d): titleW=%d < 1", tc.width, titleW)
		}
		if barW < 1 {
			t.Errorf("btLayout(%d): barW=%d < 1", tc.width, barW)
		}
		var total int
		if narrow {
			// compact row: title + " " + bar
			total = titleW + 1 + barW
		} else {
			// full row: title + " " + bar + " " + counter(9) + " " + status(12)
			total = titleW + 1 + barW + 1 + btCounterW + 1 + btStatusW
		}
		if total != tc.width {
			t.Errorf("btLayout(%d): total row width %d ≠ %d (titleW=%d barW=%d narrow=%v)",
				tc.width, total, tc.width, titleW, barW, narrow)
		}
	}
}

// TestBtLayoutTitleShare pins the split on wide terminals: the title takes two
// thirds of the room, between btMinTitleW and btMaxTitleW, and the bar gets the rest.
func TestBtLayoutTitleShare(t *testing.T) {
	tests := []struct {
		width, wantTitleW, wantBarW int
	}{
		{80, btMinTitleW, 80 - btFixedW - btMinTitleW},
		{120, 64, 32},
		{200, btMaxTitleW, 200 - btFixedW - btMaxTitleW},
	}
	for _, tc := range tests {
		titleW, barW, narrow := btLayout(tc.width)
		if narrow || titleW != tc.wantTitleW || barW != tc.wantBarW {
			t.Errorf("btLayout(%d) = (%d, %d, narrow=%v), want (%d, %d, false)",
				tc.width, titleW, barW, narrow, tc.wantTitleW, tc.wantBarW)
		}
	}
}

func TestFitTitle(t *testing.T) {
	const title = "One Piece - Chapter 1135 - Shining Gold"
	tests := []struct {
		name, title, tags string
		w                 int
		want              string
	}{
		// the whole row is too long: the title gives way, the tags stay whole
		{"tags kept", title, " [TCB Scans]", 40, "One Piece - Chapter 1135 - … [TCB Scans]"},
		// a title that fits is padded so the tags sit against the right edge
		{"short title", "Chapter 5", " [TCB Scans]", 40, "Chapter 5" + strings.Repeat(" ", 19) + " [TCB Scans]"},
		// wide runes count two cells
		{"cjk", "第一話の題名", " [en]", 19, "第一話の題名   [en]"},
		{"cjk truncated", "第一話の題名第一話", " [en]", 12, "第一話… [en]"},
		// no tags: identical to fixedW
		{"no tags", title, "", 40, fixedW(title, 40)},
		// the tags alone don't fit: they are truncated
		{"tags too long", title, " [en] [Very Long Group Name]", 10, " [en] [Ve…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fitTitle(tc.title, tc.tags, tc.w)
			if got != tc.want {
				t.Errorf("fitTitle() = %q, want %q", got, tc.want)
			}
			if w := runewidth.StringWidth(got); w != tc.w {
				t.Errorf("fitTitle() width = %d, want %d", w, tc.w)
			}
		})
	}
}

// TestViewWidths renders chapter and bundle rows at various terminal widths and
// asserts that every row's visible width (ANSI-stripped) does not exceed the
// terminal width.
func TestViewWidths(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:      "Chapter 0042 The Battle of Wits",
		PagesCount: 20,
	}
	cjkChapter := &grabber.Chapter{
		Title:      "第042话 大决战",
		PagesCount: 20,
	}
	longTitle := "A Very Long Series Title That Exceeds The Maximum Width"

	widths := []int{120, 80, 56, 50, 40, 20, 10}

	for _, w := range widths {
		t.Run("chapter", func(t *testing.T) {
			m := newProgressModel("My Hero Academia", "", false, false, false, true)
			m.width = w
			{
				next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: 40})
				m = next.(progressModel)
			}
			s := m.states[chapter]
			row := m.renderChapterRow(s)
			checkRowWidth(t, row, w, "chapter")
		})

		t.Run("chapter_cjk", func(t *testing.T) {
			m := newProgressModel(longTitle, "", false, false, false, true)
			m.width = w
			{
				next, _ := m.Update(chapterStartedMsg{chapter: cjkChapter, total: 40})
				m = next.(progressModel)
			}
			s := m.states[cjkChapter]
			row := m.renderChapterRow(s)
			checkRowWidth(t, row, w, "chapter_cjk")
		})

		t.Run("chapter_done", func(t *testing.T) {
			m := newProgressModel("Series", "", false, false, false, true)
			m.width = w
			{
				next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: 40})
				m = next.(progressModel)
			}
			{
				next, _ := m.Update(packedMsg{chapter: chapter, failed: false})
				m = next.(progressModel)
			}
			s := m.states[chapter]
			row := m.renderChapterRow(s)
			checkRowWidth(t, row, w, "chapter_done")
		})

		t.Run("chapter_failed", func(t *testing.T) {
			m := newProgressModel("Series", "", false, false, false, true)
			m.width = w
			{
				next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: 40})
				m = next.(progressModel)
			}
			for i := 0; i < 10; i++ {
				{
					next, _ := m.Update(pageSuccessMsg{chapter: chapter})
					m = next.(progressModel)
				}
			}
			{
				next, _ := m.Update(packedMsg{chapter: chapter, failed: true})
				m = next.(progressModel)
			}
			s := m.states[chapter]
			row := m.renderChapterRow(s)
			checkRowWidth(t, row, w, "chapter_failed")
		})

		t.Run("bundle", func(t *testing.T) {
			m := newProgressModel("Series", "1-5", true, false, false, true)
			m.width = w
			{
				next, _ := m.Update(bundleStartMsg{total: 50})
				m = next.(progressModel)
			}
			row := m.renderBundleRow()
			checkRowWidth(t, row, w, "bundle")
		})

		t.Run("bundle_gathering", func(t *testing.T) {
			m := newProgressModel("Series", "1-5", true, false, false, true)
			m.width = w
			checkRowWidth(t, m.renderBundleRow(), w, "bundle_gathering")
		})

		t.Run("bundle_failed", func(t *testing.T) {
			m := newProgressModel("Series", "1-5", true, false, false, true)
			m.width = w
			for _, msg := range []tea.Msg{bundleStartMsg{total: 50}, pageSuccessMsg{}, downloadDoneMsg{err: fmt.Errorf("disk full")}} {
				next, _ := m.Update(msg)
				m = next.(progressModel)
			}
			checkRowWidth(t, m.renderBundleRow(), w, "bundle_failed")
		})

		// "12345/12345" outgrows the 9-cell counter; the status must survive
		t.Run("bundle_large", func(t *testing.T) {
			m := newProgressModel(longTitle, "1-500", true, false, false, true)
			m.width = w
			next, _ := m.Update(bundleStartMsg{total: 12345})
			m = next.(progressModel)
			m.bundleDownloaded = 12345
			row := m.renderBundleRow()
			checkRowWidth(t, row, w, "bundle_large")
			if _, _, narrow := btLayout(w); !narrow {
				plain := ansi.Strip(row)
				if !strings.Contains(plain, "12345/12345") || !strings.HasSuffix(strings.TrimSpace(plain), "downloading") {
					t.Errorf("bundle_large at width=%d: row %q lost its counter or status", w, plain)
				}
			}
		})
	}
}

// exitedProgram returns a program that has run and exited, on which Send is a
// no-op: the events can be called without a live TUI.
func exitedProgram() *tea.Program {
	prog := tea.NewProgram(newProgressModel("", "", false, false, false, true),
		tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	go prog.Send(downloadDoneMsg{})
	prog.Run()
	return prog
}

// TestPrintlnNoBlockAfterQuit verifies that bubbleEvents.println does not
// block after the Bubble Tea program has quit (i.e., after done is closed).
// It is primarily a hang-detector: if println blocks, the test times out.
func TestPrintlnNoBlockAfterQuit(t *testing.T) {
	prog := tea.NewProgram(progressModel{
		width:         80,
		states:        make(map[grabber.Filterable]*barState),
		volStates:     make(map[*core.Volume]*volumeBarState),
		chapterVolume: make(map[*grabber.Chapter]*core.Volume),
	})
	done := make(chan struct{})
	ev := &bubbleEvents{prog: prog, done: done}

	const N = 20
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < N; i++ {
			ev.println(fmt.Sprintf("line %02d", i))
		}
		prog.Send(downloadDoneMsg{})
	}()
	prog.Run()
	close(done)
	wg.Wait()

	// After done is closed, println must not block regardless of program state.
	ev.println("after quit")
}

// checkRowWidth strips ANSI codes and checks the visible width.
func checkRowWidth(t *testing.T, row string, maxWidth int, label string) {
	t.Helper()
	visible := ansi.StringWidth(row)
	if visible > maxWidth {
		t.Errorf("%s at width=%d: visible width %d > %d (row=%q)",
			label, maxWidth, visible, maxWidth, row)
	}
}

// TestReleaseLabelInMessages checks that the plain and TTY messages name the
// release the way the bars do: group and language only when asked for.
func TestReleaseLabelInMessages(t *testing.T) {
	stubExit(t)
	var buf bytes.Buffer
	prevOut, prevNoColor := color.Output, color.NoColor
	color.Output, color.NoColor = &buf, true
	t.Cleanup(func() { color.Output, color.NoColor = prevOut, prevNoColor })

	chapter := &grabber.Chapter{Title: "Chapter 5", Number: 5, Language: "en", Group: "Group A"}
	errBoom := errors.New("boom")

	tests := []struct {
		name                string
		showLang, showGroup bool
		want                string
	}{
		{"group off", false, false, "Chapter 5"},
		{"group on", false, true, "Chapter 5 [Group A]"},
		{"language and group on", true, true, "Chapter 5 [en] [Group A]"},
	}
	for _, tt := range tests {
		t.Run("plain "+tt.name, func(t *testing.T) {
			buf.Reset()
			ev := plainEvents{showLang: tt.showLang, showGroup: tt.showGroup}
			ev.Started(chapter)
			ev.Page(chapter, 2, errBoom)
			ev.DownloadFailed(chapter, errBoom)
			want := "downloading " + tt.want + "...\n" +
				"- error downloading page 3 of " + tt.want + ": boom\n" +
				"- error downloading " + tt.want + ": boom\n"
			if got := buf.String(); got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
		t.Run("tty "+tt.name, func(t *testing.T) {
			// with done closed, println writes to stderr, which is captured here
			done := make(chan struct{})
			close(done)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			prevErr := os.Stderr
			os.Stderr = w
			t.Cleanup(func() { os.Stderr = prevErr })

			ev := &bubbleEvents{prog: exitedProgram(), done: done, showLang: tt.showLang, showGroup: tt.showGroup}
			ev.FetchFailed(chapter, errBoom)
			ev.Skipped(chapter)
			ev.Page(chapter, 2, errBoom)
			w.Close()
			os.Stderr = prevErr
			out, _ := io.ReadAll(r)
			want := "- error fetching " + tt.want + ": boom\n" +
				"- skipping " + tt.want + ": --skip-pages excludes all of its pages\n" +
				"- error downloading page 3 of " + tt.want + ": boom\n"
			if got := string(out); got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// the download-all prompt counts the chapters the listing marks as locked
func TestAllChaptersLabel(t *testing.T) {
	free := &grabber.Chapter{Number: 1}
	paid := &grabber.Chapter{Number: 2, Locked: true}

	if got, want := allChaptersLabel(grabber.Filterables{free}), "Do you want to download all 1 chapters"; got != want {
		t.Errorf("no locked chapters: got %q, want %q", got, want)
	}
	want := "Do you want to download all 2 chapters (1 locked, skipped)"
	if got := allChaptersLabel(grabber.Filterables{free, paid}); got != want {
		t.Errorf("one locked chapter: got %q, want %q", got, want)
	}
}

// locked chapters are dropped before downloading, with one line naming them
func TestSkipLocked(t *testing.T) {
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 133},
		&grabber.Chapter{Number: 134, Locked: true},
		&grabber.Chapter{Number: 135, Locked: true},
	}

	open, note := skipLocked(chapters)
	if len(open) != 1 || open[0].GetNumber() != 133 {
		t.Errorf("open = %v, want only chapter 133", open)
	}
	if want := "skipping 2 locked chapters: 134-135"; note != want {
		t.Errorf("note = %q, want %q", note, want)
	}

	open, note = skipLocked(chapters[:1])
	if len(open) != 1 || note != "" {
		t.Errorf("no locked chapters: open = %v, note = %q", open, note)
	}

	open, note = skipLocked(chapters[1:])
	if len(open) != 0 || note == "" {
		t.Errorf("all locked: open = %v, note = %q", open, note)
	}
}

// A chapter shows up as "fetching" while its page list is fetched, and the
// same row turns into the downloading one on Started.
func TestChapterRowFetchingThenDownloading(t *testing.T) {
	listed := &grabber.Chapter{Title: "Chapter 0001"}
	fetched := &grabber.Chapter{Title: "Chapter 0001", PagesCount: 10}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	send := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}

	send(chapterFetchingMsg{listed: listed})
	row := ansi.Strip(m.renderChapterRow(m.states[listed]))
	if !strings.Contains(row, "Series - Chapter 0001") || !strings.Contains(row, "fetching") || strings.Contains(row, "0/0") {
		t.Errorf("fetching row %q, want the title and fetching without a counter", row)
	}

	send(chapterFetchedMsg{listed: listed, chapter: fetched})
	send(chapterStartedMsg{chapter: fetched, total: 10 * steps})
	if len(m.states) != 1 || len(m.chapters) != 1 {
		t.Fatalf("%d states and %d rows, want the fetching row reused", len(m.states), len(m.chapters))
	}
	row = ansi.Strip(m.renderChapterRow(m.states[fetched]))
	if !strings.Contains(row, " 0/10 ") || !strings.Contains(row, "downloading") {
		t.Errorf("started row %q, want 0/10 downloading", row)
	}
}

// A chapter that ends before it starts leaves its fetching row with a final status.
func TestChapterRowFetchingEnds(t *testing.T) {
	listed := &grabber.Chapter{Title: "Chapter 0001"}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	send := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}

	send(chapterFetchingMsg{listed: listed})
	send(chapterEndedMsg{chapter: listed, status: "failed"})
	if s := m.states[listed]; s.status != "failed" {
		t.Errorf("status after a failed fetch = %q, want failed", s.status)
	}
}

// A cancelled run still ends the chapter whose fetch was in flight, so its row
// doesn't stay on "fetching".
func TestChapterRowLeavesFetchingWhenCancelled(t *testing.T) {
	listed := &grabber.Chapter{Title: "Chapter 0001"}
	fetched := &grabber.Chapter{Title: "Chapter 0001", PagesCount: 10}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	send := func(msg tea.Msg) {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}

	send(chapterFetchingMsg{listed: listed})
	send(chapterFetchedMsg{listed: listed, chapter: fetched})
	send(chapterStartedMsg{chapter: fetched, total: 10 * steps})
	send(downloadFailedMsg{chapter: fetched})
	if s := m.states[fetched]; s == nil || s.status == "fetching" || s.status != "failed" {
		t.Errorf("row after the cancelled download = %+v, want failed", s)
	}
}

// Bundle and volume chapters add no row of their own while fetching.
func TestFetchingAddsNoRowInBundleOrVolume(t *testing.T) {
	listed := &grabber.Chapter{Title: "Chapter 0001"}
	m := newProgressModel("Series", "1-2", true, false, false, true)
	next, _ := m.Update(chapterFetchingMsg{listed: listed})
	if m = next.(progressModel); len(m.states) != 0 {
		t.Errorf("bundle got %d rows from fetching", len(m.states))
	}

	m = newProgressModel("Series", "", false, false, false, true)
	v := &core.Volume{Number: 1, Chapters: grabber.Filterables{listed}}
	next, _ = m.Update(volumeStartMsg{volume: v, chCount: 1})
	next, _ = next.Update(chapterFetchingMsg{listed: listed})
	if m = next.(progressModel); len(m.states) != 0 {
		t.Errorf("volume chapter got %d rows from fetching", len(m.states))
	}
}

// The fetched chapter's title can differ from the listing's, and the row shows it.
func TestChapterRowTakesFetchedTitle(t *testing.T) {
	listed := &grabber.Chapter{Title: "Market"}
	fetched := &grabber.Chapter{Title: "Chapter 0001 Market", PagesCount: 10}
	m := newProgressModel("Series", "", false, false, false, true)
	m.width = 80
	for _, msg := range []tea.Msg{chapterFetchingMsg{listed: listed}, chapterFetchedMsg{listed: listed, chapter: fetched}} {
		next, _ := m.Update(msg)
		m = next.(progressModel)
	}
	if row := ansi.Strip(m.renderChapterRow(m.states[fetched])); !strings.Contains(row, "Series - Chapter 0001 Market") {
		t.Errorf("row %q, want the fetched chapter's title", row)
	}
}
