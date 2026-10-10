// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ranges"
)

func TestNoChaptersMessage(t *testing.T) {
	if got := NoChaptersMessage("", ""); got != "No chapters found" {
		t.Errorf("unexpected message without filters: %q", got)
	}
	want := `No chapters found for language "mx" (perhaps the site uses a different language code, e.g. "es-la" for Latin American Spanish)`
	if got := NoChaptersMessage("mx", ""); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	want = `No chapters found for scanlation group "Omega"`
	if got := NoChaptersMessage("", "Omega"); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRequestsBelowListedFloor(t *testing.T) {
	// manhuaplus tales-of-demons-and-gods01: the site pruned everything below
	// chapter 280, so asking from 1 must warn...
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 529}}, 280) {
		t.Error("a range starting below the listed floor must be flagged")
	}
	// ...but a request within the listed window must not
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 300}}, 280) {
		t.Error("a range within the listed window must not be flagged")
	}
	// any of several ranges dipping below the floor is enough
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 300, End: 310}, {Begin: 100, End: 120}}, 280) {
		t.Error("a later range below the floor must be flagged")
	}
	// lists starting at the beginning have nothing missing below them, even
	// when the user asks for chapter 0 (prologue/oneshot numbering)
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 1) {
		t.Error("a floor of 1 must never be flagged")
	}
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 0, End: 10}}, 0) {
		t.Error("a floor of 0 must never be flagged")
	}
	// fractional floors above 1 still count (a list starting at e.g. 1.5
	// means chapter 1 was pruned)
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 10}}, 1.5) {
		t.Error("a fractional floor above 1 must be flagged")
	}
	// decimal chapter numbers within the listed window must not be flagged,
	// floor included (>= floor is served, only < floor is missing)
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 529.1, End: 529.1}}, 280) {
		t.Error("a single decimal chapter inside the window must not be flagged")
	}
	if RequestsBelowListedFloor([]ranges.Range{{Begin: 280, End: 280}}, 280) {
		t.Error("a range starting exactly at the floor must not be flagged")
	}
	// a continuation series (season 2 numbered 111+) is the reason Run only
	// consults this for user-supplied ranges: the implicit "download all"
	// range is synthesised as {Begin: 1, End: last}, which the predicate
	// flags even though the user asked for nothing that is missing
	if !RequestsBelowListedFloor([]ranges.Range{{Begin: 1, End: 179}}, 111) {
		t.Error("the synthesised all-chapters range is flagged, hence Run's rangeRequested guard")
	}
	// no ranges at all must not panic nor flag
	if RequestsBelowListedFloor(nil, 280) {
		t.Error("an empty range set must not be flagged")
	}
}

func TestHasDuplicateChapterNumbers(t *testing.T) {
	dup := grabber.Filterables{
		&grabber.Chapter{Number: 1, Language: "en"},
		&grabber.Chapter{Number: 1, Language: "es"},
		&grabber.Chapter{Number: 2, Language: "en"},
	}
	if !HasDuplicateChapterNumbers(dup) {
		t.Error("expected duplicates to be detected")
	}

	uniq := grabber.Filterables{
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
		&grabber.Chapter{Number: 3},
	}
	if HasDuplicateChapterNumbers(uniq) {
		t.Error("expected no duplicates")
	}

	// same number twice in the same language: one entry per scanlation group,
	// which the titles already disambiguate (see #164)
	sameLang := grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Alpha]"},
		&grabber.Chapter{Number: 1, Title: "Chapter 1 [Delta]"},
	}
	if HasDuplicateChapterNumbers(sameLang) {
		t.Error("duplicates sharing a language must not trigger the language tag")
	}
}

func TestAllChaptersRangeIncludesChapterZero(t *testing.T) {
	// a hardcoded Begin: 1 silently dropped chapter 0 and any 0.x prologue
	// from the "download all" prompt path (#176); the range must instead
	// span the sorted list's own ends
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 0},
		&grabber.Chapter{Number: 0.5},
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2},
	}

	got := AllChaptersRange(chapters)
	want := ranges.Range{Begin: 0, End: 2}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}

	filtered := chapters.FilterRanges([]ranges.Range{got})
	if len(filtered) != len(chapters) {
		t.Errorf("expected all %d chapters to survive filtering, got %d", len(chapters), len(filtered))
	}
}

func TestAllChaptersRangeNotStartingAtOne(t *testing.T) {
	// a continuation series, or one thinned by --language/--scanlator, can
	// legitimately list nothing below its own floor; the synthesized range
	// must reflect that floor rather than assuming 1
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 280},
		&grabber.Chapter{Number: 281},
		&grabber.Chapter{Number: 282.5},
	}

	got := AllChaptersRange(chapters)
	want := ranges.Range{Begin: 280, End: 282.5}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}
