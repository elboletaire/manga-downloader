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
	// Current is the one value the loaded list shows when no filter is set, for
	// the sites whose default is a single value rather than every value: sites
	// that list one edition at a time (Manga Plus) show the edition the URL
	// points at. It's empty otherwise, scanlation groups included: their default
	// is every group's releases.
	Current string
}

// LanguageLister is implemented by the sites whose chapters can be filtered by
// language
type LanguageLister interface {
	LanguageOptions() FilterOptions
}

// ScanlatorLister is for the sites that can only list one scanlation group's
// releases per request, so the group is a request option the grabber reads from
// Settings.Scanlator itself. It lists the groups the series has, and its
// FetchChapters keeps the selected one (through SelectGroup, so the match and
// the unknown-group error are the ones every other site gives).
//
// Sites whose one listing already holds every group's releases don't implement
// it: they set Chapter.Group, and core filters the listing by it (SelectGroup,
// see GroupOptions).
type ScanlatorLister interface {
	ScanlatorOptions() FilterOptions
}

// Refilterable is implemented by grabbers whose FetchChapters serves cached data
// on a filter change, without a browser or network access, so the controller
// reuses such a site on a filter-only reload. For the generic group filter that
// means the listing already holds every group's releases, and core does the
// rest. Grabbers that must fetch to apply their filter (mangadex, mangafire,
// mangaplus) must not implement this.
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
