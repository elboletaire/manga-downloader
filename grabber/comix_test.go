// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"reflect"
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
			2, "", false,
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
			if ch.Group != c.wantGrp {
				t.Errorf("group = %q, want %q", ch.Group, c.wantGrp)
			}
			if want := chapterURLID(ch.URL); ch.ID != want {
				t.Errorf("ID = %q, want the reader's path %q", ch.ID, want)
			}
		})
	}
}

// TestComixCachesChaptersPerURL checks that a second FetchChapters of the same
// series is served from the cache, every group's row included, whatever the
// scanlator setting: the group is applied to the listing by core
func TestComixCachesChaptersPerURL(t *testing.T) {
	resetComixPace(t)

	comix := &Comix{
		Grabber: &Grabber{
			URL:      "https://comix.to/title/test-series",
			Settings: &Settings{Scanlator: ""},
		},
		cachedURL: "https://comix.to/title/test-series",
		cachedChapters: []*ComixChapter{
			{Chapter: Chapter{Number: 1, Title: "Chapter 1", Group: "Alpha"}, URL: "http://ex.com/1"},
			{Chapter: Chapter{Number: 1, Title: "Chapter 1", Group: "Beta"}, URL: "http://ex.com/1b"},
			{Chapter: Chapter{Number: 2, Title: "Chapter 2"}, URL: "http://ex.com/2"},
		},
	}

	for _, scanlator := range []string{"", "Alpha", "all"} {
		comix.Settings.Scanlator = scanlator
		chapters, errs := comix.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("unexpected errors for %q: %v", scanlator, errs)
		}
		if len(chapters) != 3 {
			t.Errorf("%q: got %d chapters, want the 3 cached rows", scanlator, len(chapters))
		}
		for _, ch := range chapters {
			if strings.Contains(ch.GetTitle(), "[") {
				t.Errorf("%q: title %q was tagged; the group is a field, not a title", scanlator, ch.GetTitle())
			}
		}
	}
	if comix.cachedURL != "https://comix.to/title/test-series" || len(comix.cachedChapters) != 3 {
		t.Errorf("the cache changed: %q, %d rows", comix.cachedURL, len(comix.cachedChapters))
	}
}

// resetComixPace clears the shared pacing state and makes comixSleep a no-op
// until the test ends.
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

// fakeComixClock replaces comixNow and comixSleep with a manually moved clock.
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

	// a new instance must not burst again: the rate limit is per host
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

// FetchChapter keeps the group the chapter list named, with the release's ID
// and title, next to the pages it read off the reader
func TestComixChapterKeepsGroup(t *testing.T) {
	row := `<li class="mchap-item"><span class="mchap-row__ch">Chapter 4</span><a class="mchap-row__primary" href="https://comix.to/title/abc/4-release"></a><div class="mchap-row__secondary"><a class="mchap-row__group"><span>Ah, my next job</span></a></div></li>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(row))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := comixParseRow(doc.Find(".mchap-item").First(), "https://comix.to")
	if err != nil {
		t.Fatal(err)
	}

	reader := `<main class="rpage-main"><div class="rpage-page" data-page="1"><img class="rpage-page__img" referrerpolicy="no-referrer" src="https://rxn.example.site/hi/p1"></div></main>`
	chapter, err := comixChapter(listed, reader)
	if err != nil {
		t.Fatalf("comixChapter() error = %v", err)
	}
	// the group is one name, a comma and all
	if chapter.Group != "Ah, my next job" || !reflect.DeepEqual(chapter.GetGroups(), []string{"Ah, my next job"}) {
		t.Errorf("group = %q (%v), want the one name Ah, my next job", chapter.Group, chapter.GetGroups())
	}
	if want := chapterURLID(listed.URL); chapter.ID != want {
		t.Errorf("ID = %q, want %q", chapter.ID, want)
	}
	if len(chapter.Pages) != 1 {
		t.Errorf("got %d pages, want 1", len(chapter.Pages))
	}
}
