// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strconv"
	"strings"
	"testing"
)

// newTestAtsumaru builds an Atsumaru with the scanlator names already cached,
// so the group logic can be exercised without hitting the series page
func newTestAtsumaru(scanlator string, scanlators ...atsumaruScanlator) *Atsumaru {
	return &Atsumaru{
		Grabber:        &Grabber{URL: "https://atsu.moe/manga/exqmE", Settings: &Settings{Scanlator: scanlator}},
		scanlators:     scanlators,
		scanlatorsDone: true,
	}
}

// testInfo mirrors the shape of the real /api/manga/info payload: every
// group's chapters mixed together, tagged only with an opaque scanId
func testInfo() *atsumaruMangaInfo {
	info := &atsumaruMangaInfo{Title: "Latna Saga"}
	add := func(scanId string, numbers ...float64) {
		for _, n := range numbers {
			num := strconv.FormatFloat(n, 'f', -1, 64)
			info.Chapters = append(info.Chapters, atsumaruInfoChapter{
				Id:     scanId + "-" + num,
				Title:  "Episode " + num,
				Number: n,
				ScanId: scanId,
			})
		}
	}
	// alpha has fewer chapters but is the group the user wants (#164)
	add("alpha-id", 1, 2, 3)
	add("delta-id", 1, 2, 3, 4)

	return info
}

func TestAtsumaruGroupsAreNamedAndCounted(t *testing.T) {
	a := newTestAtsumaru("",
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)

	groups := a.groups(testInfo())
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// site order, not chapter-count order
	if groups[0].Name != "Alpha" || groups[0].Count != 3 {
		t.Errorf("expected Alpha with 3 chapters, got %q with %d", groups[0].Name, groups[0].Count)
	}
	if groups[1].Name != "Delta" || groups[1].Count != 4 {
		t.Errorf("expected Delta with 4 chapters, got %q with %d", groups[1].Name, groups[1].Count)
	}
}

// a group listed on the series page with nothing uploaded yet must not show up
// as a selectable option
func TestAtsumaruGroupsSkipsEmptyOnes(t *testing.T) {
	a := newTestAtsumaru("",
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
		atsumaruScanlator{Id: "gamma-id", Name: "Gamma"},
	)

	for _, g := range a.groups(testInfo()) {
		if g.Name == "Gamma" {
			t.Error("a group with no chapters must not be listed")
		}
	}
}

// when the series page can't be read the ids still work as selectors, in
// first-seen order so the default pick stays deterministic
func TestAtsumaruGroupsFallBackToIds(t *testing.T) {
	a := newTestAtsumaru("")

	groups := a.groups(testInfo())
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Id != "alpha-id" || groups[0].displayName() != "alpha-id" {
		t.Errorf("expected the id as display name, got %q", groups[0].displayName())
	}
}

func TestAtsumaruSelectGroupsDefaultsToMostChapters(t *testing.T) {
	a := newTestAtsumaru("",
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)

	selected, err := a.selectGroups(a.groups(testInfo()))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(selected) != 1 || selected[0].Name != "Delta" {
		t.Errorf("expected only Delta (most chapters), got %v", groupNames(selected))
	}
}

func TestAtsumaruSelectGroupsByName(t *testing.T) {
	// case-insensitive: the issue reporter tried "alpha"
	for _, pref := range []string{"Alpha", "alpha", " ALPHA ", "alpha-id"} {
		a := newTestAtsumaru(pref,
			atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
			atsumaruScanlator{Id: "delta-id", Name: "Delta"},
		)

		selected, err := a.selectGroups(a.groups(testInfo()))
		if err != nil {
			t.Fatalf("unexpected error for %q: %s", pref, err)
		}
		if len(selected) != 1 || selected[0].Name != "Alpha" {
			t.Errorf("expected only Alpha for %q, got %v", pref, groupNames(selected))
		}
	}
}

func TestAtsumaruSelectGroupsAll(t *testing.T) {
	for _, pref := range []string{"all", "ALL"} {
		a := newTestAtsumaru(pref,
			atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
			atsumaruScanlator{Id: "delta-id", Name: "Delta"},
		)

		selected, err := a.selectGroups(a.groups(testInfo()))
		if err != nil {
			t.Fatalf("unexpected error for %q: %s", pref, err)
		}
		if len(selected) != 2 {
			t.Errorf("expected every group for %q, got %v", pref, groupNames(selected))
		}
	}
}

// an unknown group must fail loudly listing the real ones, rather than
// silently falling back to the default pick
func TestAtsumaruSelectGroupsUnknownErrors(t *testing.T) {
	a := newTestAtsumaru("Omega",
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)

	_, err := a.selectGroups(a.groups(testInfo()))
	if err == nil {
		t.Fatal("expected an error for an unknown scanlation group")
	}
	for _, want := range []string{"Omega", "Alpha", "Delta"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in the error, got %q", want, err.Error())
		}
	}
}

// with a single group there's nothing to choose, so no tag is added
func TestAtsumaruFetchChaptersTagsTitlesOnlyWhenAmbiguous(t *testing.T) {
	cases := []struct {
		scanlator string
		wantTag   bool
	}{
		{"", false},
		{"Alpha", false},
		{"all", true},
	}

	for _, c := range cases {
		a := newTestAtsumaru(c.scanlator,
			atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
			atsumaruScanlator{Id: "delta-id", Name: "Delta"},
		)
		a.info = testInfo()

		chapters, errs := a.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("unexpected errors for %q: %v", c.scanlator, errs)
		}

		tagged := 0
		for _, ch := range chapters {
			if strings.Contains(ch.GetTitle(), "[") {
				tagged++
			}
		}
		if c.wantTag && tagged != len(chapters) {
			t.Errorf("expected every title tagged for %q, got %d of %d", c.scanlator, tagged, len(chapters))
		}
		if !c.wantTag && tagged != 0 {
			t.Errorf("expected no tagged titles for %q, got %d", c.scanlator, tagged)
		}
	}
}

// --scanlator all keeps every group's chapters, duplicate numbers included:
// that's the whole point, and the packer disambiguates them downstream
func TestAtsumaruFetchChaptersAllKeepsDuplicateNumbers(t *testing.T) {
	a := newTestAtsumaru("all",
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)
	a.info = testInfo()

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 7 {
		t.Fatalf("expected all 7 chapters, got %d", len(chapters))
	}

	ones := chapters.Filter(func(f Filterable) bool { return f.GetNumber() == 1 })
	if len(ones) != 2 {
		t.Fatalf("expected chapter 1 from both groups, got %d", len(ones))
	}
	if ones[0].GetTitle() == ones[1].GetTitle() {
		t.Errorf("both chapter 1s share the title %q, they'd collide when packed", ones[0].GetTitle())
	}
}
