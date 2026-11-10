// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Pornhwareader is a grabber for pornhwareader.com, an adult English reader of
// PornhwaDB. The series page is server rendered: its chapter list is a
// paginated set of links (?page=N, 100 chapters a page) that plain HTTP gets
// in full. The images aren't in the DOM, though: each chapter page carries its
// ordered "pages" array in the Next.js RSC flight payload.
type Pornhwareader struct {
	*Grabber
}

// NewPornhwareader returns a new Pornhwareader grabber
func NewPornhwareader(g *Grabber) *Pornhwareader {
	return &Pornhwareader{Grabber: g}
}

// pornhwareaderDomain is the site's host, matched without a fetch
const pornhwareaderDomain = "pornhwareader.com"

// pornhwareaderChapterRe matches a series' chapter link path and captures the
// series path (/series/{id}/{slug}) and the chapter number
var pornhwareaderChapterRe = regexp.MustCompile(`^(/series/\d+/[^/]+)/chapter-(\d+(?:\.\d+)?)$`)

// PornhwareaderChapter represents a Pornhwareader chapter
type PornhwareaderChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a pornhwareader.com URL
func (p *Pornhwareader) Test() (bool, error) {
	return hostMatchesDomain(p.URL, pornhwareaderDomain), nil
}

// FetchTitle fetches and returns the manga title
func (p *Pornhwareader) FetchTitle() (string, error) {
	p.resolveSeriesURL()
	doc, err := p.fetchDocument(p.seriesURL(1))
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find(`meta[property="og:title"]`).AttrOr("content", "")), nil
}

// FetchChapters returns the chapters of the manga, walking the chapter list's
// pages until one adds no chapter
func (p *Pornhwareader) FetchChapters() (chapters Filterables, errs []error) {
	p.resolveSeriesURL()
	seen := map[float64]bool{}
	for page := 1; ; page++ {
		doc, err := p.fetchDocument(p.seriesURL(page))
		if err != nil {
			return nil, []error{err}
		}

		added := 0
		doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			uri, err := url.Parse(href)
			if err != nil {
				return
			}
			matches := pornhwareaderChapterRe.FindStringSubmatch(uri.Path)
			if matches == nil || matches[1] != p.seriesPath() {
				return
			}
			number, err := strconv.ParseFloat(matches[2], 64)
			if err != nil || seen[number] {
				return
			}
			seen[number] = true
			added++

			chapterURL := p.BaseUrl() + matches[0]
			chapters = append(chapters, &PornhwareaderChapter{
				Chapter{
					Number: number,
					ID:     chapterURLID(chapterURL),
					Label:  Label{Style: LabelNumbered},
				},
				chapterURL,
			})
		})

		if added == 0 {
			break
		}
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (p *Pornhwareader) FetchChapter(f Filterable) (*Chapter, error) {
	pchap := f.(*PornhwareaderChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     pchap.URL,
		Referer: p.seriesURL(1),
	})
	if err != nil {
		return nil, err
	}

	pages, err := pornhwareaderPages(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         pchap.ID,
	}
	for i, page := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    page.URL,
		})
	}

	return chapter, nil
}

// resolveSeriesURL turns a chapter link into its series' URL, so a pasted
// chapter loads the whole list
func (p *Pornhwareader) resolveSeriesURL() {
	uri, err := url.Parse(p.URL)
	if err != nil {
		return
	}
	if matches := pornhwareaderChapterRe.FindStringSubmatch(uri.Path); matches != nil {
		p.URL = p.BaseUrl() + matches[1]
	}
}

// seriesPath returns the series' path (/series/{id}/{slug}) of the URL
func (p *Pornhwareader) seriesPath() string {
	uri, err := url.Parse(p.URL)
	if err != nil {
		return ""
	}

	return strings.TrimSuffix(uri.Path, "/")
}

// seriesURL returns the given page of the series' chapter list (page 1 is the
// bare series URL)
func (p *Pornhwareader) seriesURL(page int) string {
	uri := p.BaseUrl() + p.seriesPath()
	if page > 1 {
		uri += "?page=" + strconv.Itoa(page)
	}

	return uri
}

// fetchDocument fetches a page and parses it
func (p *Pornhwareader) fetchDocument(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// pornhwareaderPage is one image of a chapter, as embedded in its RSC payload
type pornhwareaderPage struct {
	Ordinal int    `json:"ordinal"`
	URL     string `json:"url"`
}

// pornhwareaderPages extracts the ordered page list out of a chapter page's
// RSC flight payload
func pornhwareaderPages(html string) ([]pornhwareaderPage, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	// anchored on the array: a bare "pages": also matches the chapter list's
	// page counts, which come later in the stream
	idx := strings.Index(stream, `"pages":[`)
	if idx == -1 {
		return nil, fmt.Errorf("no page list found in the page")
	}
	raw, err := extractBalancedJSON(stream[idx:], `"pages":`)
	if err != nil {
		return nil, err
	}

	var pages []pornhwareaderPage
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Ordinal < pages[j].Ordinal
	})

	return pages, nil
}
