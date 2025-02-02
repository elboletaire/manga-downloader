package grabber

import (
	"slices"
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

// FilterList returns the Filterables, without the ones with a Number matched in specified blacklist
func (f Filterables) FilterOutList(blacklist []float64) Filterables {
	chaps := f.Filter(func(c Filterable) bool {
		return ! slices.Contains(blacklist, c.GetNumber())
	})

	return chaps
}

// SortByNumber sorts Filterables by Number
func (f Filterables) SortByNumber() Filterables {
	sort.Slice(f, func(i, j int) bool {
		return f[i].GetNumber() < f[j].GetNumber()
	})

	return f
}
