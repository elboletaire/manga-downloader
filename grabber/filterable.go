// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/ranges"
)

// Enumerable represents an object that can be enumerated
type Enumerable interface {
	GetNumber() float64
}

// Titleable represents an object that can be titled
type Titleable interface {
	GetTitle() string
}

// Filterable represents an filterable objects
type Filterable interface {
	Enumerable
	Titleable
}

// Volumed is a chapter that knows its volume. It isn't part of Filterable:
// most sites never say, so VolumeOf is how callers ask.
type Volumed interface {
	GetVolume() int
}

// VolumeOf returns the volume of a listed chapter, 0 when it has none
func VolumeOf(f Filterable) int {
	if v, ok := f.(Volumed); ok {
		return v.GetVolume()
	}
	return 0
}

// ChapterTitlePrefix matches the chapter word and number a title may open with
// ("Chapter 0005: ", "Capítulo 5 - ", "Vol. 3 Ch. 5 "), and the separator after them
var ChapterTitlePrefix = regexp.MustCompile(`(?i)^(?:vol(?:ume|umen)?\.?\s*\d+\s*)?(?:episode|chapter|chapitre|chap|cap[ií]tulo|capitolo|cap|ch|ep|c|b[öo]l[üu]m|глава)\.?\s*0*(\d+(?:\.\d+)?)\b(?:\s*[:.\-‐‒–—―−]+)?\s*`)

// bareNumber matches a number a title opens with and no chapter word ("186. Foo")
var bareNumber = regexp.MustCompile(`^0*(\d+(?:\.\d+)?)\b`)

// ChapterName names a chapter in messages: its label, or "Chapter N (label)"
// when the label doesn't open with its number, as sites often number their
// own seasons in titles. Chapter 0 is left to the label (it may be unread).
func ChapterName(f Filterable) string {
	title, number := ChapterLabel(f), f.GetNumber()
	if number <= 0 {
		return title
	}
	for _, re := range []*regexp.Regexp{ChapterTitlePrefix, bareNumber} {
		if m := re.FindStringSubmatch(title); m != nil {
			if n, err := strconv.ParseFloat(m[1], 64); err == nil && n == number {
				return title
			}
		}
	}
	name := NumberLabel(number)
	if title == "" {
		return name
	}
	return name + " (" + title + ")"
}

// Lockable is a chapter the listing may mark as locked. Like Volumed, it isn't
// part of Filterable: most sites never say, so LockedOf is how callers ask.
type Lockable interface {
	GetLocked() bool
}

// LockedOf reports whether the listing marks a chapter as locked
func LockedOf(f Filterable) bool {
	if l, ok := f.(Lockable); ok {
		return l.GetLocked()
	}
	return false
}

// volumeRe matches a whole volume number: a bare "3", or one behind a
// "Volume"/"Vol."/"v" label ("Volume 3", "Vol. 03", "v98")
var volumeRe = regexp.MustCompile(`(?i)^(?:vol(?:ume|umen)?\.?|v)?\s*0*(\d+)$`)

// parseVolume reads a volume number out of what a site gives ("3", "03",
// "Volume 3", "v98"). Anything else ("none", "TBD", "", "10.5") is 0, unknown:
// a volume has to be a whole number to name a file after it.
func parseVolume(s string) int {
	m := volumeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// Filterables represents a slice of Filterable
type Filterables []Filterable

// Filter allows to filter Filterables by the given condition
func (f Filterables) Filter(cond func(Filterable) bool) Filterables {
	filtered := Filterables{}
	for _, chap := range f {
		if cond(chap) {
			filtered = append(filtered, chap)
		}
	}

	return filtered
}

// FilterRanges returns the specified ranges of Filterables sorted by their Number
func (f Filterables) FilterRanges(rngs []ranges.Range) Filterables {
	chaps := Filterables{}
	for _, r := range rngs {
		chaps = append(chaps, f.Filter(func(c Filterable) bool {
			return c.GetNumber() >= float64(r.Begin) && c.GetNumber() <= float64(r.End)
		})...)
	}

	return chaps
}

// SortByNumber sorts Filterables by Number. The sort is stable so that
// chapters sharing a number (e.g. one entry per scanlation group, or per
// language) keep the order the grabber returned them in, which is what
// decides their bundle folder suffix and their filename version suffix.
func (f Filterables) SortByNumber() Filterables {
	sort.SliceStable(f, func(i, j int) bool {
		return f[i].GetNumber() < f[j].GetNumber()
	})

	return f
}

// UnavailableFilterError is the error for a filter value (a --language, say)
// the title doesn't offer. Available lists what it does, empty when the site
// can't say.
type UnavailableFilterError struct {
	Filter    string
	Value     string
	Available []string
}

func (e *UnavailableFilterError) Error() string {
	available := "none"
	if len(e.Available) > 0 {
		available = strings.Join(SortedValues(e.Available), ", ")
	}

	return fmt.Sprintf("%s %q isn't available for this title (available: %s)", e.Filter, e.Value, available)
}
