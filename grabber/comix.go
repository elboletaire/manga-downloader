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
// The reader page virtualizes/lazily-mounts its page images via an
// IntersectionObserver, so a plain render only ever captures a handful of
// pages near the top; browser.GetHTMLWithScroll progressively scrolls the
// page so the site's own JS mounts (and decrypts) the rest before the final
// HTML snapshot. Every 10th "page" turns out to be an ad slot with no image
// container at all (confirmed by comparing the reader's own page counter,
// e.g. "137", against the real image count after scrolling, which
// consistently landed at 137 minus one skipped slot per 10 — those aren't
// missed content, there's nothing to scrape there.
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

// comixPagingRe extracts the "showing X to Y of Z items" chapter-list footer
// text, used to know when pagination is exhausted
var comixPagingRe = regexp.MustCompile(`(\d+)\s+to\s+(\d+)\s+of\s+(\d+)`)

const (
	comixChapterRowSelector = ".mchap-item"
	comixFooterSelector     = ".mchap-foot__hint"
	comixImageSelector      = ".rpage-page__img"
	// scrolling the reader page in fractional steps of the (growing)
	// document height reliably mounts every real page regardless of the
	// chapter's actual length, since each step re-reads the current
	// scrollHeight rather than assuming a fixed page size upfront
	comixReaderScrollIterations = 35
	comixReaderScrollPause      = 600 * time.Millisecond
)

// Test returns true if the URL is a comix.to URL. It only checks the
// hostname (no fetch) so it can be tried early, before starting a browser.
func (c *Comix) Test() (bool, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return false, nil
	}
	return comixHostRe.MatchString(u.Hostname()), nil
}

// fetchPage1 renders (and caches) the first page of the series' chapter list
func (c *Comix) fetchPage1() (*goquery.Document, error) {
	if c.doc != nil {
		return c.doc, nil
	}

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

// FetchChapter renders the chapter reader page in a real browser, scrolling
// it so every lazily-mounted page image gets decrypted and inserted into the
// DOM before reading it back out
func (c *Comix) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*ComixChapter)

	html, err := browser.GetHTMLWithScroll(
		chap.URL, comixImageSelector, comixReaderScrollIterations, comixReaderScrollPause, 0,
	)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	doc.Find(comixImageSelector).Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", s.AttrOr("data-src", "")))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
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
