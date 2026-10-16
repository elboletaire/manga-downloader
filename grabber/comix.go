// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
// per scanlation group upload). FetchChapters returns every row, with its
// group, and --scanlator is applied to them by core (see grabber.SelectGroup).
//
// The reader lazily mounts its pages; an init script turns on its "Preload
// images: all" setting so they all mount at once, and the snapshot is taken
// once every slot has its img (scrolling the reader's container if not).
//
// Cloudflare answers loads faster than about 0.7 a second (six in a burst)
// with 502 and Retry-After: 60, so loads are paced (comixLoadGap) across
// every Comix instance.
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
	// cachedURL and cachedChapters cache the parsed chapter rows per series
	// URL, so changing the scanlator filter doesn't re-fetch the whole page list
	cachedURL      string
	cachedChapters []*ComixChapter
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

// its chapter API serves headless browsers a captcha of its own, every time,
// and rate-limits per browser session, so a 429 is retried in a new one
func init() {
	browser.RequireVisible("comix.to")
	browser.RestartOnRateLimit("comix.to")
}

// comixChapterRe matches chapter reader URLs whose path is
// /title/{slug}/{chapter-id}: the series URL is the first two path segments.
var comixChapterRe = regexp.MustCompile(`(?i)^(https?://(?:www\.)?comix\.to/title/[^/?#]+)/[^/?#]`)

// comixPagingRe extracts the "showing X to Y of Z items" chapter-list footer
// text, used to know when pagination is exhausted
var comixPagingRe = regexp.MustCompile(`(\d+)\s+to\s+(\d+)\s+of\s+(\d+)`)

const (
	comixChapterRowSelector = "li.mchap-item"
	comixFooterSelector     = ".mchap-foot__hint"
	comixSlotSelector       = ".rpage-page"
	comixImageSelector      = `img.rpage-page__img[src]:not([src=""])`
	comixReaderScrollPause  = 150 * time.Millisecond
	// comixLoadBurst loads go out unpaced, then comixLoadGap separates them
	comixLoadBurst = 3
	comixLoadGap   = 2 * time.Second
)

// comixSleep and comixNow are time.Sleep and time.Now, replaced in tests
var (
	comixSleep = time.Sleep
	comixNow   = time.Now
)

// comixPace is shared by every Comix: used is the burst spent (idle time earns
// it back, a load per gap), last is when the previous load finished
var comixPace struct {
	sync.Mutex
	used int
	last time.Time
}

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

// load runs get, spaced comixLoadGap after the previous load once the burst
// is spent. The browser package retries a 429/5xx.
func (c *Comix) load(get func() (string, error)) (string, error) {
	comixPace.Lock()
	defer comixPace.Unlock()

	idle := comixNow().Sub(comixPace.last)
	// capped before the int conversion, which would overflow on 32-bit
	earned := int(min(idle/comixLoadGap, comixLoadBurst))
	comixPace.used = max(0, comixPace.used-earned)
	if comixPace.used >= comixLoadBurst {
		comixSleep(comixLoadGap - idle)
	}
	html, err := get()
	comixPace.used = min(comixPace.used+1, comixLoadBurst)
	comixPace.last = comixNow()

	return html, err
}

// fetchPage1 renders (and caches) the first page of the series' chapter list
func (c *Comix) fetchPage1() (*goquery.Document, error) {
	if c.doc != nil {
		return c.doc, nil
	}
	c.resolveSeriesURL()

	html, err := c.load(func() (string, error) {
		return browser.GetHTML(c.URL, comixFooterSelector, 0)
	})
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
// been seen. Each row is one release, with its scanlation group in Chapter.Group.
//
// Results are cached per series URL, so a scanlator change in the app is a
// filter on the cached rows instead of a new walk of the browser.
func (c *Comix) FetchChapters() (chapters Filterables, errs []error) {
	c.resolveSeriesURL()

	if c.cachedURL == c.URL && c.cachedChapters != nil {
		return comixListing(c.cachedChapters), nil
	}

	var rows []*ComixChapter
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
			html, err = c.load(func() (string, error) {
				return browser.GetHTML(pageURL, comixFooterSelector, 0)
			})
			if err == nil {
				doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
			}
		}
		if err != nil {
			errs = append(errs, err)
			break
		}

		doc.Find(comixChapterRowSelector).Each(func(i int, s *goquery.Selection) {
			ch, err := comixParseRow(s, c.BaseUrl())
			if err != nil {
				return
			}
			rows = append(rows, ch)
		})

		shown, total, ok := comixParsePaging(doc.Find(comixFooterSelector).Text())
		if !ok || shown >= total {
			break
		}
		page++
	}

	// Cache for filter-only changes, unless errors cut the walk short
	if len(errs) == 0 {
		c.cachedURL = c.URL
		c.cachedChapters = rows
	}

	return comixListing(rows), errs
}

// comixListing returns the rows as the listing core filters
func comixListing(rows []*ComixChapter) Filterables {
	chapters := make(Filterables, 0, len(rows))
	for _, ch := range rows {
		chapters = append(chapters, ch)
	}

	return chapters
}

// FetchChapter renders the chapter reader page in a real browser, with
// every page preloaded (or scrolled into view), and reads the mounted page
// images back out
func (c *Comix) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*ComixChapter)

	html, err := c.load(func() (string, error) {
		return browser.GetHTMLWithScroll(chap.URL, comixImageSelector, comixReaderScroll, 0)
	})
	if err != nil {
		return nil, err
	}

	return comixChapter(chap, html)
}

// comixChapter builds the chapter of one release from its rendered reader
func comixChapter(chap *ComixChapter, html string) (*Chapter, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	pages, err := comixPages(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ChapterLabel(chap), err)
	}

	chapter := &Chapter{
		Title:      chap.GetTitle(),
		Label:      labelOf(chap),
		Number:     chap.GetNumber(),
		Language:   "en",
		ID:         chap.ID,
		Pages:      pages,
		PagesCount: int64(len(pages)),
	}
	chapter.SetGroups(chap.GetGroups()...)

	return chapter, nil
}

// Refilterable implements grabber.Refilterable: FetchChapters serves the cached
// rows, every group's, so a scanlator change is a filter on them.
func (*Comix) Refilterable() {}

// comixParseRow extracts a chapter entry from a rendered chapter list row.
// The scanlation group (`.mchap-row__secondary a.mchap-row__group span`) is left empty if none.
func comixParseRow(row *goquery.Selection, baseURL string) (*ComixChapter, error) {
	text := row.Find(".mchap-row__ch").Text()
	number, ok := parseChapterNumber(text)
	if !ok {
		return nil, fmt.Errorf("couldn't parse chapter number from %q", text)
	}

	href := strings.TrimSpace(row.Find("a.mchap-row__primary").AttrOr("href", ""))
	if href == "" {
		return nil, fmt.Errorf("no chapter link found")
	}
	if !strings.HasPrefix(href, "http") {
		href = baseURL + href
	}

	// the group is empty when the row names none
	group := strings.TrimSpace(row.Find(".mchap-row__secondary a.mchap-row__group span").Text())

	chapter := &ComixChapter{
		Chapter{
			Number: number,
			ID:     chapterURLID(href),
			Label:  Label{Style: LabelNumbered},
		},
		href,
	}
	chapter.SetGroups(group)

	return chapter, nil
}

// comixPages reads the page images off a rendered reader. A slot without
// its img never mounted, which would silently drop the page, so it's an
// error instead.
//
// The srcs are read as soon as the imgs mount, before they load, so a dead
// first host isn't retried on the reader's fallback hosts.
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
