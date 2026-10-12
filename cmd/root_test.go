// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

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
	want := "Wǒ Kě'ài Dào Bào - Chapter 0001 Anuku Hilang [id]:"
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
	want := "Series - Chapter 0003 [vi]:"
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
	want := "One Piece - Capítulo 1142:"
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
	if !strings.HasSuffix(got, "[id]:") {
		t.Errorf("language tag must survive truncation, got %q", got)
	}
}
