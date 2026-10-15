// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

// a series page's seroval stream, trimmed to the shape the site serializes:
// the series object (with postTitle) right before initialChapters, the
// list's entries as $R[n]= definitions with unquoted keys, !0/!1 booleans and
// void 0, and the site's own totalChapterCount right after the list
const tanstackSeriesFixture = `$R[1]={id:1,slug:"series",postTitle:"Eleceed",_count:$R[2]={chapters:2}},` +
	`initialChapters:$R[106]=[` +
	`$R[107]={id:21437,slug:"chapter-2",number:2,title:"Itching, for \"Battle\": Part",isLocked:!1,mangaPost:$R[950]={id:void 0,slug:"eleceed"}},` +
	`$R[108]={id:21436,slug:"chapter-1",number:1,title:"",isLocked:!0,price:0}` +
	`],totalChapterCount:2,firstChapter:$R[951]={number:1,slug:"chapter-1"}`

func TestParseTanstackSeries(t *testing.T) {
	series, err := parseTanstackSeries(tanstackSeriesFixture)
	if err != nil {
		t.Fatalf("parseTanstackSeries: %v", err)
	}

	if series.Title != "Eleceed" {
		t.Errorf("Title = %q, want %q", series.Title, "Eleceed")
	}
	if series.TotalChapterCount != 2 {
		t.Errorf("TotalChapterCount = %d, want 2", series.TotalChapterCount)
	}
	if len(series.Chapters) != 2 {
		t.Fatalf("len(Chapters) = %d, want 2", len(series.Chapters))
	}
	if got := series.Chapters[0]; got.Slug != "chapter-2" || got.Number != 2 || got.Title != `Itching, for "Battle": Part` {
		t.Errorf("Chapters[0] = %+v", got)
	}
	if got := series.Chapters[1]; got.Slug != "chapter-1" || got.Number != 1 || got.Title != "" {
		t.Errorf("Chapters[1] = %+v", got)
	}
}

// the chapter list is only trustworthy when it's followed by the site's own
// total, so a page where it isn't must fail rather than be read blind
func TestParseTanstackSeriesRequiresTotal(t *testing.T) {
	body := `$R[1]={postTitle:"Eleceed"},initialChapters:$R[106]=[$R[107]={slug:"chapter-1",number:1}],firstChapter:$R[2]={number:1}`
	if _, err := parseTanstackSeries(body); err == nil {
		t.Error("parseTanstackSeries() without totalChapterCount: want an error")
	}
}

func TestParseTanstackSeriesMissingChapterList(t *testing.T) {
	if _, err := parseTanstackSeries(`$R[1]={postTitle:"Eleceed"}`); err == nil {
		t.Error("parseTanstackSeries() without initialChapters: want an error")
	}
}

func TestSerovalToJSON(t *testing.T) {
	lit := `[$R[1]={id:void 0,flag:!0,off:!1,gone:null,list:[1,2.5,-3],name:"a\"b:c,d"},$R[2]={slug:"x"}]`

	js, err := serovalToJSON(lit)
	if err != nil {
		t.Fatalf("serovalToJSON: %v", err)
	}

	var got []map[string]interface{}
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatalf("unmarshal %s: %v", js, err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	first := got[0]
	if first["id"] != nil || first["flag"] != true || first["off"] != false {
		t.Errorf("scalars = %v", first)
	}
	if first["name"] != `a"b:c,d` {
		t.Errorf("name = %q", first["name"])
	}
	if got[1]["slug"] != "x" {
		t.Errorf("second element = %v", got[1])
	}
}

// a bare reference points at a definition elsewhere in the graph, which this
// converter can't resolve, so it must fail loudly instead of guessing
func TestSerovalToJSONRejectsBareReference(t *testing.T) {
	if _, err := serovalToJSON(`{mangaPost:$R[950]}`); err == nil {
		t.Error("serovalToJSON with a bare $R[n] reference: want an error")
	}
}

// the chapter list being the only thing the grabber reads, an empty or
// truncated list must be an error rather than a silently short series
func TestTanstackComicFetchChaptersGuards(t *testing.T) {
	cases := []struct {
		name      string
		series    *tanstackSeries
		wantChaps int // -1 means "expect an error"
	}{
		{"empty list", &tanstackSeries{TotalChapterCount: 0}, -1},
		{
			"truncated list",
			&tanstackSeries{
				TotalChapterCount: 5,
				Chapters: []tanstackChapter{
					{Number: 2, Slug: "chapter-2"},
					{Number: 1, Slug: "chapter-1"},
				},
			},
			-1,
		},
		{
			"complete list",
			&tanstackSeries{
				TotalChapterCount: 2,
				Chapters: []tanstackChapter{
					{Number: 2, Slug: "chapter-2"},
					{Number: 1, Slug: "chapter-1"},
				},
			},
			2,
		},
	}

	for _, c := range cases {
		// pre-seeding the cache keeps fetchSeries from hitting the network
		tc := &tanstackComic{Grabber: &Grabber{URL: "https://hivetoons.org/series/x"}, series: c.series}

		chapters, errs := tc.FetchChapters()
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
		for _, ch := range chapters {
			tch := ch.(*TanstackChapter)
			if tch.ID != tch.Slug {
				t.Errorf("%s: chapter ID = %q, want its slug %q", c.name, tch.ID, tch.Slug)
			}
		}
	}
}

func TestTanstackComicSeriesSlug(t *testing.T) {
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
		tc := tanstackComic{Grabber: &Grabber{URL: c.url}}
		got, err := tc.seriesSlug()
		if c.want == "" {
			if err == nil {
				t.Errorf("seriesSlug(%q) = %q, want an error", c.url, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("seriesSlug(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestTanstackComicResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://vortexscans.org/series/archmage-curriculum/chapter-46",
			wanted: "https://vortexscans.org/series/archmage-curriculum",
		},
		{
			name:   "chapter URL with query string",
			input:  "https://hivetoons.org/series/eleceed/chapter-417?page=1",
			wanted: "https://hivetoons.org/series/eleceed",
		},
		{
			name:   "series URL unchanged",
			input:  "https://vortexscans.org/series/archmage-curriculum",
			wanted: "https://vortexscans.org/series/archmage-curriculum",
		},
		{
			name:   "series URL with trailing slash unchanged",
			input:  "https://vortexscans.org/series/archmage-curriculum/",
			wanted: "https://vortexscans.org/series/archmage-curriculum/",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://vortexscans.org/",
			wanted: "https://vortexscans.org/",
		},
	}

	for _, c := range cases {
		tc := &tanstackComic{Grabber: &Grabber{URL: c.input}}
		tc.resolveSeriesURL()
		if tc.URL != c.wanted {
			t.Errorf("%s: got %q, want %q", c.name, tc.URL, c.wanted)
		}
	}
}

// a series page with free chapters, a coin-locked one, a permanently locked
// one and one only a short link locks
const tanstackLockedFixture = `$R[1]={id:1,slug:"series",postTitle:"Winning Shot"},` +
	`initialChapters:$R[2]=[` +
	`$R[3]={id:2,slug:"chapter-2",number:2,title:"2",price:0,isPermanentlyLocked:!1,isLockedByCoins:!1,isShortLinkLocked:!1,isLocked:!1},` +
	`$R[4]={id:3,slug:"chapter-1",number:1,title:"1",price:0,isPermanentlyLocked:!1,isLockedByCoins:!1,isShortLinkLocked:!1,isLocked:!1},` +
	`$R[5]={id:4,slug:"chapter-3",number:3,title:"3",price:100,isPermanentlyLocked:!0,isLockedByCoins:!0,isShortLinkLocked:!1,isLocked:!0},` +
	`$R[6]={id:5,slug:"chapter-4",number:4,title:"4",price:0,isPermanentlyLocked:!1,isLockedByCoins:!1,isShortLinkLocked:!0,isLocked:!0}` +
	`],totalChapterCount:4`

func TestTanstackListingLocks(t *testing.T) {
	series, err := parseTanstackSeries(tanstackLockedFixture)
	if err != nil {
		t.Fatalf("parseTanstackSeries: %v", err)
	}
	// the series is cached, so no request is made
	g := &tanstackComic{Grabber: &Grabber{URL: "https://en-hijala.com/series/winning-shot!"}, series: series}

	chapters, errs := g.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters: %v", errs)
	}

	want := map[string]bool{"chapter-1": false, "chapter-2": false, "chapter-3": true, "chapter-4": false}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(want))
	}
	for _, c := range chapters {
		slug := c.(*TanstackChapter).Slug
		if got := LockedOf(c); got != want[slug] {
			t.Errorf("LockedOf(%s) = %v, want %v", slug, got, want[slug])
		}
	}
}
