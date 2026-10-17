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
		tc := &TanstackComic{Grabber: &Grabber{URL: "https://hivetoons.org/series/x"}, series: c.series}

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
		tc := TanstackComic{Grabber: &Grabber{URL: c.url}}
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
		tc := &TanstackComic{Grabber: &Grabber{URL: c.input}}
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
	g := &TanstackComic{Grabber: &Grabber{URL: "https://en-hijala.com/series/winning-shot!"}, series: series}

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

// hijalaSeriesFixture is the seroval tail of en-hijala.com's series page,
// trimmed to the series object and three of its 252 chapters. The count
// stays at the page's 252, as parseTanstackSeries does not check it.
const hijalaSeriesFixture = `$R[52]={id:11,slug:"double-click",postTitle:"Double Click",postType:"SERIES",_count:$R[57]={bookmarks:4,ratings:0,chapters:252,covers:0}},initialChapters:$R[59]=[$R[60]={id:5935,slug:"chapter-252",number:252,title:null,featuredImage:null,createdAt:"2026-07-12T21:50:29.125Z",updatedAt:"2026-07-12T21:50:29.125Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[61]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}},$R[62]={id:5934,slug:"chapter-251",number:251,title:null,featuredImage:null,createdAt:"2026-07-12T21:50:28.923Z",updatedAt:"2026-07-12T21:50:28.923Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[63]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}},$R[562]={id:466,slug:"chapter-1",number:1,title:"",featuredImage:"https://storage.en-hijala.com/upload/2025/06/27/Double-Click_episode_1_3e5175f3.webp",createdAt:"2025-06-27T16:57:56.743Z",updatedAt:"2026-04-01T18:43:11.786Z",mangaPostId:11,unlockAt:null,becameFreeAt:null,isPermanentlyLocked:!1,price:0,isShortLinkLocked:!1,isLocked:!1,likesCount:0,mangaPost:$R[563]={id:void 0,slug:"double-click",featuredImage:"https://storage.en-hijala.com/upload/2025/06/24/9672b20b-a60a-47b9-aca7-d989fcabac4c.webp",saleActive:!1,salePercentage:null,saleEndDate:null}}],totalChapterCount:252,firstChapter:$R[564]={number:1,slug:"chapter-1"}`

func TestHijalaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://en-hijala.com/series/double-click", true},
		{"https://EN-HIJALA.COM/series/double-click", true},
		{"https://En-Hijala.Com/series/double-click", true},
		{"https://www.en-hijala.com/series/double-click", true},
		{"https://othermanga.com/series/double-click", false},
	}
	for _, c := range cases {
		h := NewTanstackComic(&Grabber{URL: c.url})
		got, _ := h.Test()
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestHijalaParseSeries(t *testing.T) {
	series, err := parseTanstackSeries(hijalaSeriesFixture)
	if err != nil {
		t.Fatalf("parseTanstackSeries: %v", err)
	}
	if series.Title != "Double Click" {
		t.Errorf("Title = %q, want %q", series.Title, "Double Click")
	}
	if series.TotalChapterCount != 252 {
		t.Errorf("TotalChapterCount = %d, want 252", series.TotalChapterCount)
	}
	want := []tanstackChapter{
		{Number: 252, Slug: "chapter-252"},
		{Number: 251, Slug: "chapter-251"},
		{Number: 1, Slug: "chapter-1"},
	}
	if len(series.Chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(series.Chapters), len(want))
	}
	for i, w := range want {
		if got := series.Chapters[i]; got.Number != w.Number || got.Slug != w.Slug {
			t.Errorf("chapter %d = %+v, want number %v slug %q", i, got, w.Number, w.Slug)
		}
	}
}

func TestHijalaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// chapter URLs: strip the /chapter-N suffix
		{"https://en-hijala.com/series/double-click/chapter-248", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/my-series/chapter-1", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5", "https://en-hijala.com/series/my-series"},
		{"https://en-hijala.com/series/my-series/chapter-10.5/", "https://en-hijala.com/series/my-series"},
		// series URLs: unchanged
		{"https://en-hijala.com/series/double-click", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/", "https://en-hijala.com/series/double-click/"},
		// query and fragment on chapter URL: stripped
		{"https://en-hijala.com/series/double-click/chapter-248?x=1", "https://en-hijala.com/series/double-click"},
		{"https://en-hijala.com/series/double-click/chapter-1#page2", "https://en-hijala.com/series/double-click"},
		// www prefix, query, fragment on series URL: unchanged (no chapter suffix)
		{"https://www.en-hijala.com/series/double-click?foo=bar", "https://www.en-hijala.com/series/double-click?foo=bar"},
		// uppercase host: chapter stripped
		{"https://EN-HIJALA.COM/series/double-click/chapter-248", "https://EN-HIJALA.COM/series/double-click"},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			h := NewTanstackComic(&Grabber{URL: c.in})
			h.resolveSeriesURL()
			if h.URL != c.want {
				t.Errorf("resolveSeriesURL(%q) URL = %q, want %q", c.in, h.URL, c.want)
			}
		})
	}
}

func TestTanstackDomains(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://hivetoons.org/series/eleceed", true},
		{"https://hivetoons.org/series/eleceed/chapter-3", true},
		{"https://vortexscans.org/series/archmage-curriculum", true},
		{"https://azorafly.com/series/a-ghostly-song/chapter-20", true},
		// the domain is the host: a mention in the query doesn't match
		{"https://example.com/?ref=hivetoons.org", false},
		{"https://example.com/series/eleceed", false},
	}

	for _, c := range cases {
		got, err := NewTanstackComic(&Grabber{URL: c.url}).Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// newTanstackServer serves the fixtures on their paths from a host no grabber
// lists, and counts the requests each path gets. It returns the base URL.
func newTanstackServer(t *testing.T, routes map[string]string) (string, *requestCounter) {
	t.Helper()

	ids := &identifyServer{origin: "https://hivetoons.org", routes: routes}
	srv, counter := newCountingServer(t, ids.ServeHTTP)
	ids.base = srv.URL

	return srv.URL, counter
}

// an unlisted host with the platform's markup is identified by it, and the
// page is fetched once: the markup testers share that GET
func TestTanstackMarkupIdentified(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		fixture string
	}{
		{"series page", "/series/eleceed", "tanstack_series.html"},
		{"reader page", "/series/eleceed/chapter-1", "tanstack_chapter.html"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, counter := newTanstackServer(t, map[string]string{c.path: c.fixture})

			site, _ := NewSite(base+c.path, &Settings{})
			if _, ok := site.(*TanstackComic); !ok {
				t.Fatalf("NewSite() = %T, want *TanstackComic", site)
			}
			if got := counter.count(c.path); got != 1 {
				t.Errorf("GET %s = %d, want 1", c.path, got)
			}
		})
	}
}

// pages of other platforms, or with none, carry no TanStack markers
func TestTanstackMarkupRejects(t *testing.T) {
	for _, fixture := range []string{"madara_series.html", "themesia_series.html", "unrelated_page.html"} {
		t.Run(fixture, func(t *testing.T) {
			const path = "/series/some-manga"
			base, counter := newTanstackServer(t, map[string]string{path: fixture})

			got, err := NewTanstackComicByMarkup(&Grabber{URL: base + path}).Test()
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			if got {
				t.Error("Test() = true, want false")
			}
			if n := counter.count(path); n != 1 {
				t.Errorf("GET %s = %d, want 1", path, n)
			}
		})
	}
}

// a URL that isn't a series or chapter shape is declined without a request,
// and so is any URL on an unlisted host for the domain-only grabber
func TestTanstackNoRequest(t *testing.T) {
	paths := []string{"/", "/manga/eleceed", "/series", "/series/eleceed/extra", "/chapter/eleceed"}
	routes := map[string]string{}
	for _, p := range paths {
		routes[p] = "tanstack_series.html"
	}
	base, counter := newTanstackServer(t, routes)

	for _, p := range paths {
		got, _ := NewTanstackComicByMarkup(&Grabber{URL: base + p}).Test()
		if got {
			t.Errorf("Test(%q) = true, want false", p)
		}
		if n := counter.count(p); n != 0 {
			t.Errorf("GET %s = %d, want 0", p, n)
		}
	}

	got, _ := NewTanstackComic(&Grabber{URL: base + "/series/eleceed"}).Test()
	if got {
		t.Error("domain-only Test() on an unlisted host = true, want false")
	}
	if n := counter.count("/series/eleceed"); n != 0 {
		t.Errorf("domain-only GET = %d, want 0", n)
	}
}

// hivetoons used to be a PlainHTML selector entry, which silently truncated
// every series to the ~20 server-rendered anchors. IdentifySite must hand a
// hivetoons URL to the TanStack grabber, which means both that it's registered
// in the domain-matching block (before PlainHTML) and that the old selector
// entry is really gone, since a re-added one would shadow it. All the grabbers
// tested before it match by domain, so this needs no network.
func TestHivetoonsIsIdentified(t *testing.T) {
	g := &Grabber{URL: "https://hivetoons.org/series/eleceed", Settings: &Settings{}}

	site, errs := g.IdentifySite()
	if len(errs) > 0 {
		t.Fatalf("IdentifySite() errs = %v", errs)
	}
	if _, ok := site.(*TanstackComic); !ok {
		t.Fatalf("IdentifySite() = %T, want *TanstackComic", site)
	}
}
