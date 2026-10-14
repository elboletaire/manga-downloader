// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

// FilterAll is the explicit "every value" sentinel for --language and
// --scanlator, for the sites whose filter accepts it (see FilterOptions.All)
const FilterAll = "all"

// FilterOptions lists the values a series offers for one filter. It's only
// meaningful once the chapters have been fetched.
type FilterOptions struct {
	// Values are every value the series has, whatever filter is active ("en",
	// "es-la", group names), in the site's order. They're what --language and
	// --scanlator take.
	Values []string
	// All is set when the site accepts FilterAll, which keeps every value
	All bool
	// Current is the value shown when no filter is set, for sites that show one
	// edition at a time (Manga Plus: the URL's). Empty when every value is shown.
	Current string
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
