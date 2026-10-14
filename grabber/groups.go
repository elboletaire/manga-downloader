// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"sort"
	"strings"
)

// groupSeparator joins the groups of a release that several groups made
const groupSeparator = ", "

// groupGetter is a listed chapter that says which scanlation group made it
type groupGetter interface {
	GetGroup() string
}

// groupNames returns the distinct groups a release's Group names, in order
func groupNames(group string) []string {
	parts := strings.Split(group, groupSeparator)
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}

	return uniqueValues(parts)
}

// GroupOptions lists the scanlation groups of a listing, by the names
// --scanlator takes: most chapters first, then by name. It's nil when no
// chapter names a group, as there's then nothing to choose from.
func GroupOptions(chapters Filterables) *FilterOptions {
	counts := map[string]int{}
	for _, c := range chapters {
		g, ok := c.(groupGetter)
		if !ok {
			continue
		}
		for _, name := range groupNames(g.GetGroup()) {
			counts[name]++
		}
	}
	if len(counts) == 0 {
		return nil
	}

	values := make([]string, 0, len(counts))
	for name := range counts {
		values = append(values, name)
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] != counts[values[j]] {
			return counts[values[i]] > counts[values[j]]
		}
		return values[i] < values[j]
	})

	return &FilterOptions{Values: values, All: true}
}

// SelectGroup applies --scanlator to a listing of releases: the ones of the
// selected group, matched ignoring case. An empty selection, "all", and a
// listing that names no group keep every chapter (the last one because there's
// no group to select by). Chapters naming no group are kept only when no group
// is selected. A group the listing doesn't have is an UnavailableFilterError
// listing the ones it does.
//
// Sites that list every group's releases in one listing get this from core;
// the ones that list one group per request (see ScanlatorLister) call it on
// their own listing, so both behave the same.
func SelectGroup(chapters Filterables, selection string) (Filterables, error) {
	selection = strings.TrimSpace(selection)
	options := GroupOptions(chapters)
	if selection == "" || strings.EqualFold(selection, FilterAll) || options == nil {
		return chapters, nil
	}

	kept := Filterables{}
	for _, c := range chapters {
		if groupMatches(c, selection) {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		return nil, &UnavailableFilterError{Filter: "scanlator", Value: selection, Available: options.Values}
	}

	return kept, nil
}

// groupMatches reports whether a chapter's group is selection, or one of the
// groups it names, ignoring case
func groupMatches(c Filterable, selection string) bool {
	g, ok := c.(groupGetter)
	if !ok {
		return false
	}
	if strings.EqualFold(g.GetGroup(), selection) {
		return true
	}
	for _, name := range groupNames(g.GetGroup()) {
		if strings.EqualFold(name, selection) {
			return true
		}
	}

	return false
}
