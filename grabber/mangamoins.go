// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangamoins is a grabber for mangamoins.com, behind a Turnstile only a visible
// browser passes. Pages come from the scan API over plain HTTP, with the
// clearance the browser got.
type Mangamoins struct {
	*Grabber
	series   *mangamoinsSeries
	resolved bool
}

// headless never passes its Cloudflare check, so skip the doomed probe
func init() {
	browser.RequireVisible("mangamoins.com")
}

// NewMangamoins returns a new Mangamoins grabber
func NewMangamoins(g *Grabber) *Mangamoins {
	return &Mangamoins{Grabber: g}
}

// MangamoinsChapter represents a mangamoins.com chapter
type MangamoinsChapter struct {
	Chapter
	// Slug is the chapter's own identifier on the site, what the scan API is asked for
	Slug string
}

// mangamoinsSeries is what the series page gives: the title and the chapter list
type mangamoinsSeries struct {
	title    string
	chapters []mangamoinsListed
}

// mangamoinsListed is one row of the series page's chapter list
type mangamoinsListed struct {
	slug   string
	number string // as the site writes it, "172.2" for a decimal
	title  string
}

// mangamoinsScan is what the scan API answers for a chapter slug
type mangamoinsScan struct {
	PageNumbers  int    `json:"pageNumbers"`
	PagesBaseURL string `json:"pagesBaseUrl"`
	Manga        string `json:"manga"`
}

// mangamoinsScanRe matches a reader path (/scan/{slug}) and captures the slug
var mangamoinsScanRe = regexp.MustCompile(`(?i)^/scan/([^/?#]+)/?$`)

// Test returns true if the URL is a mangamoins.com URL. It only checks the
// hostname (no fetch), since starting a browser is expensive.
func (m *Mangamoins) Test() (bool, error) {
	return hostMatchesDomain(m.URL, "mangamoins.com"), nil
}

// mangamoinsScanSlug returns the chapter slug of a reader URL
func mangamoinsScanSlug(rawURL string) (string, bool) {
	host, path, ok := chapterPath(rawURL)
	if !ok || host != "mangamoins.com" {
		return "", false
	}

	m := mangamoinsScanRe.FindStringSubmatch(path)
	if m == nil {
		return "", false
	}

	return m[1], true
}

// resolveSeriesURL rewrites a reader URL to its series URL, once. The reader
// is rendered in the browser first, which is what gets the session its clearance.
func (m *Mangamoins) resolveSeriesURL() error {
	if m.resolved {
		return nil
	}

	slug, ok := mangamoinsScanSlug(m.URL)
	if !ok {
		m.resolved = true
		return nil
	}

	if _, err := browser.GetHTML(m.URL, "img.reader-manga-page", 0); err != nil {
		return err
	}

	scan, err := m.fetchScan(slug)
	if err != nil || scan.Manga == "" {
		return errSeriesNotFound(m.URL)
	}

	m.URL = m.BaseUrl() + "/manga/" + scan.Manga
	m.resolved = true

	return nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangamoins) FetchTitle() (string, error) {
	if err := m.resolveSeriesURL(); err != nil {
		return "", err
	}

	series, err := m.fetchSeries()
	if err != nil {
		return "", err
	}

	return series.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangamoins) FetchChapters() (Filterables, []error) {
	if err := m.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	series, err := m.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(series.chapters))
	for _, c := range series.chapters {
		number, err := strconv.ParseFloat(c.number, 64)
		if err != nil {
			continue
		}

		chapters = append(chapters, &MangamoinsChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    sanitizeTitle(c.title),
				Language: "fr",
				ID:       c.slug,
				Label:    Label{Style: LabelNumbered, Number: c.number, Lang: "fr"},
			},
			Slug: c.slug,
		})
	}

	return chapters, nil
}

// FetchChapter fetches the scan of a chapter and returns its pages
func (m *Mangamoins) FetchChapter(f Filterable) (*Chapter, error) {
	mchap, ok := f.(*MangamoinsChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	scan, err := m.fetchScan(mchap.Slug)
	if err != nil {
		// only a challenge (403, or an HTML page where JSON belongs) means the
		// clearance expired; anything else, like an unknown slug, fails now
		if !mangamoinsChallenged(err) {
			return nil, err
		}
		// the session's clearance may have expired: render the reader again
		if _, err := browser.GetHTML(m.BaseUrl()+"/scan/"+mchap.Slug, "img.reader-manga-page", 0); err != nil {
			return nil, err
		}
		if scan, err = m.fetchScan(mchap.Slug); err != nil {
			return nil, err
		}
	}

	pages, err := mangamoinsPages(scan)
	if err != nil {
		return nil, err
	}

	// the listed title and label stay: the API's own number drops decimals
	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		Language:   "fr",
		ID:         mchap.ID,
		Pages:      pages,
		PagesCount: int64(len(pages)),
	}, nil
}

// fetchSeries fetches (and caches) the series page, reading its title and chapter list
func (m *Mangamoins) fetchSeries() (*mangamoinsSeries, error) {
	if m.series != nil {
		return m.series, nil
	}

	// the wait selector is the chapter rows: the challenge page has an h1 too
	body, err := browser.GetHTML(m.URL, "a.manga-detail__chapter", 0)
	if err != nil {
		return nil, err
	}

	series, err := parseMangamoinsSeries(body)
	if err != nil {
		return nil, err
	}

	m.series = series

	return m.series, nil
}

// parseMangamoinsSeries reads the title and the chapter rows of a series page
func parseMangamoinsSeries(body string) (*mangamoinsSeries, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	series := &mangamoinsSeries{
		title: sanitizeTitle(doc.Find("h1#manga-title").First().Text()),
	}

	doc.Find("a.manga-detail__chapter").Each(func(_ int, row *goquery.Selection) {
		// the rows link relatively (/scan/OP01)
		href, _ := row.Attr("href")
		m := mangamoinsScanRe.FindStringSubmatch(href)
		if m == nil {
			return
		}

		series.chapters = append(series.chapters, mangamoinsListed{
			slug:   m[1],
			number: strings.TrimSpace(row.Find("span.manga-detail__chapter-num").First().Text()),
			title:  strings.TrimSpace(row.Find("span.manga-detail__chapter-title").First().Text()),
		})
	})

	if len(series.chapters) == 0 {
		return nil, errors.New("no chapters found on the series page")
	}

	return series, nil
}

// fetchScan asks the scan API for a chapter. The API is plain HTTP, but only
// with the clearance the browser got for the site.
func (m *Mangamoins) fetchScan(slug string) (*mangamoinsScan, error) {
	body, err := http.GetText(http.RequestParams{
		URL:     m.BaseUrl() + "/api/v1/scan?slug=" + url.QueryEscape(slug),
		Referer: m.BaseUrl() + "/scan/" + slug,
		Headers: map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, err
	}

	var scan mangamoinsScan
	if err := json.Unmarshal([]byte(body), &scan); err != nil {
		return nil, err
	}

	return &scan, nil
}

// mangamoinsChallenged reports whether a scan request failed the way an expired
// clearance does: a 403, or an HTML challenge served where the JSON belongs
func mangamoinsChallenged(err error) bool {
	var statusErr *http.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Code == 403
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

// mangamoinsPages builds the page list of a scan: the pages are named by their
// number, two digits at least (01.webp, 52.webp)
func mangamoinsPages(scan *mangamoinsScan) ([]Page, error) {
	if scan.PageNumbers <= 0 {
		return nil, errors.New("no pages found for this chapter")
	}
	if scan.PagesBaseURL == "" {
		return nil, errors.New("the chapter has no pages base URL")
	}

	base := strings.TrimRight(scan.PagesBaseURL, "/") + "/"

	pages := make([]Page, 0, scan.PageNumbers)
	for i := 1; i <= scan.PageNumbers; i++ {
		pages = append(pages, Page{
			Number: int64(i),
			URL:    base + fmt.Sprintf("%02d.webp", i),
		})
	}

	return pages, nil
}
