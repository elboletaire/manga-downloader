// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

// Comix is a grabber for comix.to. It looks like a plain server-rendered
// site, but only the manga's own metadata is embedded in the initial HTML:
// both the chapter list and the reader's page-image list are fetched
// client-side from a JSON API (/api/v1/manga/{hid}/chapters,
// /api/v1/chapters/{id}) that requires a rotating per-request token and,
// even with a valid token, returns its body AES-encrypted (an `x-enc: 1`
// response header, high-entropy base64 payload under an `"e"` key) — no
// Cloudflare wall involved, plain HTTP just can't get usable data out of it.
// A real browser sidesteps all of that: the site's own JS decrypts the
// response and renders normal DOM, so both pages are scraped by rendering
// them and reading selectors, like PlainHTML.
//
// The chapter list is classic page-based pagination (?page=N, not infinite
// scroll) and often lists the same chapter number multiple times (one row
// per scanlation group upload), so FetchChapters dedupes by number, keeping
// the first (highest-ranked) row seen for each.
//
// The reader lazily mounts its page images via an IntersectionObserver, in
// an inner scroll container (.rpage-main: window.scrollTo does nothing
// there). Its "Preload images: all" setting (persisted in localStorage)
// mounts every page at once instead, so an init script turns it on before
// the reader loads. The snapshot is taken as soon as every slot has its img
// mounted, without waiting for the images to download (a slot keeps its
// loader until then). If the setting doesn't take,
// browser.GetHTMLWithScroll steps the container a viewport at a time until
// they're all mounted.
//
// The images live on rotating hosts that refuse any request with a Referer
// (403; the reader sets referrerpolicy="no-referrer"), so pages are
// downloaded without one. They need no cookies.
type Comix struct {
	*Grabber
	title string
	// doc caches the first (page 1) chapter-list render, reused by both
	// FetchTitle and FetchChapters
	doc *goquery.Document
}

func NewComix(g *Grabber) *Comix {
	return &Comix{Grabber: g}
}

// ComixChapter represents a Comix Chapter
type ComixChapter struct {
	Chapter
	// URL is the chapter reader page URL
	URL string
}

// comixHostRe matches comix.to and its subdomains
var comixHostRe = regexp.MustCompile(`(?i)(^|\.)comix\.to$`)

// comixChapterRe matches chapter reader URLs whose path is
// /title/{slug}/{chapter-id}: the series URL is the first two path segments.
var comixChapterRe = regexp.MustCompile(`(?i)^(https?://(?:www\.)?comix\.to/title/[^/?#]+)/[^/?#]`)

// comixPagingRe extracts the "showing X to Y of Z items" chapter-list footer
// text, used to know when pagination is exhausted
var comixPagingRe = regexp.MustCompile(`(\d+)\s+to\s+(\d+)\s+of\s+(\d+)`)

const (
	comixChapterRowSelector = ".mchap-item"
	comixFooterSelector     = ".mchap-foot__hint"
	comixSlotSelector       = ".rpage-page"
	comixImageSelector      = `img.rpage-page__img[src]:not([src=""])`
	comixReaderScrollPause  = 150 * time.Millisecond
)

// comixPreloadJS sets the reader's "Preload images" to "all" in both of its
// persisted settings (paged and webtoon), keeping whatever else is there
const comixPreloadJS = `try{for(const k of["reader.default.v3","reader.webtoon.v3"]){` +
	`const v=JSON.parse(localStorage.getItem(k)||"null")||{state:{},version:0};` +
	`v.state=Object.assign({},v.state,{preload:"all"});` +
	`localStorage.setItem(k,JSON.stringify(v))}}catch(e){}`

// comixReaderScroll preloads every page, scrolling the reader as a fallback,
// until every slot has its img mounted
var comixReaderScroll = browser.Scroll{
	Container: ".rpage-main",
	Done: `(function(){var s=document.querySelectorAll("` + comixSlotSelector + `");` +
		`if(!s.length)return false;` +
		`for(var i=0;i<s.length;i++)if(!s[i].querySelector('` + comixImageSelector + `'))return false;` +
		`return true})()`,
	Pause: comixReaderScrollPause,
	Init:  comixPreloadJS,
}

// Test returns true if the URL is a comix.to URL. It only checks the
// hostname (no fetch) so it can be tried early, before starting a browser.
func (c *Comix) Test() (bool, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return false, nil
	}
	return comixHostRe.MatchString(u.Hostname()), nil
}

// resolveSeriesURL rewrites c.URL to the series URL when it is a chapter
// reader URL (/title/{slug}/{chapter-id}). Series URLs are unchanged.
func (c *Comix) resolveSeriesURL() {
	if match := comixChapterRe.FindStringSubmatch(c.URL); match != nil {
		c.URL = match[1]
	}
}

// fetchPage1 renders (and caches) the first page of the series' chapter list
func (c *Comix) fetchPage1() (*goquery.Document, error) {
	if c.doc != nil {
		return c.doc, nil
	}
	c.resolveSeriesURL()

	html, err := browser.GetHTML(c.URL, comixFooterSelector, 0)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	c.doc = doc

	return doc, nil
}

// FetchTitle fetches and returns the manga title
func (c *Comix) FetchTitle() (string, error) {
	if c.title != "" {
		return c.title, nil
	}

	doc, err := c.fetchPage1()
	if err != nil {
		return "", err
	}

	c.title = sanitizeTitle(doc.Find("h1.mpage__title").First().Text())

	return c.title, nil
}

// FetchChapters returns the chapters of the manga, walking the series'
// paginated chapter list (?page=N) until the footer reports every item has
// been seen. Duplicate chapter numbers (multiple groups uploading the same
// chapter) are collapsed, keeping the first one encountered.
func (c *Comix) FetchChapters() (chapters Filterables, errs []error) {
	seen := map[float64]bool{}
	page := 1

	for {
		var doc *goquery.Document
		var err error
		if page == 1 {
			doc, err = c.fetchPage1()
		} else {
			pageURL, uerr := comixWithPage(c.URL, page)
			if uerr != nil {
				errs = append(errs, uerr)
				break
			}
			var html string
			html, err = browser.GetHTML(pageURL, comixFooterSelector, 0)
			if err == nil {
				doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
			}
		}
		if err != nil {
			errs = append(errs, err)
			break
		}

		doc.Find(comixChapterRowSelector).Each(func(i int, s *goquery.Selection) {
			text := s.Find(".mchap-row__ch").Text()
			number, ok := parseChapterNumber(text)
			if !ok || seen[number] {
				return
			}

			href := strings.TrimSpace(s.Find("a.mchap-row__primary").AttrOr("href", ""))
			if href == "" {
				return
			}
			if !strings.HasPrefix(href, "http") {
				href = c.BaseUrl() + href
			}

			seen[number] = true
			chapters = append(chapters, &ComixChapter{
				Chapter{
					Number: number,
					Title:  "Chapter " + strconv.FormatFloat(number, 'f', -1, 64),
				},
				href,
			})
		})

		shown, total, ok := comixParsePaging(doc.Find(comixFooterSelector).Text())
		if !ok || shown >= total {
			break
		}
		page++
	}

	return chapters, errs
}

// FetchChapter renders the chapter reader page in a real browser, with
// every page preloaded (or scrolled into view), and reads the mounted page
// images back out
func (c *Comix) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*ComixChapter)

	html, err := browser.GetHTMLWithScroll(chap.URL, comixImageSelector, comixReaderScroll, 0)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	pages, err := comixPages(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.GetTitle(), err)
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   "en",
		Pages:      pages,
		PagesCount: int64(len(pages)),
	}, nil
}

// comixPages reads the page images off a rendered reader. A slot without
// its img never mounted, which would silently drop the page, so it's an
// error instead.
func comixPages(doc *goquery.Document) ([]Page, error) {
	slots := doc.Find(comixSlotSelector)
	if slots.Length() == 0 {
		return nil, fmt.Errorf("no pages found in the reader")
	}

	var pages []Page
	missing := 0
	slots.Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.Find(comixImageSelector).First().AttrOr("src", ""))
		if src == "" {
			missing++
			return
		}
		pages = append(pages, Page{
			Number:    int64(len(pages) + 1),
			URL:       src,
			NoReferer: true,
		})
	})
	if missing > 0 {
		return nil, fmt.Errorf("%d of %d pages never loaded in the reader", missing, slots.Length())
	}

	return pages, nil
}

// comixWithPage returns seriesURL with its "page" query param set to page
func comixWithPage(seriesURL string, page int) (string, error) {
	u, err := url.Parse(seriesURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("page", fmt.Sprint(page))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// comixParsePaging parses the chapter-list footer text ("Showing 1 to 20 of
// 248 items") into (shown, total), ok=false if it didn't match
func comixParsePaging(text string) (shown, total int, ok bool) {
	m := comixPagingRe.FindStringSubmatch(text)
	if len(m) != 4 {
		return 0, 0, false
	}
	shown, err1 := strconv.Atoi(m[2])
	total, err2 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return shown, total, true
}
