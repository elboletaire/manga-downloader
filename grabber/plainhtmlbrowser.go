// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/fatih/color"
)

// BrowserSiteSelector is a SiteSelector for sites that need a real browser
// (javascript rendering, TLS-fingerprint blocks or cloudflare challenges).
// Unlike plain SiteSelector entries, these are matched by domain instead of
// by fetching the page, since starting a browser is expensive.
type BrowserSiteSelector struct {
	SiteSelector
	// Domains that this selector applies to (matched without the www. prefix)
	Domains []string
	// ChaptersWait is the CSS selector to wait for on the series page
	ChaptersWait string
	// ImageWait is the CSS selector to wait for on the reader page
	ImageWait string
	// Settle is an extra wait applied after ChaptersWait/ImageWait first
	// match, for pages that keep appending content afterwards (e.g.
	// mangahub.io's reader, which progressively appends more page <img>
	// elements to the DOM after the first one shows up). Zero means no
	// extra wait, matching every other site's immediate-render behavior.
	Settle time.Duration
}

// browserSelectors is the list of sites that need browser rendering. Their
// series & reader pages are rendered in Chrome, but images are downloaded
// via plain HTTP reusing the browser cookies & user agent.
var browserSelectors = []BrowserSiteSelector{
	// Madara-based wordpress sites behind a cloudflare challenge, they
	// usually need --browser-visible
	{
		SiteSelector: SiteSelector{
			Title:        "div.post-title h1",
			Rows:         "li.wp-manga-chapter",
			Chapter:      "a",
			ChapterTitle: "a",
			Link:         "a",
			Image:        "div.reading-content img",
		},
		Domains:      []string{"toongod.org", "dragontea.ink", "manhuaus.com", "toonily.com"},
		ChaptersWait: "li.wp-manga-chapter",
		ImageWait:    "div.reading-content",
	},
	// kappabeast.com: react SPA behind a cloudflare challenge, usually
	// needs --browser-visible. Images are hosted on blogger/strapi CDNs.
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         `a[href*="/reader/"]`,
			Chapter:      "h4",
			ChapterTitle: "p",
			Image:        `img[alt^="Page"]`,
		},
		Domains:      []string{"kappabeast.com"},
		ChaptersWait: `a[href*="/reader/"]`,
		ImageWait:    `img[alt^="Page"]`,
	},
	// sushiscan.net (french) and drakecomic.org (Drake Scans): mangastream/
	// themesia theme behind a cloudflare challenge, usually needs
	// --browser-visible. All the reader pages come from the embedded
	// ts_reader javascript call. drakecomic.org's initial render is an
	// (almost) empty body — #chapterlist is filled a few seconds later by
	// an admin-ajax.php call — but WaitVisible already polls for the wait
	// selector, so no extra settle is needed.
	{
		SiteSelector: SiteSelector{
			Title:        "h1.entry-title",
			Rows:         "#chapterlist li",
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Link:         "a",
			Image:        "#readerarea img",
		},
		Domains:      []string{"sushiscan.net", "drakecomic.org"},
		ChaptersWait: "#chapterlist li",
		ImageWait:    "#readerarea img",
	},
	// mangakakalot.gg (manganelo/manganato family) behind a cloudflare challenge, needs
	// --browser-visible. Images sit on a CDN that only checks the Referer, so
	// they still download via plain HTTP (BaseUrl referer is enough).
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         ".chapter-list .row",
			Chapter:      "a",
			ChapterTitle: "a",
			Link:         "a",
			Image:        ".container-chapter-reader img",
		},
		Domains:      []string{"mangakakalot.gg", "natomanga.com"},
		ChaptersWait: ".chapter-list .row",
		ImageWait:    ".container-chapter-reader img",
	},
	// mangahub.io: behind a cloudflare managed challenge that a visible
	// browser clears in a few seconds; images are served from a plain CDN
	// (imgx.mghcdn.com) and download fine via plain HTTP with just a
	// referer, no cookies needed. Chapter rows use CSS-module-hashed class
	// names (subject to change on redeploys), so we key off the stable
	// Bootstrap "list-group-item" class instead and pull the chapter number
	// straight out of the whole row's text (it contains "#1188").
	{
		SiteSelector: SiteSelector{
			Title: "h1",
			Rows:  "li.list-group-item",
			Link:  `a[href*="/chapter/"]`,
			Image: "img.PB0mN",
		},
		Domains:      []string{"mangahub.io"},
		ChaptersWait: "li.list-group-item",
		ImageWait:    "img.PB0mN",
		// the reader appends page <img> tags progressively; without this,
		// GetHTML returns as soon as the first one shows up and later
		// pages are missing (a 16-page chapter yielded only 6 without it)
		Settle: 5 * time.Second,
	},
	// manhuatop.org: Madara wordpress theme behind a cloudflare challenge,
	// usually needs --browser-visible. Its reader wraps real pages in
	// div.reading-content img#image-N, but also throws in a decorative
	// "about_manhuatop" banner and a "To_be_continued" footer image sharing
	// the same wp-manga-chapter-img class as real pages, so the plain
	// "div.reading-content img" selector (used by the toongod/manhuaus entry
	// above) would pull them in as junk pages — the id^="image-" filter
	// keeps only the real, ordered page images. Images download via plain
	// HTTP with no cookies needed once harvested.
	{
		SiteSelector: SiteSelector{
			Title:        "div.post-title h1",
			Rows:         "li.wp-manga-chapter",
			Chapter:      "a",
			ChapterTitle: "a",
			Link:         "a",
			Image:        `div.reading-content img[id^="image-"]`,
		},
		Domains:      []string{"manhuatop.org"},
		ChaptersWait: "li.wp-manga-chapter",
		ImageWait:    `div.reading-content img[id^="image-"]`,
	},
}

// PlainHTMLBrowser is the browser-rendered variant of PlainHTML: pages are
// fetched with a local Chrome instead of plain HTTP requests, then parsed
// with the same selector-driven logic.
type PlainHTMLBrowser struct {
	*PlainHTML
	selector BrowserSiteSelector
}

// NewPlainHTMLBrowser returns a new PlainHTMLBrowser grabber
func NewPlainHTMLBrowser(g *Grabber) *PlainHTMLBrowser {
	return &PlainHTMLBrowser{PlainHTML: NewPlainHTML(g)}
}

// matchBrowserSelector returns the registered browser selector for the given
// host (matched without the www. prefix), if any
func matchBrowserSelector(host string) (BrowserSiteSelector, bool) {
	host = strings.TrimPrefix(host, "www.")
	for _, selector := range browserSelectors {
		for _, domain := range selector.Domains {
			if host == domain {
				return selector, true
			}
		}
	}
	return BrowserSiteSelector{}, false
}

// Test matches the URL against the registered browser-based site domains and,
// on match, renders the series page in the browser
func (m *PlainHTMLBrowser) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")

	selector, matched := matchBrowserSelector(host)
	if !matched {
		return false, nil
	}
	m.selector = selector
	m.site = selector.SiteSelector
	browser.SetSettle(m.selector.Settle)

	color.Blue("this site needs a real browser, launching Chrome (may take a few seconds)...")
	// GetHTML tries headless first and, if the page is behind a challenge,
	// automatically reopens a visible window and retries — no flag needed.
	html, err := browser.GetHTML(m.URL, m.selector.ChaptersWait, 0)
	if err != nil {
		return false, err
	}

	m.doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return false, err
	}
	m.rows = m.doc.Find(m.site.Rows)

	return m.rows.Length() > 0, nil
}

// FetchChapter renders the reader page in the browser and extracts its pages
func (m *PlainHTMLBrowser) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*PlainHTMLChapter)

	html, err := browser.GetHTML(chap.URL, m.selector.ImageWait, 0)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	return m.chapterFromDoc(f, doc), nil
}
