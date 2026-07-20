package grabber

import (
	"net/url"
	"strings"

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
		Domains:      []string{"toongod.org", "dragontea.ink"},
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

// Test matches the URL against the registered browser-based site domains and,
// on match, renders the series page in the browser
func (m *PlainHTMLBrowser) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")

	matched := false
	for _, selector := range browserSelectors {
		for _, domain := range selector.Domains {
			if host == domain {
				m.selector = selector
				m.site = selector.SiteSelector
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		return false, nil
	}

	color.Blue("this site needs a real browser, launching Chrome (may take a few seconds)...")
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
