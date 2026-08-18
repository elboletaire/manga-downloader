// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"sort"

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
