// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strings"
	"testing"
)

func TestMatchBrowserSelector(t *testing.T) {
	cases := []struct {
		host      string
		wantMatch bool
	}{
		{"toongod.org", true},
		{"www.toongod.org", true}, // www. prefix is stripped
		{"dragontea.ink", true},
		{"kappabeast.com", true},
		{"sushiscan.net", true},
		{"drakecomic.org", false},  // handled by the Drakecomic grabber
		{"mangakakalot.gg", false}, // handled by the Manganelo grabber
		{"natomanga.com", false},   // handled by the Manganelo grabber
		{"manhuaus.com", true},
		{"toonily.com", true},
		{"www.toonily.com", true}, // www. prefix is stripped
		{"mangahub.io", true},
		{"www.mangahub.io", true}, // www. prefix is stripped
		{"manhuatop.org", true},
		{"www.manhuatop.org", true}, // www. prefix is stripped
		{"setsuscans.com", true},
		{"www.setsuscans.com", true}, // www. prefix is stripped
		{"example.com", false},
		{"notatoongod.org", false}, // must be an exact host match
	}

	for _, c := range cases {
		_, ok := matchBrowserSelector(c.host)
		if ok != c.wantMatch {
			t.Errorf("matchBrowserSelector(%q) matched = %v, want %v", c.host, ok, c.wantMatch)
		}
	}
}

// TestBrowserSelectorsAreComplete guards against a selector entry missing the
// fields the grabber relies on.
func TestBrowserSelectorsAreComplete(t *testing.T) {
	for _, s := range browserSelectors {
		if len(s.Domains) == 0 {
			t.Error("a browser selector has no domains")
		}
		if s.Title == "" || s.Rows == "" || s.Image == "" {
			t.Errorf("browser selector for %v is missing a required selector", s.Domains)
		}
		if s.ChaptersWait == "" || s.ImageWait == "" {
			t.Errorf("browser selector for %v is missing a wait selector", s.Domains)
		}
	}
}

func TestBrowserPathSeriesURL(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		depth int
		want  string
	}{
		// chapter URL resolved to series URL
		{"toonily chapter", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6/chapter-244/", 2, "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6"},
		// series URL with trailing slash — already at depth, no change
		{"toonily series trailing slash", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6/", 2, ""},
		// series URL without trailing slash — already at depth, no change
		{"toonily series", "https://toonily.com/serie/the-beginning-after-the-end-ea2130e6", 2, ""},
		// www. prefix is preserved in the result
		{"toongod www chapter", "https://www.toongod.org/webtoon/solo-leveling/chapter-200/", 2, "https://www.toongod.org/webtoon/solo-leveling"},
		// kappabeast reader adds two extra segments
		{"kappabeast chapter", "https://kappabeast.com/series/tekkarian/reader/chapter-2", 2, "https://kappabeast.com/series/tekkarian"},
		// depth 0 disables reconstruction
		{"depth 0 disabled", "https://toonily.com/serie/slug/chapter-1/", 0, ""},
		// query and fragment are stripped
		{"query stripped", "https://sushiscan.net/catalogue/mushoku-tensei/chapter-17/?style=list", 2, "https://sushiscan.net/catalogue/mushoku-tensei"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &PlainHTMLBrowser{
				PlainHTML: NewPlainHTML(&Grabber{URL: c.url}),
				selector:  BrowserSiteSelector{SeriesPathDepth: c.depth},
			}
			got := m.pathSeriesURL()
			if got != c.want {
				t.Errorf("pathSeriesURL() = %q, want %q", got, c.want)
			}
		})
	}
}

func asmotoonEntry(t *testing.T) BrowserSiteSelector {
	t.Helper()
	sel, ok := matchBrowserSelector("asmotoon.com")
	if !ok {
		t.Fatal("no asmotoon.com entry")
	}
	return sel
}

const asmotoonSeriesHTML = `<html><body><h1>My Son's Ex-Girlfriend Became My Subordinate</h1>
<div class="flex-1"><div class="text-xs">Type</div><div><span class="w-2"></span><span class="text-sm font-bold text-violet-400">%s</span></div></div>
<div id="chapters-list">
<a href="/my-son-chapter-5" class="chapter-row flex" data-ch-label="Chapter 5" data-ch-locked="1">
  <div class="ch-info"><div class="ch-title ch-title-full text-sm">Chapter 5                </div></div>
  <div class="ch-meta"><span data-lock-chip=""><img alt="Coins"><span>100</span></span></div></a>
<a href="/my-son-chapter-4" class="chapter-row flex" data-ch-label="Chapter 4" data-ch-locked="0">
  <div class="ch-info"><div class="ch-title ch-title-full text-sm">Chapter 4                </div></div>
  <div class="ch-meta"></div></a>
</div></body></html>`

func TestAsmotoonChapters(t *testing.T) {
	sel := asmotoonEntry(t)
	m := PlainHTML{Grabber: &Grabber{URL: "https://asmotoon.com/my-son"}, site: sel.SiteSelector}
	m.doc = docFromHTML(t, strings.Replace(asmotoonSeriesHTML, "%s", "MANGA", 1))
	m.rows = m.doc.Find(m.site.Rows)

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("chapters = %d, errs = %v", len(chapters), errs)
	}
	if chapters[0].GetNumber() != 5 || chapters[1].GetNumber() != 4 {
		t.Errorf("numbers = %v, %v", chapters[0].GetNumber(), chapters[1].GetNumber())
	}
	if !LockedOf(chapters[0]) || LockedOf(chapters[1]) {
		t.Errorf("locked = [%v %v], want [true false]", LockedOf(chapters[0]), LockedOf(chapters[1]))
	}
	if got := chapters[1].(*PlainHTMLChapter).URL; got != "https://asmotoon.com/my-son-chapter-4" {
		t.Errorf("url = %q", got)
	}
}

func TestAsmotoonNovelMarker(t *testing.T) {
	sel := asmotoonEntry(t)
	for typ, want := range map[string]bool{"NOVEL": true, "MANGA": false} {
		doc := docFromHTML(t, strings.Replace(asmotoonSeriesHTML, "%s", typ, 1))
		if got := doc.Find(sel.Novel).Length() > 0; got != want {
			t.Errorf("Novel marker on a %s series = %v, want %v", typ, got, want)
		}
	}
}

func TestAsmotoonReaderImages(t *testing.T) {
	sel := asmotoonEntry(t)
	doc := docFromHTML(t, `<html><body><a href="/my-son" title="Back to series">x</a>
<div id="strip-reader">
<div class="reader-page reader-note" data-page="1"><div class="prose-note">credits</div></div>
<div class="reader-page-ph"><img class="reader-page w-full" data-page="1" src="https://cdn.meowing.org/uploads/a.avif"></div>
<div class="reader-page-ph"><img class="reader-page w-full" data-page="2" src="https://cdn.meowing.org/uploads/b.avif"></div>
</div></body></html>`)
	want := []string{"https://cdn.meowing.org/uploads/a.avif", "https://cdn.meowing.org/uploads/b.avif"}
	if got := getPlainHTMLImageURL(sel.Image, doc); !reflect.DeepEqual(got, want) {
		t.Errorf("images = %v, want %v", got, want)
	}
	if doc.Find(sel.SeriesLink).AttrOr("href", "") != "/my-son" {
		t.Error("SeriesLink does not find the back link")
	}
}
