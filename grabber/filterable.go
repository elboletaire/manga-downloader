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

// ordered is implemented by Filterables that carry a secondary sort key
// (Chapter.SortOrder) for breaking ties between chapters sharing a Number
type ordered interface {
	GetSortOrder() float64
}

// SortByNumber sorts Filterables by Number
func (f Filterables) SortByNumber() Filterables {
	sort.Slice(f, func(i, j int) bool {
		if f[i].GetNumber() != f[j].GetNumber() {
			return f[i].GetNumber() < f[j].GetNumber()
		}

		// sort.Slice is unstable, so same-numbered chapters (e.g. a baozimh
		// chapter split into 上/中/下 parts shares a number) would otherwise
		// come out in an arbitrary order - one that can even differ between
		// the fetch-time sort and the post-download re-sort in bundle mode.
		// Tie-break on the part order when the Filterable carries one.
		var oi, oj float64
		if x, ok := f[i].(ordered); ok {
			oi = x.GetSortOrder()
		}
		if x, ok := f[j].(ordered); ok {
			oj = x.GetSortOrder()
		}
		return oi < oj
	})

	return f
}
