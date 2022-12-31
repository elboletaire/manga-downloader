package models

import (
	"sort"

	"github.com/elboletaire/manga-downloader/ranges"
)

type Filterable interface {
	GetNumber() float64
	GetTitle() string
}

type Filterables []Filterable

// Filter allows to filter Filterables by the given condition
func (f Filterables) Filter(cond func(Filterable) bool) Filterables {
	var filtered Filterables
	for _, chap := range f {
		if cond(chap) {
			filtered = append(filtered, chap)
		}
	}

	return filtered
}

// FilterRanges returns the specified ranges of Filterables sorted by their Number
func (f Filterables) FilterRanges(rngs ranges.Ranges) Filterables {
	var chaps Filterables
	for _, r := range rngs {
		chaps = append(chaps, f.Filter(func(c Filterable) bool {
			return c.GetNumber() >= float64(r.Begin) && c.GetNumber() <= float64(r.End)
		})...)
	}

	return chaps.SortByNumber()
}

// SortByNumber sorts Filterables by Number
func (f Filterables) SortByNumber() Filterables {
	sort.Slice(f, func(i, j int) bool {
		return f[i].GetNumber() < f[j].GetNumber()
	})

	return f
}
