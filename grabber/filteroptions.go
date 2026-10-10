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
	// Current is the value the loaded list shows when no filter is set and All
	// isn't either: sites that list one edition at a time (Manga Plus) show the
	// edition the URL points at. It's empty otherwise.
	Current string
}

// LanguageLister is implemented by the sites whose chapters can be filtered by
// language
type LanguageLister interface {
	LanguageOptions() FilterOptions
}

// ScanlatorLister is implemented by the sites hosting several scanlation
// groups' versions of the same chapters
type ScanlatorLister interface {
	ScanlatorOptions() FilterOptions
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
