// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ranges"
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
	if got := noChaptersMessage("", ""); got != "No chapters found" {
		t.Errorf("unexpected message without filters: %q", got)
	}
	want := `No chapters found for language "mx" (perhaps the site uses a different language code, e.g. "es-la" for Latin American Spanish)`
	if got := noChaptersMessage("mx", ""); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	want = `No chapters found for scanlation group "Omega"`
	if got := noChaptersMessage("", "Omega"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRequestsBelowListedFloor(t *testing.T) {
	// manhuaplus tales-of-demons-and-gods01: the site pruned everything below
	// chapter 280, so asking from 1 must warn...
	if !requestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 529}}, 280) {
		t.Error("a range starting below the listed floor must be flagged")
	}
	// ...but a request within the listed window must not
	if requestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 300}}, 280) {
		t.Error("a range within the listed window must not be flagged")
	}
	// any of several ranges dipping below the floor is enough
	if !requestsBelowListedFloor([]ranges.Range{{Begin: 300, End: 310}, {Begin: 100, End: 120}}, 280) {
		t.Error("a later range below the floor must be flagged")
	}
	// lists starting at the beginning have nothing missing below them, even
	// when the user asks for chapter 0 (prologue/oneshot numbering)
	if requestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 1) {
		t.Error("a floor of 1 must never be flagged")
	}
	if requestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 0) {
		t.Error("a floor of 0 must never be flagged")
	}
	// fractional floors above 1 still count (a list starting at e.g. 1.5
	// means chapter 1 was pruned)
	if !requestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 10}}, 1.5) {
		t.Error("a fractional floor above 1 must be flagged")
	}
	// decimal chapter numbers within the listed window must not be flagged,
	// floor included (>= floor is served, only < floor is missing)
	if requestsBelowListedFloor([]ranges.Range{{Begin: 529.1, End: 529.1}}, 280) {
		t.Error("a single decimal chapter inside the window must not be flagged")
	}
	if requestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 280}}, 280) {
		t.Error("a range starting exactly at the floor must not be flagged")
	}
	// a continuation series (season 2 numbered 111+) is the reason Run only
	// consults this for user-supplied ranges: the implicit "download all"
	// range is synthesised as {Begin: 1, End: last}, which the predicate
	// flags even though the user asked for nothing that is missing
	if !requestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 179}}, 111) {
		t.Error("the synthesised all-chapters range is flagged, hence Run's rangeRequested guard")
	}
	// no ranges at all must not panic nor flag
	if requestsBelowListedFloor(nil, 280) {
		t.Error("an empty range set must not be flagged")
	}
}

func TestHasDuplicateChapterNumbers(t *testing.T) {
	dup := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en"},
		&grabber.Chapter{Number: 1, Language: "es"},
		&grabber.Chapter{Number: 2, Language: "en"},
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

	// same number twice in the same language: one entry per scanlation group,
	// which the titles already disambiguate (see #164)
	sameLang := grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Alpha]"},
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Delta]"},
	}
	if hasDuplicateChapterNumbers(sameLang) {
		t.Error("duplicates sharing a language must not trigger the language tag")
	}
}
