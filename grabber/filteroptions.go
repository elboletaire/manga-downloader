// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"sort"
	"strings"
	"unicode"
)

// FilterAll is the explicit "every value" sentinel for --language and
// --scanlator, for the sites whose filter accepts it (see FilterOptions.All)
const FilterAll = "all"

// FilterOptions lists the values a series offers for one filter. It's only
// meaningful once the chapters have been fetched.
type FilterOptions struct {
	// Values are every value the series has, whatever filter is active ("en",
	// "es-la", group names). They're what --language and --scanlator take;
	// users see them through Sorted.
	Values []string
	// All is set when the site accepts FilterAll, which keeps every value
	All bool
	// Current is the value shown when no filter is set, for sites that show one
	// edition at a time (Manga Plus: the URL's). Empty when every value is shown.
	Current string
}

// Sorted returns a copy of the options with Values in alphabetical order, the
// one order users see them in, whatever order the site lists them.
func (o FilterOptions) Sorted() FilterOptions {
	o.Values = SortedValues(o.Values)

	return o
}

// SortedValues returns a sorted copy of values: case-insensitive, with digit
// runs compared as numbers, stable for values that tie.
func SortedValues(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return naturalLess(sorted[i], sorted[j])
	})

	return sorted
}

func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		if unicode.IsDigit(rune(a[0])) && unicode.IsDigit(rune(b[0])) {
			na, ra := digitRun(a)
			nb, rb := digitRun(b)
			if na != nb {
				return len(na) < len(nb) || (len(na) == len(nb) && na < nb)
			}
			a, b = ra, rb

			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}

	return len(a) < len(b)
}

// digitRun splits the leading digits of s, without their leading zeros
func digitRun(s string) (digits, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	return strings.TrimLeft(s[:i], "0"), s[i:]
}

// LanguageLister is implemented by the sites whose chapters can be filtered by
// language
type LanguageLister interface {
	LanguageOptions() FilterOptions
}

// ScanlatorLister is for sites that list one scanlation group's releases per
// request, so the grabber reads Settings.Scanlator and filters with SelectGroup.
// Sites whose listing holds every group's releases set Chapter.Group instead.
type ScanlatorLister interface {
	ScanlatorOptions() FilterOptions
}

// Refilterable is for grabbers whose FetchChapters serves a filter change from cache,
// without a browser or request, so the app can reuse the site. Grabbers that must
// fetch to apply a filter (mangadex, mangafire, mangaplus) mustn't implement it.
type Refilterable interface {
	// Refilterable marks that FetchChapters serves cached data on a filter
	// change: no browser or network access is needed.
	Refilterable()
}

// uniqueValues drops the empty and repeated values, keeping the first
// occurrence of each in its place
func uniqueValues(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		unique = append(unique, v)
	}

	return unique
}
