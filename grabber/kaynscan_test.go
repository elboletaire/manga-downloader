// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnwrapDevalue(t *testing.T) {
	// a slice matching the shape Astro's islands actually serialize:
	// {"tag":[0,val]} everywhere, arrays of tagged entries, and nested objects
	raw := `{
		"title":[0,"Some Title"],
		"totalChapterCount":[0,3],
		"initialChap":[1,[
			[0,{"number":[0,3],"slug":[0,"chapter-3"],"title":[0,""]}],
			[0,{"number":[0,2],"slug":[0,"chapter-2"],"title":[0,""]}],
			[0,{"number":[0,1],"slug":[0,"chapter-1"],"title":[0,""]}]
		]]
	}`

	var generic interface{}
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	unwrapped := unwrapDevalue(generic)

	b, err := json.Marshal(unwrapped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	props := &kaynscanSeriesProps{}
	if err := json.Unmarshal(b, props); err != nil {
		t.Fatalf("unmarshal into props: %v", err)
	}

	if props.TotalChapterCount != 3 {
		t.Errorf("TotalChapterCount = %d, want 3", props.TotalChapterCount)
	}
	if len(props.InitialChap) != 3 {
		t.Fatalf("len(InitialChap) = %d, want 3", len(props.InitialChap))
	}

	gotNumbers := []float64{}
	gotSlugs := []string{}
	for _, c := range props.InitialChap {
		gotNumbers = append(gotNumbers, c.Number)
		gotSlugs = append(gotSlugs, c.Slug)
	}

	wantNumbers := []float64{3, 2, 1}
	wantSlugs := []string{"chapter-3", "chapter-2", "chapter-1"}

	if !reflect.DeepEqual(gotNumbers, wantNumbers) {
		t.Errorf("numbers = %v, want %v", gotNumbers, wantNumbers)
	}
	if !reflect.DeepEqual(gotSlugs, wantSlugs) {
		t.Errorf("slugs = %v, want %v", gotSlugs, wantSlugs)
	}
}

func TestExtractKaynscanIslandProps(t *testing.T) {
	body := `<html><body>` +
		`<astro-island uid="a" props="{&quot;unrelated&quot;:[0,true]}"></astro-island>` +
		`<astro-island uid="b" props="{&quot;totalChapterCount&quot;:[0,42]}"></astro-island>` +
		`</body></html>`

	raw, err := extractKaynscanIslandProps(body, "totalChapterCount")
	if err != nil {
		t.Fatalf("extractKaynscanIslandProps: %v", err)
	}

	want := `{"totalChapterCount":[0,42]}`
	if raw != want {
		t.Errorf("extractKaynscanIslandProps = %q, want %q", raw, want)
	}
}

func TestExtractKaynscanIslandPropsMissingMarker(t *testing.T) {
	body := `<astro-island props="{&quot;foo&quot;:[0,1]}"></astro-island>`

	if _, err := extractKaynscanIslandProps(body, "totalChapterCount"); err == nil {
		t.Error("expected an error when the marker is not found, got nil")
	}
}
