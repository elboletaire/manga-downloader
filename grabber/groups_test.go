// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"reflect"
	"testing"
)

// groupedChapter is a listed release of a chapter by a group
func groupedChapter(number float64, group string) *Chapter {
	return &Chapter{Number: number, Group: group}
}

// groupedListing lists chapter 1 by both groups and chapter 2 by Delta only,
// plus a chapter naming no group at all
func groupedListing() Filterables {
	return Filterables{
		groupedChapter(1, "Alpha"),
		groupedChapter(1, "Delta"),
		groupedChapter(2, "Delta"),
		groupedChapter(3, ""),
	}
}

func TestGroupOptionsListsGroupsByName(t *testing.T) {
	got := GroupOptions(groupedListing())
	want := &FilterOptions{Values: []string{"Alpha", "Delta"}, All: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GroupOptions() = %+v, want %+v", got, want)
	}
}

func TestGroupOptionsIsNilWithoutGroups(t *testing.T) {
	if got := GroupOptions(Filterables{groupedChapter(1, ""), groupedChapter(2, "")}); got != nil {
		t.Errorf("GroupOptions() = %+v, want nil when no chapter names a group", got)
	}
}

func TestGroupOptionsSplitsJoinedGroups(t *testing.T) {
	listing := Filterables{groupedChapter(1, "Alpha, Delta"), groupedChapter(2, "Delta")}
	got := GroupOptions(listing)
	if got == nil || !reflect.DeepEqual(got.Values, []string{"Alpha", "Delta"}) {
		t.Errorf("GroupOptions() = %+v, want Alpha then Delta", got)
	}
}

// the default, "all" and an empty selection, keep every release, the ones
// naming no group included
func TestSelectGroupKeepsEverythingByDefault(t *testing.T) {
	for _, selection := range []string{"", "  ", "all", "ALL", " all "} {
		got, err := SelectGroup(groupedListing(), selection)
		if err != nil {
			t.Fatalf("SelectGroup(%q) error = %v", selection, err)
		}
		if len(got) != 4 {
			t.Errorf("SelectGroup(%q) kept %d chapters, want all 4", selection, len(got))
		}
	}
}

func TestSelectGroupByNameIgnoringCase(t *testing.T) {
	for _, selection := range []string{"Alpha", "alpha", " ALPHA "} {
		got, err := SelectGroup(groupedListing(), selection)
		if err != nil {
			t.Fatalf("SelectGroup(%q) error = %v", selection, err)
		}
		if len(got) != 1 || got[0].(*Chapter).Group != "Alpha" || got[0].GetNumber() != 1 {
			t.Errorf("SelectGroup(%q) = %d chapters, want the one Alpha release of 1", selection, len(got))
		}
	}
}

// a chapter naming several groups is one of each of them
func TestSelectGroupMatchesAJoinedGroup(t *testing.T) {
	listing := Filterables{groupedChapter(1, "Alpha, Delta"), groupedChapter(2, "Alpha")}
	got, err := SelectGroup(listing, "delta")
	if err != nil {
		t.Fatalf("SelectGroup error = %v", err)
	}
	if len(got) != 1 || got[0].GetNumber() != 1 {
		t.Errorf("SelectGroup(delta) = %d chapters, want the joined release of 1", len(got))
	}
}

// chapters that name no group are kept only when no group is selected
func TestSelectGroupDropsUngroupedChapters(t *testing.T) {
	got, err := SelectGroup(groupedListing(), "Delta")
	if err != nil {
		t.Fatalf("SelectGroup error = %v", err)
	}
	for _, c := range got {
		if c.(*Chapter).Group == "" {
			t.Errorf("an ungrouped chapter (%g) was kept for a selected group", c.GetNumber())
		}
	}
	if len(got) != 2 {
		t.Errorf("SelectGroup(Delta) kept %d chapters, want 2", len(got))
	}
}

func TestSelectGroupUnknownListsTheAvailableGroups(t *testing.T) {
	_, err := SelectGroup(groupedListing(), "Omega")

	var unavailable *UnavailableFilterError
	if !errors.As(err, &unavailable) {
		t.Fatalf("SelectGroup(Omega) error = %v, want an UnavailableFilterError", err)
	}
	if unavailable.Filter != "scanlator" || unavailable.Value != "Omega" {
		t.Errorf("error = %+v, want the scanlator Omega", unavailable)
	}
	if !reflect.DeepEqual(unavailable.Available, []string{"Alpha", "Delta"}) {
		t.Errorf("Available = %v, want the listed groups", unavailable.Available)
	}
}

// a listing that names no group has nothing to select by, so the flag is
// ignored there: a saved group must not stop a site without groups from loading
func TestSelectGroupIgnoresAListingWithoutGroups(t *testing.T) {
	listing := Filterables{groupedChapter(1, ""), groupedChapter(2, "")}
	got, err := SelectGroup(listing, "Omega")
	if err != nil {
		t.Fatalf("SelectGroup error = %v, want the flag ignored", err)
	}
	if len(got) != 2 {
		t.Errorf("SelectGroup kept %d chapters, want all 2", len(got))
	}
}

// a group whose name holds ", " is one group, both as an option and to select
func TestGroupsKeepASeparatorInOneName(t *testing.T) {
	big := &Chapter{Number: 1}
	big.SetGroups("Ah, my next job is to exterminate monsters project", "Delta")
	listing := Filterables{big, groupedChapter(2, "Delta")}

	got := GroupOptions(listing)
	if got == nil || !reflect.DeepEqual(got.Values, []string{"Ah, my next job is to exterminate monsters project", "Delta"}) {
		t.Fatalf("GroupOptions() = %+v, want the whole name next to Delta", got)
	}
	selected, err := SelectGroup(listing, "ah, my next job is to exterminate monsters project")
	if err != nil {
		t.Fatalf("SelectGroup error = %v", err)
	}
	if len(selected) != 1 || selected[0].GetNumber() != 1 {
		t.Errorf("SelectGroup = %d chapters, want the release of 1", len(selected))
	}
}

// SetGroups trims the names and drops the blank and repeated ones, and Group
// is their join; a Chapter that only has Group gets its groups split from it
func TestChapterGroups(t *testing.T) {
	c := &Chapter{}
	c.SetGroups(" Alpha ", "", "Alpha", "Delta")
	if c.Group != "Alpha, Delta" || !reflect.DeepEqual(c.GetGroups(), []string{"Alpha", "Delta"}) {
		t.Errorf("SetGroups: Group %q, GetGroups %v", c.Group, c.GetGroups())
	}

	c.SetGroups()
	if c.Group != "" || c.GetGroups() != nil {
		t.Errorf("SetGroups() without names: Group %q, GetGroups %v, want none", c.Group, c.GetGroups())
	}

	split := &Chapter{Group: "Alpha, Delta"}
	if !reflect.DeepEqual(split.GetGroups(), []string{"Alpha", "Delta"}) {
		t.Errorf("GetGroups() without Groups = %v, want the split Group", split.GetGroups())
	}
}
