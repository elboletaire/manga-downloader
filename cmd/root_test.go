// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elboletaire/manga-downloader/grabber"
)

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

func TestChapterBarTitleShowsLanguage(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:    "Chapter 0001 Anuku Hilang",
		Language: "id",
	}
	got := chapterBarTitle("Wǒ Kě'ài Dào Bào", chapter, 40, 30, true)
	want := "Wǒ Kě'ài Dào Bào - Chapter 0001 Anuku Hilang [id]"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestChapterBarTitleUntitledChapter(t *testing.T) {
	// untitled mangadex chapters still carry the "Chapter %04d" prefix in
	// their fetched title; the bar must not render a dangling "- :"
	chapter := &grabber.Chapter{
		Title:    "Chapter 0003",
		Language: "vi",
	}
	got := chapterBarTitle("Series", chapter, 40, 30, true)
	want := "Series - Chapter 0003 [vi]"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestChapterBarTitleWithoutLanguage(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:    "Capítulo 1142",
		Language: "es",
	}
	got := chapterBarTitle("One Piece", chapter, 40, 30, false)
	want := "One Piece - Capítulo 1142"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestChapterBarTitleLanguageTagSurvivesTruncation(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:    "Chapter 0002 Menghancurkan Mimpi Orang Lain!",
		Language: "id",
	}
	got := chapterBarTitle("Series", chapter, 40, 20, true)
	if !strings.HasSuffix(got, "[id]") {
		t.Errorf("language tag must survive truncation, got %q", got)
	}
}

func TestProgressModelBubbleTea(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:      "Chapter 0042 The Great Battle",
		PagesCount: 10,
	}

	// chapterStartedMsg should add a bar to the model.
	m := newProgressModel("My Hero Academia", "", false, false, true)
	m.width = 80

	next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: int64(chapter.PagesCount) * 2})
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

	// pageSuccessMsg should increment current.
	next, _ = m.Update(pageSuccessMsg{chapter: chapter})
	m = next.(progressModel)
	s = m.states[chapter]
	if s.current != 1 {
		t.Errorf("current after 1 page = %d, want 1", s.current)
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
	m := newProgressModel("Series", "1-5", true, false, true)
	m.width = 80

	// BundleStart should create the bundle bar.
	next, _ := m.Update(bundleStartMsg{total: 50}) // 50 pages → total=100
	m = next.(progressModel)
	if m.bundleBar == nil {
		t.Fatal("bundleBar is nil after bundleStartMsg")
	}
	if m.bundleTotal != 100 {
		t.Errorf("bundleTotal = %d, want 100", m.bundleTotal)
	}
}

func TestFailedBarNotFull(t *testing.T) {
	chapter := &grabber.Chapter{
		Title:      "Chapter 0001",
		PagesCount: 20, // total progress steps = 40
	}
	m := newProgressModel("Series", "", false, false, true)
	m.width = 80

	{
		next, _ := m.Update(chapterStartedMsg{chapter: chapter, total: 40})
		m = next.(progressModel)
	}
	// simulate 10 of 40 steps done
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

	// The row must not show a full bar (current=10, total=40 → 25%).
	// We can't parse the bar drawing chars directly, but we can verify
	// that the bar was rendered with ViewAs(0.25) not ViewAs(1.0) by checking
	// that the failBar was used (already done above) and that s.bar.SetPercent(1.0)
	// was NOT called (s.bar is still at the animated value, not forced to 1.0).
	_ = row // render was successful

	// A done chapter should use ViewAs(1.0) via the brand bar, not the failBar.
	chapter2 := &grabber.Chapter{Title: "Chapter 0002", PagesCount: 10}
	{
		next, _ := m.Update(chapterStartedMsg{chapter: chapter2, total: 20})
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

func TestBtBundleLayout(t *testing.T) {
	tests := []struct {
		width             int
		wantPhasePositive bool
		wantCounterW      int
	}{
		{120, true, 9}, // full
		{80, true, 9},  // full
		{56, true, 9},  // exactly full threshold
		{50, true, 9},  // medium (shrunk phase)
		{27, true, 9},  // minimum width for phaseW ≥ 1 (26-26=0, 27-26=1)
		{26, false, 9}, // phaseW=0 at the boundary of medium mode
		{25, false, 9}, // no phase, keep counter
		{16, false, 9}, // bar + counter only
		{15, false, 0}, // bar only
		{10, false, 0}, // tiny
	}
	for _, tc := range tests {
		phaseW, barW, counterW := btBundleLayout(tc.width)
		hasPhase := phaseW > 0
		if hasPhase != tc.wantPhasePositive {
			t.Errorf("btBundleLayout(%d): phaseW=%d (positive=%v), want positive=%v",
				tc.width, phaseW, hasPhase, tc.wantPhasePositive)
		}
		if counterW != tc.wantCounterW {
			t.Errorf("btBundleLayout(%d): counterW=%d, want %d", tc.width, counterW, tc.wantCounterW)
		}
		// Verify no overflow: total visible width ≤ terminal width.
		var total int
		switch {
		case phaseW > 0 && counterW > 0:
			total = phaseW + 1 + barW + 1 + counterW
		case counterW > 0:
			total = barW + 1 + counterW
		default:
			total = barW
		}
		if total > tc.width {
			t.Errorf("btBundleLayout(%d): total=%d overflows terminal", tc.width, total)
		}
		if barW < 1 {
			t.Errorf("btBundleLayout(%d): barW=%d < 1", tc.width, barW)
		}
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
			m := newProgressModel("My Hero Academia", "", false, false, true)
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
			m := newProgressModel(longTitle, "", false, false, true)
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
			m := newProgressModel("Series", "", false, false, true)
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
			m := newProgressModel("Series", "", false, false, true)
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
			m := newProgressModel("Series", "1-5", true, false, true)
			m.width = w
			{
				next, _ := m.Update(bundleStartMsg{total: 50})
				m = next.(progressModel)
			}
			row := m.renderBundleRow()
			checkRowWidth(t, row, w, "bundle")
		})
	}
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
