// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnwrapDevalue(t *testing.T) {
	// a blob matching the shape Astro's islands actually serialize: plain
	// values and objects wrapped as [0, value], arrays as [1, [...]] of
	// wrapped elements, and "undefined" as the 1-element array [0]
	raw := `{
		"post":[0,{"postTitle":[0,"Some Title"],"likeUserId":[0]}],
		"totalChapterCount":[0,3],
		"initialChap":[1,[
			[0,{"number":[0,3],"slug":[0,"chapter-3"],"title":[0,""]}],
			[0,{"number":[0,2],"slug":[0,"chapter-2"],"title":[0,""]}],
			[0,{"number":[0,1],"slug":[0,"chapter-1"],"title":[0,"Prologue"]}]
		]]
	}`

	var generic interface{}
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	unwrapped := unwrapDevalue(generic)

	root, ok := unwrapped.(map[string]interface{})
	if !ok {
		t.Fatalf("unwrapped root is not a map: %T", unwrapped)
	}
	post, ok := root["post"].(map[string]interface{})
	if !ok {
		t.Fatalf("post is not a map: %T", root["post"])
	}
	if post["likeUserId"] != nil {
		t.Errorf("likeUserId (undefined) = %v, want nil", post["likeUserId"])
	}

	b, err := json.Marshal(unwrapped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	props := &astroSeriesProps{}
	if err := json.Unmarshal(b, props); err != nil {
		t.Fatalf("unmarshal into props: %v", err)
	}

	if props.Post.PostTitle != "Some Title" {
		t.Errorf("Post.PostTitle = %q, want %q", props.Post.PostTitle, "Some Title")
	}
	if props.TotalChapterCount != 3 {
		t.Errorf("TotalChapterCount = %d, want 3", props.TotalChapterCount)
	}
	if len(props.InitialChap) != 3 {
		t.Fatalf("len(InitialChap) = %d, want 3", len(props.InitialChap))
	}

	gotNumbers := []float64{}
	gotSlugs := []string{}
	gotTitles := []string{}
	for _, c := range props.InitialChap {
		gotNumbers = append(gotNumbers, c.Number)
		gotSlugs = append(gotSlugs, c.Slug)
		gotTitles = append(gotTitles, c.Title)
	}

	wantNumbers := []float64{3, 2, 1}
	wantSlugs := []string{"chapter-3", "chapter-2", "chapter-1"}
	wantTitles := []string{"", "", "Prologue"}

	if !reflect.DeepEqual(gotNumbers, wantNumbers) {
		t.Errorf("numbers = %v, want %v", gotNumbers, wantNumbers)
	}
	if !reflect.DeepEqual(gotSlugs, wantSlugs) {
		t.Errorf("slugs = %v, want %v", gotSlugs, wantSlugs)
	}
	if !reflect.DeepEqual(gotTitles, wantTitles) {
		t.Errorf("titles = %v, want %v", gotTitles, wantTitles)
	}
}

func TestExtractAstroIslandProps(t *testing.T) {
	body := `<html><body>` +
		`<astro-island uid="a" props="{&quot;unrelated&quot;:[0,true]}"></astro-island>` +
		`<astro-island uid="b" props="{&quot;totalChapterCount&quot;:[0,42]}"></astro-island>` +
		`</body></html>`

	raw, err := extractAstroIslandProps(body, "totalChapterCount")
	if err != nil {
		t.Fatalf("extractAstroIslandProps: %v", err)
	}

	want := `{"totalChapterCount":[0,42]}`
	if raw != want {
		t.Errorf("extractAstroIslandProps = %q, want %q", raw, want)
	}
}

func TestExtractAstroIslandPropsMissingMarker(t *testing.T) {
	body := `<astro-island props="{&quot;foo&quot;:[0,1]}"></astro-island>`

	if _, err := extractAstroIslandProps(body, "totalChapterCount"); err == nil {
		t.Error("expected an error when the marker is not found, got nil")
	}
}

// finding *an* astro-island holding the marker key doesn't prove it's the one
// holding the chapter list, so an empty list must be an error rather than a
// silently empty series
func TestAstroPlatformFetchChaptersGuards(t *testing.T) {
	cases := []struct {
		name      string
		props     string
		wantChaps int // -1 means "expect an error"
	}{
		{"empty list", `{"totalChapterCount":0}`, -1},
		{
			"truncated list",
			`{"totalChapterCount":5,"initialChap":[{"number":2,"slug":"chapter-2"},{"number":1,"slug":"chapter-1"}]}`,
			-1,
		},
		{
			"complete list",
			`{"totalChapterCount":2,"initialChap":[{"number":2,"slug":"chapter-2"},{"number":1,"slug":"chapter-1"}]}`,
			2,
		},
	}

	for _, c := range cases {
		props := &astroSeriesProps{}
		if err := json.Unmarshal([]byte(c.props), props); err != nil {
			t.Fatalf("%s: unmarshal: %v", c.name, err)
		}
		// pre-seeding the cache keeps fetchSeriesProps from hitting the network
		a := &astroPlatform{Grabber: &Grabber{URL: "https://hivetoons.org/series/x"}, series: props}

		chapters, errs := a.FetchChapters()
		if c.wantChaps == -1 {
			if len(errs) == 0 {
				t.Errorf("%s: FetchChapters() returned %d chapters and no error, want an error", c.name, len(chapters))
			}
			continue
		}
		if len(errs) > 0 {
			t.Errorf("%s: FetchChapters() errs = %v", c.name, errs)
			continue
		}
		if len(chapters) != c.wantChaps {
			t.Errorf("%s: FetchChapters() got %d chapters, want %d", c.name, len(chapters), c.wantChaps)
		}
	}
}

func TestAstroPlatformSeriesSlug(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://vortexscans.org/series/archmage-curriculum", "archmage-curriculum"},
		{"https://vortexscans.org/series/archmage-curriculum/chapter-20", "archmage-curriculum"},
		{"https://hivetoons.org/series/eleceed?foo=1", "eleceed"},
		{"https://vortexscans.org/series/a-rogue-guard-in-a-medieval-fantasy-pglwl7vt", "a-rogue-guard-in-a-medieval-fantasy-pglwl7vt"},
		{"https://vortexscans.org/", ""},
	}

	for _, c := range cases {
		a := astroPlatform{Grabber: &Grabber{URL: c.url}}
		if got := a.seriesSlug(); got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
