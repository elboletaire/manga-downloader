// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/elboletaire/manga-downloader/grabber"
)

// TestCalculateTitleLengthsBarMinimum checks that the rendered title row
// leaves at least minBarWidth columns for the bar filler on common terminal
// widths.  The rendered label is "manga - chapter:" whose width is
// mangaLen + titleSeparatorWidth + chapterLen.
func TestCalculateTitleLengthsBarMinimum(t *testing.T) {
	for _, termWidth := range []int{50, 80, 120, 200} {
		mangaLen, chapterLen := calculateTitleLengths(termWidth)
		// Full label width including the " - " and ":" separator.
		labelWidth := mangaLen + titleSeparatorWidth + chapterLen
		remaining := termWidth - labelWidth - fixedDecorWidth
		if remaining < minBarWidth {
			t.Errorf("termWidth=%d: remaining bar space %d < minBarWidth %d (mangaLen=%d chapterLen=%d)",
				termWidth, remaining, minBarWidth, mangaLen, chapterLen)
		}
	}
}

// TestCalculateTitleLengthsWideTerm verifies that very wide terminals are
// capped so the title does not consume the whole row.
func TestCalculateTitleLengthsWideTerm(t *testing.T) {
	mangaLen, chapterLen := calculateTitleLengths(300)
	parts := mangaLen + chapterLen
	if parts > 60 {
		t.Errorf("title parts sum to %d on a 300-col terminal, want ≤60", parts)
	}
}

// TestCalculateTitleLengthsNarrowTerm verifies that very narrow terminals
// still get a non-zero title (mpb will truncate with "…" if it still doesn't fit).
func TestCalculateTitleLengthsNarrowTerm(t *testing.T) {
	mangaLen, chapterLen := calculateTitleLengths(30)
	if mangaLen < 1 || chapterLen < 1 {
		t.Errorf("narrow terminal gave zero-length title part: mangaLen=%d chapterLen=%d", mangaLen, chapterLen)
	}
}

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

	// pageSuccessMsg should increment current.
	next, _ = m.Update(pageSuccessMsg{chapter: chapter})
	m = next.(progressModel)
	s = m.states[chapter]
	if s.current != 1 {
		t.Errorf("current after 1 page = %d, want 1", s.current)
	}

	// WindowSizeMsg should update width.
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(progressModel)
	if m.width != 120 {
		t.Errorf("width after resize = %d, want 120", m.width)
	}

	// downloadDoneMsg should return Quit.
	_, cmd := m.Update(downloadDoneMsg{})
	if cmd == nil {
		t.Error("downloadDoneMsg returned nil cmd, want tea.Quit")
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
