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
// per scanlation group upload). When no scanlator is selected, FetchChapters
// dedupes by number keeping the first (highest-ranked) row per group, in a
// deterministic order. With a scanlator selected, only that group's uploads
// are returned.
//
// The reader lazily mounts its pages; an init script turns on its "Preload
// images: all" setting so they all mount at once, and the snapshot is taken
// once every slot has its img (scrolling the reader's container if not).
//
// Cloudflare rate-limits page loads: about 0.7 a second sustained (or six in
// a burst) get a 502 with Retry-After: 60 on every page until it lapses. So
// loads are spaced out (comixLoadGap) once past a short burst.
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
	// groups lists unique scanlation groups found in FetchChapters
	groups []string
	// cachedURL and cachedChapters cache parsed chapters per series URL,
	// so changing the scanlator filter doesn't re-fetch the whole page list
	cachedURL      string
	cachedChapters map[string][]*ComixChapter
	cachedGroups   []string
	// loadMu guards loads and lastLoad: how many browser loads were made,
	// and when the previous one finished
	loadMu   sync.Mutex
	loads    int
	lastLoad time.Time
}

func NewComix(g *Grabber) *Comix {
	return &Comix{Grabber: g}
}

// ComixChapter represents a Comix Chapter
type ComixChapter struct {
	Chapter
	// URL is the chapter reader page URL
	URL string
	// ScanlatorGroup is the scanlation group that uploaded this chapter
	ScanlatorGroup string
}

// comixHostRe matches comix.to and its subdomains
var comixHostRe = regexp.MustCompile(`(?i)(^|\.)comix\.to$`)

// its chapter API serves headless browsers a captcha of its own, every time
func init() { browser.RequireVisible("comix.to") }

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

// comixSleep is time.Sleep, replaced in tests
var comixSleep = time.Sleep

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

// load runs get (a browser load), spacing it from the previous one once
// the first comixLoadBurst loads are out of the way. A 429/5xx is waited out
// and retried by the browser package.
func (c *Comix) load(get func() (string, error)) (string, error) {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	if c.loads >= comixLoadBurst {
		comixSleep(comixLoadGap - time.Since(c.lastLoad))
	}
	html, err := get()
	c.loads++
	c.lastLoad = time.Now()

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
// been seen. Groups are parsed from each chapter row. Without a --scanlator
// selection, duplicate chapter numbers are collapsed keeping the first one
// seen (deterministically, per group and order). With --scanlator set, only
// that group's uploads are returned.
//
// Results are cached per series URL so a scanlator change in the app only
// re-filters cached chapters instead of re-walking the browser.
func (c *Comix) FetchChapters() (chapters Filterables, errs []error) {
	c.resolveSeriesURL()

	// Check cache: if same URL, reuse parsed chapters and only re-filter
	if c.cachedURL == c.URL && c.cachedChapters != nil {
		return c.applyScanlatorFilter(c.cachedChapters, c.cachedGroups)
	}

	// Fresh fetch: walk the chapter list
	// allChapters holds every chapter found per group
	allChapters := map[string][]*ComixChapter{}
	// seenGroups tracks groups found, in order
	var seenGroups []string
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

			group := ch.ScanlatorGroup

			// Track unique groups in order seen
			var found bool
			for _, g := range seenGroups {
				if g == group {
					found = true
					break
				}
			}
			if !found {
				seenGroups = append(seenGroups, group)
			}

			allChapters[group] = append(allChapters[group], ch)
		})

		shown, total, ok := comixParsePaging(doc.Find(comixFooterSelector).Text())
		if !ok || shown >= total {
			break
		}
		page++
	}

	// Cache for future filter-only changes, unless the walk was cut short:
	// a later call must try again rather than serve a truncated list
	if len(errs) == 0 {
		c.cachedURL = c.URL
		c.cachedChapters = allChapters
		c.cachedGroups = seenGroups
	}

	// Update filter options
	var uniqueGroups []string
	for _, g := range seenGroups {
		if g != "-" {
			uniqueGroups = append(uniqueGroups, g)
		}
	}
	c.groups = uniqueGroups

	// the walk's own errors (a page that never loaded) come along
	chapters, filterErrs := c.applyScanlatorFilter(allChapters, seenGroups)
	return chapters, append(errs, filterErrs...)
}

// applyScanlatorFilter applies the scanlator selection to cached chapters,
// returning filtered chapters with appropriate deduplication and tagging.
func (c *Comix) applyScanlatorFilter(allChapters map[string][]*ComixChapter, seenGroups []string) (chapters Filterables, errs []error) {
	selection := strings.TrimSpace(c.Settings.Scanlator)

	// Apply scanlator filter, or return error if invalid
	selectedGroups, selectErr := selectScanlators(selection, seenGroups, allChapters)
	if selectErr != nil {
		errs = append(errs, selectErr)
		return chapters, errs
	}

	// Only tag group names when explicitly selecting "all", not for default
	// no-filter behavior which should dedupe to the first group
	tagGroups := strings.EqualFold(selection, "all")
	seen := map[float64]bool{}

	for _, group := range selectedGroups {
		for _, ch := range allChapters[group] {
			// Clone the chapter to avoid modifying cached copies
			chCopy := *ch
			if tagGroups {
				// Keep all chapters, with group name appended to avoid collisions
				if chCopy.ScanlatorGroup != "-" {
					chCopy.Title += " [" + chCopy.ScanlatorGroup + "]"
				}
				chapters = append(chapters, &chCopy)
			} else {
				// Single group or no filter or ungrouped-only: dedupe by chapter number
				if !seen[chCopy.Number] {
					chapters = append(chapters, &chCopy)
					seen[chCopy.Number] = true
				}
			}
		}
	}

	return chapters, errs
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

// ScanlatorOptions returns the available scanlation groups for this series
func (c *Comix) ScanlatorOptions() FilterOptions {
	return FilterOptions{
		Values: c.groups,
		All:    true,
	}
}

// Refilterable implements grabber.Refilterable: FetchChapters re-applies the
// scanlator filter to its cached chapter map without any browser fetch.
func (*Comix) Refilterable() {}

// selectScanlators applies the --scanlator filter to the available groups,
// returning the ordered list of groups to download, or an error if the
// selection doesn't match any group.
func selectScanlators(selection string, allGroups []string, allChapters map[string][]*ComixChapter) ([]string, error) {
	selection = strings.TrimSpace(selection)

	if selection == "" {
		// No filter: return all groups in order, but ungrouped chapters ("-")
		// are returned first for determinism and to match the user's expectation
		// of "first come first served"
		var result []string
		if _, ok := allChapters["-"]; ok {
			result = append(result, "-")
		}
		for _, g := range allGroups {
			if g != "-" {
				result = append(result, g)
			}
		}
		return result, nil
	}

	if strings.EqualFold(selection, "all") {
		return allGroups, nil
	}

	// Exact match on group name or "-" for ungrouped
	for _, g := range allGroups {
		if strings.EqualFold(g, selection) || g == selection {
			return []string{g}, nil
		}
	}

	// Not found: build error with available groups
	available := make([]string, len(allGroups))
	for i, g := range allGroups {
		available[i] = g
	}
	return nil, fmt.Errorf(
		"no scanlation group %q for this series, available: %s",
		selection, strings.Join(available, ", "),
	)
}

// comixParseRow extracts a chapter entry from a rendered chapter list row.
// The scanlation group (`.mchap-row__secondary a.mchap-row__group span`) defaults to "-" if empty.
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

	// Extract scanlation group from the row; empty means ungrouped
	group := strings.TrimSpace(row.Find(".mchap-row__secondary a.mchap-row__group span").Text())
	if group == "" {
		group = "-"
	}

	return &ComixChapter{
		Chapter{
			Number: number,
			Title:  "Chapter " + strconv.FormatFloat(number, 'f', -1, 64),
		},
		href,
		group,
	}, nil
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
