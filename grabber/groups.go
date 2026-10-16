// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
)

// groupSeparator joins the groups of a release that several groups made
const groupSeparator = ", "

// groupsGetter is a listed chapter that says which scanlation groups made it
type groupsGetter interface {
	GetGroups() []string
}

// feedTeam is a group a feed names as having made a chapter (bluesolo and
// fmteam share the shape). The list can hold nulls, see teamNames
type feedTeam struct {
	Name string `json:"name"`
}

// teamNames returns the names of the teams that made a chapter, skipping the
// null entries the feed can hold
func teamNames(teams []*feedTeam) []string {
	var names []string
	for _, t := range teams {
		if t != nil {
			names = append(names, t.Name)
		}
	}

	return names
}

// cleanGroups trims the group names, drops the empty ones and the repeated
// ones, keeping the order
func cleanGroups(names []string) []string {
	trimmed := make([]string, len(names))
	for i, name := range names {
		trimmed[i] = strings.TrimSpace(name)
	}

	return uniqueValues(trimmed)
}

// joinGroups returns the display string of a release's groups: their cleaned
// names joined with groupSeparator
func joinGroups(names []string) string {
	return strings.Join(cleanGroups(names), groupSeparator)
}

// GroupOptions lists the scanlation groups of a listing, by the names
// --scanlator takes, sorted by name. It's nil when no chapter names a group,
// as there's then nothing to choose from.
func GroupOptions(chapters Filterables) *FilterOptions {
	seen := map[string]bool{}
	var values []string
	for _, c := range chapters {
		g, ok := c.(groupsGetter)
		if !ok {
			continue
		}
		for _, name := range g.GetGroups() {
			if !seen[name] {
				seen[name] = true
				values = append(values, name)
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	values = SortedValues(values)

	return &FilterOptions{Values: values, All: true}
}

// SelectGroup applies --scanlator to a listing, matched ignoring case. "", "all" and
// a listing naming no group keep everything; releases naming no group are kept only
// when none is selected. A group the listing lacks is an UnavailableFilterError.
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

// groupMatches reports whether one of the groups that made a chapter is
// selection, ignoring case
func groupMatches(c Filterable, selection string) bool {
	g, ok := c.(groupsGetter)
	if !ok {
		return false
	}
	for _, name := range g.GetGroups() {
		if strings.EqualFold(name, selection) {
			return true
		}
	}

	return false
}
