// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

// Cloudflare blocks the headless browser on these sites outright
func init() {
	browser.RequireVisible("dragontea.ink")
	browser.RequireVisible("toongod.org")
	browser.RequireVisible("manhuaus.com")
	browser.RequireVisible("setsuscans.com")
	browser.RequireVisible("asmotoon.com")
}

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
	// SeriesPathDepth is the number of non-empty path segments in a series
	// URL (e.g. 2 for /serie/{slug}). When > 0, Test() reconstructs the
	// series URL by trimming any extra path segments, so chapter URLs like
	// /serie/{slug}/chapter-N are resolved without a wasted browser render.
	// 0 disables reconstruction.
	SeriesPathDepth int
	// Novel is a CSS selector for a marker that the series page is a novel,
	// which has no images to download; Test refuses such a series.
	Novel string
}

// errNovel refuses a series whose Novel selector matches: its chapters are text
var errNovel = errors.New("this is a novel, which manga-downloader can't download")

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
		Domains:         []string{"toongod.org", "dragontea.ink", "manhuaus.com", "toonily.com"},
		ChaptersWait:    "li.wp-manga-chapter",
		ImageWait:       "div.reading-content",
		SeriesPathDepth: 2,
	},
	// setsuscans.com: Madara wordpress theme behind a cloudflare challenge
	// (chapter list loads via an admin-ajax call the browser must run, so a
	// plain-HTTP Tcb/PlainHTML Test() never even sees it). Reader images are
	// lazy-loaded (data-src) but sit on the same domain and download fine via
	// plain HTTP once the browser has cleared the challenge once.
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         "li.wp-manga-chapter",
			Chapter:      "a",
			ChapterTitle: "a",
			Link:         "a",
			Image:        "div.reading-content img",
		},
		Domains:         []string{"setsuscans.com"},
		ChaptersWait:    "li.wp-manga-chapter",
		ImageWait:       "div.reading-content img",
		SeriesPathDepth: 2,
	},
	// kappabeast.com: react SPA behind a cloudflare challenge, usually
	// needs --browser-visible. Images are hosted on blogger/strapi CDNs.
	// Chapter links nest one level deeper than the series (/series/{slug}/reader/…)
	// so depth 2 trims them correctly.
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         `a[href*="/reader/"]`,
			Chapter:      "h4",
			ChapterTitle: "p",
			Image:        `img[alt^="Page"]`,
		},
		Domains:         []string{"kappabeast.com"},
		ChaptersWait:    `a[href*="/reader/"]`,
		ImageWait:       `img[alt^="Page"]`,
		SeriesPathDepth: 2,
	},
	// sushiscan.net (french): mangastream/themesia theme behind a
	// cloudflare challenge, usually needs --browser-visible. All the reader
	// pages come from the embedded ts_reader javascript call.
	// Chapter URLs use the format /{slug}-volume-N/ (one path segment,
	// different tree from series /catalogue/{slug}/), so SeriesPathDepth
	// can't reconstruct the series URL; the reader's .allc div carries
	// a direct link back to the series page instead.
	//
	// drakecomic.org (Drake Scans) used to share this entry too, but the
	// site moved wholesale to a plain-HTTP Next.js RSC platform on
	// drakecomic.net (see drakecomic.go) — #chapterlist et al no longer
	// exist there.
	{
		SiteSelector: SiteSelector{
			Title:        "h1.entry-title",
			Rows:         "#chapterlist li",
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Link:         "a",
			Image:        "#readerarea img",
			SeriesLink:   ".allc a",
		},
		Domains:         []string{"sushiscan.net"},
		ChaptersWait:    "#chapterlist li",
		ImageWait:       "#readerarea img",
		SeriesPathDepth: 2,
	},
	// mangahub.io: behind a cloudflare managed challenge that a visible
	// browser clears in a few seconds; images are served from a plain CDN
	// (imgx.mghcdn.com) and download fine via plain HTTP with just a
	// referer, no cookies needed. Chapter rows use CSS-module-hashed class
	// names (subject to change on redeploys), so we key off the stable
	// Bootstrap "list-group-item" class instead and pull the chapter number
	// straight out of the row's text (it contains "#1188"). Each row
	// actually holds TWO (sometimes three) anchors pointing at the same
	// chapter: the real slug-based link, plus one-or-more duplicate
	// "title=Chapter" links (an internal numeric-id URL, order relative to
	// the real link isn't stable) that appear to be a hover/prefetch
	// artifact. Both/all render identical text, so an untargeted Chapter
	// selector (or none, falling back to the whole row) concatenates them
	// into a doubled title. The duplicates are the only anchors carrying a
	// title attribute, so ":not([title])" reliably isolates the real one.
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         "li.list-group-item",
			Chapter:      "a:not([title])",
			ChapterTitle: "a:not([title])",
			Link:         "a:not([title])",
			Image:        "img.PB0mN",
		},
		Domains:      []string{"mangahub.io"},
		ChaptersWait: "li.list-group-item",
		ImageWait:    "img.PB0mN",
		// the reader appends page <img> tags progressively; without this,
		// GetHTML returns as soon as the first one shows up and later
		// pages are missing (a 16-page chapter yielded only 6 without it)
		Settle:          5 * time.Second,
		SeriesPathDepth: 2,
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
		Domains:         []string{"manhuatop.org"},
		ChaptersWait:    "li.wp-manga-chapter",
		ImageWait:       `div.reading-content img[id^="image-"]`,
		SeriesPathDepth: 2,
	},
	// asmotoon.com (Asmodeus Scans): Cloudflare needs a visible window. Series
	// and chapters are both one path segment, so a chapter URL finds its series
	// through SeriesLink; novels share the site and Novel refuses them.
	{
		SiteSelector: SiteSelector{
			Title:        "h1",
			Rows:         "a.chapter-row",
			Chapter:      ".ch-title-full",
			ChapterTitle: ".ch-title-full",
			Image:        "#strip-reader img.reader-page",
			SeriesLink:   `a[title="Back to series"]`,
			Locked:       "[data-lock-chip]",
		},
		Domains:      []string{"asmotoon.com"},
		ChaptersWait: "a.chapter-row",
		ImageWait:    "#strip-reader",
		Novel:        `span.text-violet-400:contains("NOVEL")`,
	},
	// templetoons.com (Temple Scan): Next.js RSC site behind a cloudflare
	// Turnstile challenge (plain HTTP is redirected to /challenge), which a
	// headless browser clears in a few seconds. Each row is itself the <a>
	// (Link empty), with the chapter number in its label. Reader pages embed
	// the page list in the RSC payload under an obfuscated key, handled by
	// getPlainHTMLImageURL, so Image is unused but kept as a fallback.
	{
		SiteSelector: SiteSelector{
			Title:        "h1#series-title",
			Rows:         `a[id^="chapter-link-"]`,
			Chapter:      "span.font-semibold",
			ChapterTitle: "span.font-semibold",
			Image:        "img",
		},
		Domains:         []string{"templetoons.com"},
		ChaptersWait:    `a[id^="chapter-link-"]`,
		ImageWait:       "img",
		SeriesPathDepth: 2,
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

// pathSeriesURL returns the series URL derived from m.URL's path when it
// looks like a chapter URL (more path segments than m.selector.SeriesPathDepth).
// Returns "" when no reconstruction is needed: the URL is already at series
// depth, or SeriesPathDepth is 0 (disabled). Query and fragment are stripped
// from the result.
func (m *PlainHTMLBrowser) pathSeriesURL() string {
	depth := m.selector.SeriesPathDepth
	if depth == 0 {
		return ""
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return ""
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) <= depth {
		return ""
	}
	seriesU := *u
	seriesU.Path = "/" + strings.Join(parts[:depth], "/")
	seriesU.RawQuery = ""
	seriesU.Fragment = ""
	return seriesU.String()
}

// Test matches the URL against the registered browser-based site domains and,
// on match, renders the series page in the browser. When the URL looks like a
// chapter URL (more path segments than SeriesPathDepth), the series URL is
// reconstructed from the path and rendered directly, avoiding a timeout that
// would occur while waiting for chapter-list rows on a reader page.
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

	// Prefer path reconstruction: when the URL has more segments than the
	// series depth, trim the extras to get the series URL without rendering.
	// This avoids a multi-minute timeout from waiting for chapter-list rows
	// on a reader page.
	renderURL := m.URL
	if reconstructed := m.pathSeriesURL(); reconstructed != "" {
		renderURL = reconstructed
	}

	// When we know the render target is a series URL (reconstruction worked),
	// wait only for chapter rows. Otherwise use a combined selector so both
	// series and reader pages resolve quickly.
	waitSel := m.selector.ChaptersWait
	if renderURL == m.URL && m.selector.ImageWait != "" {
		waitSel = m.selector.ChaptersWait + ", " + m.selector.ImageWait
	}

	// GetHTML tries headless first and, if the page is behind a challenge,
	// automatically reopens a visible window and retries — no flag needed.
	html, err := browser.GetHTML(renderURL, waitSel, 0)
	if err != nil {
		return false, err
	}

	m.doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return false, err
	}
	m.rows = m.doc.Find(m.site.Rows)

	if m.rows.Length() > 0 {
		if m.selector.Novel != "" && m.doc.Find(m.selector.Novel).Length() > 0 {
			return false, errNovel
		}
		m.URL = renderURL
		return true, nil
	}

	// No chapter rows on the rendered page. If a SeriesLink is defined and the
	// Image selector (when set) confirms this is a reader page, follow the
	// link and render the series URL (one extra render). The Image guard
	// mirrors PlainHTML's seriesURLCandidates to prevent a loose SeriesLink
	// from firing on unrelated pages.
	if m.site.SeriesLink != "" &&
		(m.site.Image == "" || m.doc.Find(m.site.Image).Length() > 0) {
		parsedURL, _ := url.Parse(renderURL)
		if link := m.doc.Find(m.site.SeriesLink).First(); link.Length() > 0 {
			if href, ok := link.Attr("href"); ok {
				seriesURL := resolveURLRelative(href, parsedURL)
				if seriesURL != "" && seriesURL != renderURL {
					seriesHTML, serr := browser.GetHTML(seriesURL, m.selector.ChaptersWait, 0)
					if serr == nil {
						seriesDoc, serr := goquery.NewDocumentFromReader(strings.NewReader(seriesHTML))
						if serr == nil {
							seriesRows := seriesDoc.Find(m.site.Rows)
							if seriesRows.Length() > 0 {
								if m.selector.Novel != "" && seriesDoc.Find(m.selector.Novel).Length() > 0 {
									return false, errNovel
								}
								m.URL = seriesURL
								m.doc = seriesDoc
								m.rows = seriesRows
								return true, nil
							}
						}
					}
				}
			}
		}
	}

	return false, nil
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
