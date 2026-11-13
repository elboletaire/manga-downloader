// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

func TestMangafireHid(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://mangafire.to/title/dkw-one-piece", "dkw", false},
		{"https://mangafire.to/title/dkw-one-piece/chapter/9054304", "dkw", false},
		{"https://mangafire.to/manga/one-piecee.dkw", "dkw", false}, // legacy format
		{"https://mangafire.to/title/", "", true},
		{"https://mangafire.to/", "", true},
	}

	for _, c := range cases {
		m := Mangafire{Grabber: &Grabber{URL: c.url}}
		got, err := m.hid()
		if (err != nil) != c.wantErr {
			t.Errorf("hid(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("hid(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// mangafireNextTarget mirrors what the click JS in browser.captureAPI does with
// mangafireNextPageSelector: document.querySelector takes the first match in
// document order, and the click is skipped when that element reports itself
// disabled. It returns "gone" (pagination ends), "disabled" (ditto), or the
// label of the button that would be clicked.
func mangafireNextTarget(t *testing.T, pager string) string {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pager))
	if err != nil {
		t.Fatalf("parsing pager fixture: %v", err)
	}
	b := doc.Find(mangafireNextPageSelector).First()
	if b.Length() == 0 {
		return "gone"
	}
	if _, ok := b.Attr("disabled"); ok {
		return "disabled"
	}
	if v, _ := b.Attr("aria-disabled"); v == "true" {
		return "disabled"
	}
	return strings.TrimSpace(b.Text())
}

// TestMangafireNextPageSelector is the actual #169 regression guard: the bug was
// that short chapter lists render a numbered-only pager (no "Next page" arrow),
// so a selector matching only the arrow never advanced and the list was silently
// truncated to the first page. Reverting mangafireNextPageSelector to the
// arrow-only selector must fail the "numbered-only" cases below.
func TestMangafireNextPageSelector(t *testing.T) {
	// short list: numbered buttons only, no arrows at all
	numbered := func(active int) string {
		var sb strings.Builder
		sb.WriteString(`<div class="npager">`)
		for n := 1; n <= 3; n++ {
			cls := "npager__num"
			if n == active {
				cls += " is-active"
			}
			sb.WriteString(`<a class="` + cls + `">` + strconv.Itoa(n) + `</a>`)
		}
		sb.WriteString(`</div>`)
		return sb.String()
	}

	cases := []struct {
		name  string
		pager string
		want  string
	}{
		{"numbered-only pager, first page", numbered(1), "2"},
		{"numbered-only pager, middle page", numbered(2), "3"},
		// no numbered sibling and no arrow: the walk stops here
		{"numbered-only pager, last page", numbered(3), "gone"},
		{
			// windowed pager: the numbered sibling comes first in document
			// order, and advances exactly one page just like the arrow would
			name: "windowed pager, active mid-window",
			pager: `<div class="npager">` +
				`<a class="npager__nav" aria-label="Previous page">&lsaquo;</a>` +
				`<a class="npager__num">1</a>` +
				`<a class="npager__num is-active">2</a>` +
				`<a class="npager__num">3</a>` +
				`<a class="npager__nav" aria-label="Next page">&rsaquo;</a>` +
				`</div>`,
			want: "3",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mangafireNextTarget(t, c.pager); got != c.want {
				t.Errorf("next target = %q, want %q", got, c.want)
			}
		})
	}

	// the arrow is what covers the case the numbered selector cannot: the active
	// number is the last of its window but more pages follow
	t.Run("windowed pager, active last of window", func(t *testing.T) {
		pager := `<div class="npager">` +
			`<a class="npager__num">4</a>` +
			`<a class="npager__num is-active">5</a>` +
			`<a class="npager__nav" aria-label="Next page">next</a>` +
			`</div>`
		if got := mangafireNextTarget(t, pager); got != "next" {
			t.Errorf("next target = %q, want %q", got, "next")
		}
	})

	t.Run("windowed pager, last page with disabled arrow", func(t *testing.T) {
		pager := `<div class="npager">` +
			`<a class="npager__num">9</a>` +
			`<a class="npager__num is-active">10</a>` +
			`<a class="npager__nav" aria-label="Next page" aria-disabled="true">next</a>` +
			`</div>`
		if got := mangafireNextTarget(t, pager); got != "disabled" {
			t.Errorf("next target = %q, want %q", got, "disabled")
		}
	})
}

func TestParseMangafireChapters(t *testing.T) {
	const prefix = "/api/titles/9062q/chapters"
	page := func(page int, body string) browser.APIResponse {
		return browser.APIResponse{
			URL:  "https://mangafire.to" + prefix + "?language=en&sort=number&order=desc&page=" + strconv.Itoa(page) + "&limit=20&vrf=x",
			Body: body,
		}
	}

	cases := []struct {
		name      string
		responses []browser.APIResponse
		wantNums  []float64
		wantIDs   []int64
	}{
		{
			// the #169 regression: chapters spread over several pager pages
			// (sorted descending by the site) must all be collected, in order
			name: "multiple pages merge",
			responses: []browser.APIResponse{
				page(1, `{"items":[{"id":3,"number":3,"name":"C3","language":"en","type":"official"},{"id":2,"number":2,"name":"C2","language":"en","type":"official"}]}`),
				page(2, `{"items":[{"id":1,"number":1,"name":"C1","language":"en","type":"official"}]}`),
			},
			wantNums: []float64{3, 2, 1},
		},
		{
			// official and unofficial uploads of one number are separate
			// releases: every item is kept, in the site's order
			name: "every release of a number kept",
			responses: []browser.APIResponse{
				page(1, `{"items":[{"id":9455278,"number":1194,"type":"official"},{"id":9451857,"number":1194,"type":"unofficial"},{"id":9426668,"number":1193,"type":"official"},{"id":9422044,"number":1193,"type":"unofficial"}]}`),
			},
			wantNums: []float64{1194, 1194, 1193, 1193},
			wantIDs:  []int64{9455278, 9451857, 9426668, 9422044},
		},
		{
			name: "non-matching and malformed responses are skipped",
			responses: []browser.APIResponse{
				{URL: "https://mangafire.to/api/titles/9062q?vrf=x", Body: `{"data":{"title":"nope"}}`},
				page(1, `not json`),
				page(2, `{"items":[{"id":1,"number":1,"type":"official"}]}`),
			},
			wantNums: []float64{1},
		},
		{
			name:      "no responses yields empty list",
			responses: nil,
			wantNums:  []float64{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMangafireChapters(c.responses, prefix)
			if len(got) != len(c.wantNums) {
				t.Fatalf("got %d chapters, want %d (%v)", len(got), len(c.wantNums), got)
			}
			for i, item := range got {
				if item.Number != c.wantNums[i] {
					t.Errorf("chapter[%d].Number = %v, want %v", i, item.Number, c.wantNums[i])
				}
				if c.wantIDs != nil && item.Id != c.wantIDs[i] {
					t.Errorf("chapter[%d].Id = %v, want %v", i, item.Id, c.wantIDs[i])
				}
			}
		})
	}
}

func TestMangafireGroup(t *testing.T) {
	cases := map[string]string{
		"official":   "Official",
		"unofficial": "Unofficial",
		"":           "",
	}
	for in, want := range cases {
		if got := mangafireGroup(in); got != want {
			t.Errorf("mangafireGroup(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMangafireLanguage(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false}, // no flag: site default untouched
		{"en", "en", false},
		{"br", "pt-br", false}, // the code from #185
		{"PT-BR", "pt-br", false},
		{" mx ", "es-la", false},
		{"es-la", "es-la", false},
		{"es", "es", false},
		{"jp", "ja", false},
		{"xx", "", true},
	}
	for _, c := range cases {
		got, err := mangafireLanguage(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("mangafireLanguage(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if got.code != c.want {
			t.Errorf("mangafireLanguage(%q) = %q, want %q", c.in, got.code, c.want)
		}
	}
}

// the SPA's chapter calls for the default language fire before the switch, so
// the captured set holds both languages; only the requested feed may be kept
func TestMangafireChaptersFilteredByLanguage(t *testing.T) {
	const base = "https://mangafire.to/api/titles/mvzz/chapters"
	responses := []browser.APIResponse{
		{URL: base + "?language=en&sort=number&order=desc&page=1&limit=20&vrf=a", Body: `{"items":[{"id":1,"number":10,"language":"en","type":"official"}]}`},
		{URL: base + "?language=pt-br&sort=number&order=desc&page=1&limit=20&vrf=b", Body: `{"items":[{"id":2,"number":233,"language":"pt-br","type":"official"}]}`},
		{URL: base + "?language=pt-br&sort=number&order=desc&page=2&limit=20&vrf=c", Body: `{"items":[{"id":3,"number":232,"language":"pt-br","type":"official"}]}`},
	}
	got := parseMangafireChapters(responses, base+"?language=pt-br&")
	if len(got) != 2 || got[0].Id != 2 || got[1].Id != 3 {
		t.Errorf("got %+v, want only the pt-br chapters 233 and 232", got)
	}
}

func TestMangafireOptions(t *testing.T) {
	raw := `{"selected":"English","labels":["English","Spanish (LATAM)","Klingon","Português"]}`
	d, err := parseMangafireDropdown(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseMangafireDropdown: %v", err)
	}
	got := mangafireOptions(d)
	// Klingon has no code to type, so it's left out; the order is the site's
	if want := []string{"en", "es-la"}; !reflect.DeepEqual(got.Values, want) {
		t.Errorf("Values = %v, want %v", got.Values, want)
	}
	if got.Current != "en" || got.All {
		t.Errorf("Current = %q, All = %v, want en and false", got.Current, got.All)
	}
	if !d.offers("Spanish (LATAM)") || d.offers("Spanish") {
		t.Errorf("offers doesn't match the labels the dropdown shows")
	}
}

func TestParseMangafireDropdownError(t *testing.T) {
	if _, err := parseMangafireDropdown(json.RawMessage(`{"error":"language selector not found"}`)); err == nil {
		t.Error("expected the script's error to surface")
	}
	if _, err := parseMangafireDropdown(nil); err == nil {
		t.Error("expected an empty result to fail")
	}
}

func TestMangafireUnavailableLanguage(t *testing.T) {
	m := &Mangafire{languages: FilterOptions{Values: []string{"en", "es-la"}}}
	err := m.unavailableLanguage("ca")
	if want := `language "ca" isn't available for this title (available: en, es-la)`; err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
	if got := (&Mangafire{}).unavailableLanguage("ca").Error(); !strings.Contains(got, "available: none") {
		t.Errorf("got %q, want the none listing", got)
	}
}

func TestMangafireResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wanted string
		// volume is the id a volume link names, 0 for every other URL
		volume int64
	}{
		{
			name:   "chapter URL resolves to series URL",
			input:  "https://mangafire.to/title/dkw-one-piece/chapter/9485827",
			wanted: "https://mangafire.to/title/dkw-one-piece",
		},
		{
			// #465: a volume link used to be rendered as a series page, which
			// never showed a chapter row and timed out as a challenge
			name:   "volume URL resolves to series URL and keeps its id",
			input:  "https://mangafire.to/title/92kk8-naruto/volume/127605",
			wanted: "https://mangafire.to/title/92kk8-naruto",
			volume: 127605,
		},
		{
			name:   "volume URL with www, case, query and fragment",
			input:  "https://WWW.MangaFire.to/title/92kk8-naruto/Volume/127605?page=3#top",
			wanted: "https://WWW.MangaFire.to/title/92kk8-naruto",
			volume: 127605,
		},
		{
			// nothing says which volume, so it lists the chapters
			name:   "volume URL without an id",
			input:  "https://mangafire.to/title/92kk8-naruto/volume/",
			wanted: "https://mangafire.to/title/92kk8-naruto",
		},
		{
			name:   "www. prefix preserved",
			input:  "https://www.mangafire.to/title/dkw-one-piece/chapter/9485827",
			wanted: "https://www.mangafire.to/title/dkw-one-piece",
		},
		{
			name:   "legacy /manga/ format",
			input:  "https://mangafire.to/manga/one-piecee.dkw/chapter/9485827",
			wanted: "https://mangafire.to/manga/one-piecee.dkw",
		},
		{
			name:   "series URL unchanged",
			input:  "https://mangafire.to/title/dkw-one-piece",
			wanted: "https://mangafire.to/title/dkw-one-piece",
		},
		{
			name:   "short series URL unchanged",
			input:  "https://mangafire.to/title/9062q-itoko-no-onee-chan-ni-amaechau",
			wanted: "https://mangafire.to/title/9062q-itoko-no-onee-chan-ni-amaechau",
		},
		{
			name:   "series URL with query unchanged",
			input:  "https://mangafire.to/title/dkw-one-piece?lang=en",
			wanted: "https://mangafire.to/title/dkw-one-piece?lang=en",
		},
		{
			name:   "unrelated path unchanged",
			input:  "https://mangafire.to/",
			wanted: "https://mangafire.to/",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Mangafire{Grabber: &Grabber{URL: c.input}}
			m.resolveSeriesURL()
			if m.URL != c.wanted {
				t.Errorf("got %q, want %q", m.URL, c.wanted)
			}
			if m.volumeID != c.volume {
				t.Errorf("volumeID = %d, want %d", m.volumeID, c.volume)
			}
			// what load asks the title's API with
			if hid, err := m.hid(); c.wanted != "https://mangafire.to/" && (err != nil || hid == "") {
				t.Errorf("hid() = %q, %v after resolving", hid, err)
			}
		})
	}
}

func TestMangafirePagesAPI(t *testing.T) {
	cases := []struct {
		chapter MangafireChapter
		want    string
	}{
		{MangafireChapter{Id: 9054304}, "/api/chapters/9054304"},
		{MangafireChapter{Id: 9054304, reader: mangafireChapter}, "/api/chapters/9054304"},
		{MangafireChapter{Id: 127605, reader: mangafireVolume}, "/api/volumes/127605"},
	}
	for _, c := range cases {
		if got := c.chapter.pagesAPI(); got != c.want {
			t.Errorf("pagesAPI() = %q, want %q", got, c.want)
		}
	}
}

// mangafireVolumeItems reads the volumes list as load does, from a response of
// the shape the series page gets: every language's volumes in one list
func mangafireVolumeItems(t *testing.T) []mangafireChapterItem {
	t.Helper()
	const path = "/api/titles/92kk8/volumes"
	responses := []browser.APIResponse{
		{URL: "https://mangafire.to/api/titles/92kk8?vrf=a", Body: `{"data":{"title":"Naruto","url":"/title/92kk8-naruto"}}`},
		{URL: "https://mangafire.to" + path + "?vrf=b", Body: `{"items":[` +
			`{"id":129287,"number":72,"name":"Uzumaki Naruto!!","language":"en","chapterCount":10},` +
			`{"id":127608,"number":2,"name":"The Worst Client","language":"en","chapterCount":10},` +
			`{"id":127605,"number":1,"name":"Naruto Uzumaki","language":"en","chapterCount":7},` +
			`{"id":140001,"number":10,"name":"","language":"es-la","chapterCount":9},` +
			`{"id":150002,"number":2,"name":"","language":"fr","chapterCount":10},` +
			`{"id":150001,"number":1,"name":"","language":"fr","chapterCount":7},` +
			`{"id":160001,"number":1,"name":"","language":"tlh","chapterCount":7}]}`},
		// the chapter list the page also loads isn't a volume
		{URL: "https://mangafire.to/api/titles/92kk8/chapters?language=en&page=1&vrf=c", Body: `{"items":[{"id":1326884,"number":700,"language":"en","type":"unofficial"}]}`},
	}
	return parseMangafireChapters(responses, path)
}

func TestMangafireVolumes(t *testing.T) {
	items := mangafireVolumeItems(t)
	if len(items) != 7 {
		t.Fatalf("read %d volumes, want 7 (%+v)", len(items), items)
	}
	// Klingon has no code to type, so it's left out; the order is the site's
	wantLanguages := []string{"en", "es-la", "fr"}

	cases := []struct {
		name     string
		linked   int64
		language string
		wantIDs  []int64
		current  string
	}{
		{"the linked volume's language by default", 127605, "", []int64{129287, 127608, 127605}, "en"},
		{"a French link lists the French volumes", 150001, "", []int64{150002, 150001}, "fr"},
		{"all is the default too", 127605, "all", []int64{129287, 127608, 127605}, "en"},
		{"--language picks another language's volumes", 127605, "fr", []int64{150002, 150001}, "en"},
		{"--language aliases resolve", 127605, " MX ", []int64{140001}, "en"},
		{"--language makes up for an unknown volume", 1, "es-la", []int64{140001}, ""},
		{"a language without a code is still the link's default", 160001, "", []int64{160001}, "tlh"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			volumes, languages, err := mangafireVolumes(items, c.linked, c.language)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var ids []int64
			for _, v := range volumes {
				ids = append(ids, v.Id)
			}
			if !reflect.DeepEqual(ids, c.wantIDs) {
				t.Errorf("volumes = %v, want %v", ids, c.wantIDs)
			}
			if !reflect.DeepEqual(languages.Values, wantLanguages) || languages.Current != c.current || languages.All {
				t.Errorf("languages = %+v, want %v with %q current", languages, wantLanguages, c.current)
			}
		})
	}
}

func TestMangafireVolumesErrors(t *testing.T) {
	items := mangafireVolumeItems(t)

	for _, language := range []string{"ja", "es", "xx"} {
		_, languages, err := mangafireVolumes(items, 127605, language)
		var unavailable *UnavailableFilterError
		if !errors.As(err, &unavailable) {
			t.Fatalf("language %q: got %v, want an UnavailableFilterError", language, err)
		}
		if unavailable.Value != language || !reflect.DeepEqual(unavailable.Available, []string{"en", "es-la", "fr"}) {
			t.Errorf("language %q: got %+v", language, unavailable)
		}
		// the options stay offered, so a front end can still list them
		if len(languages.Values) != 3 || languages.Current != "en" {
			t.Errorf("language %q: languages = %+v", language, languages)
		}
	}

	// a volume the title doesn't list has no language to default to
	if _, _, err := mangafireVolumes(items, 1, ""); err == nil || !strings.Contains(err.Error(), "volume 1 not found") {
		t.Errorf("unknown volume: got %v", err)
	}
	if _, _, err := mangafireVolumes(nil, 127605, ""); err == nil || !strings.Contains(err.Error(), "no volumes found") {
		t.Errorf("no volumes: got %v", err)
	}
}

func TestMangafireVolumeNumber(t *testing.T) {
	cases := map[float64]int{1: 1, 72: 72, 0: 0, -1: 0, 10.5: 0}
	for in, want := range cases {
		if got := mangafireVolumeNumber(in); got != want {
			t.Errorf("mangafireVolumeNumber(%v) = %d, want %d", in, got, want)
		}
	}
}
