// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"
	"testing"

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
