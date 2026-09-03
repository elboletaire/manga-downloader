// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
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
		wantTypes map[float64]string
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
			name: "official preferred over unofficial regardless of order",
			responses: []browser.APIResponse{
				page(1, `{"items":[{"id":10,"number":1,"type":"unofficial"},{"id":11,"number":1,"type":"official"},{"id":21,"number":2,"type":"official"},{"id":20,"number":2,"type":"unofficial"}]}`),
			},
			wantNums:  []float64{1, 2},
			wantTypes: map[float64]string{1: "official", 2: "official"},
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
				if want, ok := c.wantTypes[item.Number]; ok && item.Type != want {
					t.Errorf("chapter %v type = %q, want %q", item.Number, item.Type, want)
				}
			}
		})
	}
}
