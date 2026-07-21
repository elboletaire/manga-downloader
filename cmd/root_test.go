// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestNoChaptersMessage(t *testing.T) {
	if got := noChaptersMessage(""); got != "No chapters found" {
		t.Errorf("unexpected message without language: %q", got)
	}
	want := `No chapters found for language "mx" (perhaps the site uses a different language code, e.g. "es-la" for Latin American Spanish)`
	if got := noChaptersMessage("mx"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestHasDuplicateChapterNumbers(t *testing.T) {
	dup := grabber.Filterables{
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
	}
	if !hasDuplicateChapterNumbers(dup) {
		t.Error("expected duplicates to be detected")
	}

	uniq := grabber.Filterables{
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
		&grabber.Chapter{Number: 3},
	}
	if hasDuplicateChapterNumbers(uniq) {
		t.Error("expected no duplicates")
	}
}
