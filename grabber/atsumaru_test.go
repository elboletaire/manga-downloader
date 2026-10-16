// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strconv"
	"testing"
)

// newTestAtsumaru builds an Atsumaru with the scanlator names already cached,
// so the group logic can be exercised without hitting the series page
func newTestAtsumaru(scanlators ...atsumaruScanlator) *Atsumaru {
	return &Atsumaru{
		Grabber:        &Grabber{URL: "https://atsu.moe/manga/exqmE", Settings: &Settings{}},
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

// when the series page can't be read the ids still work as the group names
func TestAtsumaruFetchChaptersFallsBackToIds(t *testing.T) {
	a := newTestAtsumaru()
	a.info = testInfo()

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := chapters[0].(*AtsumaruChapter).GetGroup(); got != "alpha-id" {
		t.Errorf("group = %q, want the id alpha-id", got)
	}
}

// FetchChapters lists every group's release of every chapter, with the group
// named and the chapter's ID kept, and no title tagged: the group is a field
func TestAtsumaruFetchChaptersListsEveryRelease(t *testing.T) {
	a := newTestAtsumaru(
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)
	a.info = testInfo()

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 7 {
		t.Fatalf("expected all 7 releases, got %d", len(chapters))
	}

	ones := chapters.Filter(func(f Filterable) bool { return f.GetNumber() == 1 })
	if len(ones) != 2 {
		t.Fatalf("expected chapter 1 from both groups, got %d", len(ones))
	}
	for i, want := range []struct{ id, group string }{{"alpha-id-1", "Alpha"}, {"delta-id-1", "Delta"}} {
		ac := ones[i].(*AtsumaruChapter)
		if ac.ID != want.id || ac.Id != want.id {
			t.Errorf("chapter 1 #%d ID = %q, Id = %q, want %q", i, ac.ID, ac.Id, want.id)
		}
		if ac.GetGroup() != want.group {
			t.Errorf("chapter 1 #%d group = %q, want %q", i, ac.GetGroup(), want.group)
		}
		if ac.GetTitle() != "Episode 1" {
			t.Errorf("chapter 1 #%d title = %q, want the site's own", i, ac.GetTitle())
		}
	}
}

// the listing is what --scanlator is applied to, through the generic layer
func TestAtsumaruListingFiltersByGroup(t *testing.T) {
	a := newTestAtsumaru(
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)
	a.info = testInfo()

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got, err := SelectGroup(chapters, "alpha")
	if err != nil {
		t.Fatalf("SelectGroup error = %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Alpha kept %d chapters, want its 3", len(got))
	}
	if options := GroupOptions(chapters); options == nil || !reflect.DeepEqual(options.Values, []string{"Alpha", "Delta"}) {
		t.Errorf("GroupOptions() = %+v, want Alpha then Delta", options)
	}
}

func TestAtsumaruResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://atsu.moe/read/2VgNt/vJly4Skj", "https://atsu.moe/manga/2VgNt"},
		{"https://www.atsu.moe/read/2VgNt/vJly4Skj", "https://www.atsu.moe/manga/2VgNt"},
		{"https://atsu.moe/read/2VgNt/vJly4Skj?page=3", "https://atsu.moe/manga/2VgNt"},
		{"https://atsu.moe/read/2VgNt/vJly4Skj#section", "https://atsu.moe/manga/2VgNt"},
		{"https://ATSU.MOE/read/2VgNt/vJly4Skj", "https://ATSU.MOE/manga/2VgNt"},
		// series URLs unchanged
		{"https://atsu.moe/manga/2VgNt", "https://atsu.moe/manga/2VgNt"},
		// unrelated paths unchanged
		{"https://atsu.moe/browse", "https://atsu.moe/browse"},
	}

	for _, c := range cases {
		a := &Atsumaru{Grabber: &Grabber{URL: c.input}}
		a.resolveSeriesURL()
		if a.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, a.URL, c.want)
		}
	}
}

// FetchChapter keeps the group the listing named, the chapter's own ID and its
// title, so the EPUB identity and {{.Group}} come out the same either way
func TestAtsumaruChapterKeepsGroup(t *testing.T) {
	a := newTestAtsumaru(
		atsumaruScanlator{Id: "alpha-id", Name: "Alpha"},
		atsumaruScanlator{Id: "delta-id", Name: "Delta"},
	)
	a.info = testInfo()

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ones := chapters.Filter(func(f Filterable) bool { return f.GetNumber() == 1 })
	body := `{"readChapter":{"pages":[{"image":"/p1.jpg"}]}}`
	for _, want := range []struct{ id, group string }{{"alpha-id-1", "Alpha"}, {"delta-id-1", "Delta"}} {
		listed := ones[0]
		if listed.(*AtsumaruChapter).ID != want.id {
			listed = ones[1]
		}
		chapter, err := atsumaruChapter(listed.(*AtsumaruChapter), body)
		if err != nil {
			t.Fatalf("atsumaruChapter() error = %v", err)
		}
		if chapter.Group != want.group || chapter.GetGroups()[0] != want.group || chapter.ID != want.id {
			t.Errorf("chapter = group %q, ID %q, want %q, %q", chapter.Group, chapter.ID, want.group, want.id)
		}
	}
}
