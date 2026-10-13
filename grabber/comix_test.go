// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestComixParsePaging(t *testing.T) {
	cases := []struct {
		text      string
		wantShown int
		wantTotal int
		wantOk    bool
	}{
		{"Showing 1 to 20 of 248 items", 20, 248, true},
		{"Showing 0 to 0 of 248 items", 0, 248, true},
		{"Showing 241 to 248 of 248 items", 248, 248, true},
		{"", 0, 0, false},
		{"nothing useful here", 0, 0, false},
	}

	for _, c := range cases {
		shown, total, ok := comixParsePaging(c.text)
		if ok != c.wantOk {
			t.Errorf("comixParsePaging(%q) ok = %v, want %v", c.text, ok, c.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if shown != c.wantShown || total != c.wantTotal {
			t.Errorf("comixParsePaging(%q) = (%d, %d), want (%d, %d)", c.text, shown, total, c.wantShown, c.wantTotal)
		}
	}
}

func TestComixWithPage(t *testing.T) {
	cases := []struct {
		seriesURL string
		page      int
		want      string
	}{
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", 1, "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=1"},
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", 3, "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=3"},
	}

	for _, c := range cases {
		got, err := comixWithPage(c.seriesURL, c.page)
		if err != nil {
			t.Errorf("comixWithPage(%q, %d) unexpected error: %v", c.seriesURL, c.page, err)
			continue
		}
		if got != c.want {
			t.Errorf("comixWithPage(%q, %d) = %q, want %q", c.seriesURL, c.page, got, c.want)
		}
	}
}

func TestComixTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://comix.to/title/e1wlg-the-scandal-maker-has-returned", true},
		{"https://www.comix.to/title/e1wlg-the-scandal-maker-has-returned", true},
		{"https://notcomix.to/title/foo", false},
		{"https://comix.to.evil.com/title/foo", false},
		{"https://mangak.io/some-manga", false},
	}

	for _, c := range cases {
		comix := &Comix{Grabber: &Grabber{URL: c.url}}
		got, err := comix.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestComixResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
	}{
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://comix.to/title/e1wlg-the-scandal-maker-has-returned/11475745-chapter-34",
			wanted: "https://comix.to/title/e1wlg-the-scandal-maker-has-returned",
		},
		{
			name:   "www. prefix preserved",
			input:  "https://www.comix.to/title/e1wlg-the-scandal-maker-has-returned/11475745-chapter-34",
			wanted: "https://www.comix.to/title/e1wlg-the-scandal-maker-has-returned",
		},
		{
			name:   "series URL unchanged",
			input:  "https://comix.to/title/e1wlg-the-scandal-maker-has-returned",
			wanted: "https://comix.to/title/e1wlg-the-scandal-maker-has-returned",
		},
		{
			name:   "series URL with query unchanged",
			input:  "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=2",
			wanted: "https://comix.to/title/e1wlg-the-scandal-maker-has-returned?page=2",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://comix.to/",
			wanted: "https://comix.to/",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comix := &Comix{Grabber: &Grabber{URL: c.input}}
			comix.resolveSeriesURL()
			if comix.URL != c.wanted {
				t.Errorf("got %q, want %q", comix.URL, c.wanted)
			}
		})
	}
}

func TestComixPages(t *testing.T) {
	img := func(n int) string {
		return fmt.Sprintf(`<div class="rpage-page" data-page="%d"><img class="rpage-page__img" referrerpolicy="no-referrer" src="https://rxn.example.site/hi/p%d"></div>`, n, n)
	}
	loader := `<div class="rpage-page" data-page="3"><div class="rpage-page__loader" aria-label="Page 3"><span class="rpage-page__counter mono">3 / 3</span></div></div>`
	// mounted but still downloading: the img is there, and so is the loader
	mounting := `<div class="rpage-page is-loading" data-page="3"><img class="rpage-page__img" referrerpolicy="no-referrer" src="https://rxn.example.site/hi/p3"><div class="rpage-page__loader" aria-hidden="true"><span class="rpage-page__counter mono">3 / 3</span></div></div>`

	cases := []struct {
		name    string
		html    string
		want    []string
		wantErr string
	}{
		{"every page mounted", img(1) + img(2), []string{"https://rxn.example.site/hi/p1", "https://rxn.example.site/hi/p2"}, ""},
		{"a page mounted but not downloaded", img(1) + img(2) + mounting, []string{"https://rxn.example.site/hi/p1", "https://rxn.example.site/hi/p2", "https://rxn.example.site/hi/p3"}, ""},
		{"a page never mounted", img(1) + img(2) + loader, nil, "1 of 3 pages never loaded"},
		{"an img without a src", img(1) + `<div class="rpage-page" data-page="2"><img class="rpage-page__img" src=""></div>`, nil, "1 of 2 pages never loaded"},
		{"no pages", `<main class="rpage-main"></main>`, nil, "no pages found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<main class="rpage-main">` + c.html + `</main>`))
			if err != nil {
				t.Fatal(err)
			}
			pages, err := comixPages(doc)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(pages) != len(c.want) {
				t.Fatalf("got %d pages, want %d", len(pages), len(c.want))
			}
			for i, p := range pages {
				if p.URL != c.want[i] || p.Number != int64(i+1) || !p.NoReferer {
					t.Errorf("page %d = %+v, want %s numbered %d without a referer", i, p, c.want[i], i+1)
				}
			}
		})
	}
}

func TestComixReaderScroll(t *testing.T) {
	init := comixReaderScroll.Init
	for _, want := range []string{`"reader.default.v3"`, `"reader.webtoon.v3"`, `preload:"all"`, "Object.assign({},v.state,", "try{", "catch(e){}"} {
		if !strings.Contains(init, want) {
			t.Errorf("init script lacks %s: %s", want, init)
		}
	}
	if strings.Contains(init, "`") {
		t.Errorf("init script has a backtick: %s", init)
	}
	// Done is spliced into the scroll step's JS: its quotes must nest
	done := comixReaderScroll.Done
	if !strings.HasPrefix(done, "(function(){") || !strings.HasSuffix(done, "})()") {
		t.Errorf("Done isn't a self-contained expression: %s", done)
	}
	if !strings.Contains(done, `querySelector('img.rpage-page__img[src]:not([src=""])')`) {
		t.Errorf("Done doesn't check for a mounted img: %s", done)
	}
}

func TestComixSelectScanlators(t *testing.T) {
	chapters := func(nums ...float64) []*ComixChapter {
		var result []*ComixChapter
		for _, n := range nums {
			result = append(result, &ComixChapter{
				Chapter: Chapter{Number: n},
				URL:     "http://example.com",
			})
		}
		return result
	}

	cases := []struct {
		name          string
		selection     string
		allGroups     []string
		allChapters   map[string][]*ComixChapter
		wantGroups    []string
		wantErr       bool
	}{
		{
			name:      "no filter returns all groups with ungrouped first",
			selection: "",
			allGroups: []string{"-", "Alpha", "Beta"},
			allChapters: map[string][]*ComixChapter{
				"-":     chapters(1, 2),
				"Alpha": chapters(1, 2, 3),
				"Beta":  chapters(3),
			},
			wantGroups: []string{"-", "Alpha", "Beta"},
		},
		{
			name:      "select all returns all groups in order",
			selection: "all",
			allGroups: []string{"Alpha", "Beta"},
			allChapters: map[string][]*ComixChapter{
				"Alpha": chapters(1, 2),
				"Beta":  chapters(2, 3),
			},
			wantGroups: []string{"Alpha", "Beta"},
		},
		{
			name:      "select specific group by name",
			selection: "Alpha",
			allGroups: []string{"-", "Alpha", "Beta"},
			allChapters: map[string][]*ComixChapter{
				"-":     chapters(1),
				"Alpha": chapters(1, 2),
				"Beta":  chapters(2),
			},
			wantGroups: []string{"Alpha"},
		},
		{
			name:      "select case-insensitive",
			selection: "alpha",
			allGroups: []string{"Alpha"},
			allChapters: map[string][]*ComixChapter{
				"Alpha": chapters(1),
			},
			wantGroups: []string{"Alpha"},
		},
		{
			name:      "select ungrouped with dash",
			selection: "-",
			allGroups: []string{"-", "Alpha"},
			allChapters: map[string][]*ComixChapter{
				"-":     chapters(1),
				"Alpha": chapters(1, 2),
			},
			wantGroups: []string{"-"},
		},
		{
			name:      "invalid group returns error",
			selection: "NoGroup",
			allGroups: []string{"Alpha"},
			allChapters: map[string][]*ComixChapter{
				"Alpha": chapters(1),
			},
			wantErr: true,
		},
		{
			name:      "empty chapters with no filter",
			selection: "",
			allGroups: []string{},
			allChapters: map[string][]*ComixChapter{
				"Alpha": chapters(1),
			},
			wantGroups: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := selectScanlators(c.selection, c.allGroups, c.allChapters)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(c.wantGroups) {
				t.Fatalf("got %d groups, want %d: %v", len(got), len(c.wantGroups), got)
			}
			for i, g := range got {
				if g != c.wantGroups[i] {
					t.Errorf("group %d = %q, want %q", i, g, c.wantGroups[i])
				}
			}
		})
	}
}

func TestComixScanlatorOptions(t *testing.T) {
	c := &Comix{
		Grabber: &Grabber{URL: "https://comix.to/title/test"},
		groups:  []string{"Alpha", "Beta", "Gamma"},
	}

	opts := c.ScanlatorOptions()
	if len(opts.Values) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(opts.Values))
	}
	if !opts.All {
		t.Errorf("All should be true")
	}
	for i, want := range []string{"Alpha", "Beta", "Gamma"} {
		if opts.Values[i] != want {
			t.Errorf("group %d = %q, want %q", i, opts.Values[i], want)
		}
	}
}

func TestComixSelectScanlatorsTitles(t *testing.T) {
	// When selecting multiple groups ("all"), chapter titles are tagged with
	// group names to avoid filename collisions, like atsumaru does
	chapters := func(nums ...float64) []*ComixChapter {
		var result []*ComixChapter
		for _, n := range nums {
			num := strconv.FormatFloat(n, 'f', -1, 64)
			result = append(result, &ComixChapter{
				Chapter: Chapter{
					Number: n,
					Title:  "Chapter " + num,
				},
				URL:            "http://example.com",
				ScanlatorGroup: "Alpha",
			})
		}
		return result
	}

	allChapters := map[string][]*ComixChapter{
		"Alpha": chapters(1, 2, 3),
		"Beta":  chapters(1, 2, 3, 4),
	}

	// Test single group selection: no tagging
	groups, err := selectScanlators("Alpha", []string{"Alpha", "Beta"}, allChapters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || groups[0] != "Alpha" {
		t.Errorf("expected [Alpha], got %v", groups)
	}

	// Test "all": multiple groups should trigger tagging
	groups, err = selectScanlators("all", []string{"Alpha", "Beta"}, allChapters)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// The calling code (in FetchChapters) should see tagGroups = true
	// because len(groups) > 1, and append [GroupName] to titles
}

func TestComixParseRow(t *testing.T) {
	// Real comix.to structure: li.mchap-item with group in
	// div.mchap-row__secondary > a.mchap-row__group > span
	row := func(chNum, href, scanlator string) string {
		groupHTML := ""
		if scanlator != "" {
			groupHTML = fmt.Sprintf(`<div class="mchap-row__secondary"><a class="mchap-row__group"><span>%s</span></a></div>`, scanlator)
		}
		return fmt.Sprintf(
			`<li class="mchap-item"><span class="mchap-row__ch">%s</span><a class="mchap-row__primary" href="%s"></a>%s</li>`,
			chNum, href, groupHTML,
		)
	}

	cases := []struct {
		name    string
		html    string
		wantNum float64
		wantGrp string
		wantErr bool
	}{
		{
			"chapter with group",
			row("Chapter 1", "https://example.com/ch/1", "Alpha"),
			1, "Alpha", false,
		},
		{
			"chapter without group",
			row("Chapter 2", "/relative/ch/2", ""),
			2, "-", false,
		},
		{
			"chapter with whitespace group",
			row("Chapter 3.5", "https://example.com/ch/3.5", "  Beta  "),
			3.5, "Beta", false,
		},
		{
			"no chapter number",
			row("Invalid", "https://example.com/ch/x", "Alpha"),
			0, "", true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(c.html))
			if err != nil {
				t.Fatal(err)
			}
			row := doc.Find(".mchap-item").First()

			ch, err := comixParseRow(row, "https://comix.to")
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ch.Number != c.wantNum {
				t.Errorf("number = %v, want %v", ch.Number, c.wantNum)
			}
			if ch.ScanlatorGroup != c.wantGrp {
				t.Errorf("group = %q, want %q", ch.ScanlatorGroup, c.wantGrp)
			}
		})
	}
}

func TestComixCachesChaptersPerURL(t *testing.T) {
	resetComixPace(t)
	// Verify that FetchChapters caches chapters and reuses them when
	// the same URL is queried again with a different scanlator selection

	// Create a test Comix with pre-cached chapters (no ungrouped chapters)
	comix := &Comix{
		Grabber: &Grabber{
			URL:      "https://comix.to/title/test-series",
			Settings: &Settings{Scanlator: ""},
		},
		cachedURL: "https://comix.to/title/test-series",
		cachedChapters: map[string][]*ComixChapter{
			"Alpha": {
				&ComixChapter{
					Chapter:        Chapter{Number: 1, Title: "Chapter 1"},
					URL:            "http://ex.com/1",
					ScanlatorGroup: "Alpha",
				},
			},
			"Beta": {
				&ComixChapter{
					Chapter:        Chapter{Number: 1, Title: "Chapter 1"},
					URL:            "http://ex.com/1b",
					ScanlatorGroup: "Beta",
				},
			},
		},
		cachedGroups: []string{"Alpha", "Beta"},
		groups:       []string{"Alpha", "Beta"},
	}

	// First call: no filter, should use cache and dedupe
	chapters, errs := comix.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter with no filter, got %d", len(chapters))
	}
	if chapters[0].GetTitle() != "Chapter 1" {
		t.Errorf("got title %q, want untagged", chapters[0].GetTitle())
	}

	// Change filter to "Alpha" and call again: should use cache, dedupe, no tags
	comix.Settings.Scanlator = "Alpha"
	chapters, errs = comix.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter for Alpha, got %d", len(chapters))
	}
	if chapters[0].GetTitle() != "Chapter 1" {
		t.Errorf("got title %q, want untagged", chapters[0].GetTitle())
	}

	// Change to "all" and call again: should use cache, keep duplicates, tag titles
	comix.Settings.Scanlator = "all"
	chapters, errs = comix.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters for 'all', got %d", len(chapters))
	}
	// Both should be chapter 1 with tags
	for i, ch := range chapters {
		if !strings.Contains(ch.GetTitle(), "[") {
			t.Errorf("chapter %d missing group tag in %q", i, ch.GetTitle())
		}
	}
}

// resetComixPace clears the pacing state shared by every Comix and makes
// comixSleep a no-op, putting both back when the test ends, so no test that
// reaches Comix.load can really sleep
func resetComixPace(t *testing.T) {
	t.Helper()
	oldNow, oldSleep := comixNow, comixSleep
	comixSleep = func(time.Duration) {}
	comixPace.used, comixPace.last = 0, time.Time{}
	t.Cleanup(func() {
		comixNow, comixSleep = oldNow, oldSleep
		comixPace.used, comixPace.last = 0, time.Time{}
	})
}

// fakeComixClock replaces comixNow and comixSleep with a clock that only
// moves when slept or advanced, resetting the shared pacing state
func fakeComixClock(t *testing.T) (advance func(time.Duration), slept *[]time.Duration) {
	t.Helper()
	resetComixPace(t)
	now := time.Unix(1e9, 0)
	slept = new([]time.Duration)
	comixNow = func() time.Time { return now }
	comixSleep = func(d time.Duration) {
		*slept = append(*slept, d)
		if d > 0 {
			now = now.Add(d)
		}
	}

	return func(d time.Duration) { now = now.Add(d) }, slept
}

func comixLoads(t *testing.T, c *Comix, n int) {
	t.Helper()
	for range n {
		if _, err := c.load(func() (string, error) { return "", nil }); err != nil {
			t.Fatal(err)
		}
	}
}

func TestComixLoadPacing(t *testing.T) {
	_, slept := fakeComixClock(t)

	comixLoads(t, NewComix(&Grabber{}), comixLoadBurst+2)
	// only the loads past the burst wait, a whole gap each
	if len(*slept) != 2 || (*slept)[0] != comixLoadGap || (*slept)[1] != comixLoadGap {
		t.Errorf("slept %v, want 2 waits of %s", *slept, comixLoadGap)
	}
}

func TestComixLoadPacingIsSharedByInstances(t *testing.T) {
	advance, slept := fakeComixClock(t)

	// a new instance right after another's walk (the app's reload) must not
	// burst again: the rate limit is per host
	comixLoads(t, NewComix(&Grabber{}), comixLoadBurst)
	comixLoads(t, NewComix(&Grabber{}), 1)
	if len(*slept) != 1 || (*slept)[0] != comixLoadGap {
		t.Errorf("slept %v, want the second instance's load to wait %s", *slept, comixLoadGap)
	}

	// idle time earns the burst back, one load per gap
	*slept = nil
	advance(comixLoadGap * comixLoadBurst)
	comixLoads(t, NewComix(&Grabber{}), comixLoadBurst)
	if len(*slept) != 0 {
		t.Errorf("slept %v after a long idle, want a fresh burst", *slept)
	}
	advance(comixLoadGap + comixLoadGap/2)
	comixLoads(t, NewComix(&Grabber{}), 2)
	if len(*slept) != 1 || (*slept)[0] != comixLoadGap {
		t.Errorf("slept %v after one gap idle, want one unpaced load, then a wait of %s", *slept, comixLoadGap)
	}
}

func TestComixDoesNotCacheATruncatedWalk(t *testing.T) {
	resetComixPace(t)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div class="mchap-foot__hint">Showing 0 to 1 of 2 items</div>`))
	if err != nil {
		t.Fatal(err)
	}
	// a URL that can't take a page param makes the walk fail on page 2
	c := &Comix{Grabber: &Grabber{URL: "https://comix.to/title/x\x7f", Settings: &Settings{}}, doc: doc}

	if _, errs := c.FetchChapters(); len(errs) == 0 {
		t.Fatal("expected the walk to fail on page 2")
	}
	if c.cachedChapters != nil {
		t.Error("a walk that hit an error must not be cached")
	}
}
