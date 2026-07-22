// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestVortexscansSeriesSlug(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://vortexscans.org/series/archmage-curriculum", "archmage-curriculum"},
		{"https://vortexscans.org/series/archmage-curriculum/chapter-20", "archmage-curriculum"},
		{"https://vortexscans.org/series/a-rogue-guard-in-a-medieval-fantasy-pglwl7vt", "a-rogue-guard-in-a-medieval-fantasy-pglwl7vt"},
		{"https://vortexscans.org/", ""},
	}

	for _, c := range cases {
		v := Vortexscans{Grabber: &Grabber{URL: c.url}}
		if got := v.seriesSlug(); got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestVsUnwrap(t *testing.T) {
	// mimics vortexscans' devalue-like hydration encoding: every value is
	// wrapped as [0, value] (plain value/object) or [1, [...]] (array of
	// wrapped elements); undefined values are encoded as [0] (1-element)
	raw := `{
		"postTitle": [0, "Archmage Curriculum"],
		"likeUserId": [0],
		"initialChap": [1, [
			[0, {"number": [0, 2], "slug": [0, "chapter-2"], "title": [0, ""]}],
			[0, {"number": [0, 1], "slug": [0, "chapter-1"], "title": [0, "Prologue"]}]
		]]
	}`

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := vsUnwrap(root["postTitle"]); got != "Archmage Curriculum" {
		t.Errorf("postTitle = %v, want %q", got, "Archmage Curriculum")
	}

	if got := vsUnwrap(root["likeUserId"]); got != nil {
		t.Errorf("likeUserId (undefined) = %v, want nil", got)
	}

	chapters, ok := vsUnwrap(root["initialChap"]).([]interface{})
	if !ok {
		t.Fatalf("initialChap did not unwrap to a slice: %T", vsUnwrap(root["initialChap"]))
	}
	if len(chapters) != 2 {
		t.Fatalf("initialChap len = %d, want 2", len(chapters))
	}

	first, ok := chapters[0].(map[string]interface{})
	if !ok {
		t.Fatalf("chapters[0] is not a map: %T", chapters[0])
	}
	want := map[string]interface{}{"number": float64(2), "slug": "chapter-2", "title": ""}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("chapters[0] = %#v, want %#v", first, want)
	}

	second, ok := chapters[1].(map[string]interface{})
	if !ok {
		t.Fatalf("chapters[1] is not a map: %T", chapters[1])
	}
	if second["slug"] != "chapter-1" || second["title"] != "Prologue" {
		t.Errorf("chapters[1] = %#v, want slug=chapter-1 title=Prologue", second)
	}
}
