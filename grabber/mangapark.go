// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangapark is a grabber for mangapark1.com, the current home of MangaPark.
// The older mangapark.to/.io/.page hosts redirect there or are dead, and their
// /series/{slug} URLs map to the same slug. Series pages live at
// /manga/{slug} and readers at /read/{slug}/chapter-N. The series page only
// server-renders its newest ~20 chapters, but every reader page carries a
// chapter dropdown with the complete list, so FetchChapters reads it from one
// reader page.
//
// The whole domain sits behind a Cloudflare JS challenge that the browser
// passes. The clearance it harvests carries to the page images on the CDN, but
// not to HTML requests over Go's HTTP client, which get a 403. So every HTML
// page (series and reader alike) is rendered through the browser, after one
// plain request that is skipped once the site has refused one. The images
// keep downloading over plain HTTP, which is what matters, since a chapter is
// ~200 of them.
type Mangapark struct {
	*Grabber
	title string
	slug  string
	// readerURL is a chapter reader page of the series, whose chapter
	// dropdown lists every chapter
	readerURL string
}

func NewMangapark(g *Grabber) *Mangapark {
	return &Mangapark{Grabber: g}
}

// MangaparkChapter represents a Mangapark Chapter
type MangaparkChapter struct {
	Chapter
	Slug string
}

// mangaparkBase is the origin every request goes to
const mangaparkBase = "https://mangapark1.com"

// mangaparkHostRe matches mangapark1.com and the hosts it replaced
var mangaparkHostRe = regexp.MustCompile(`mangapark(?:1\.com|\.(?:to|io|page))`)

// mangaparkPathRe matches the slug in a path: `/manga/{slug}` or
// `/read/{slug}/...` on the site, `/series/{slug}` on the old hosts, or a bare
// `/{slug}` (group 2), which is only accepted on the old hosts.
var mangaparkPathRe = regexp.MustCompile(`^/(?:manga|read|series)/([^/]+)|^/([^/]+)/?$`)

// mangaparkTitleRe extracts the series name out of the schema.org ComicSeries
// block every series page carries.
var mangaparkTitleRe = regexp.MustCompile(`(?s)"@type":\s*"ComicSeries".*?"name":\s*"([^"]*)"`)

// mangaparkSeriesWait is a selector that exists only on a rendered series page
// (its chapter links), never on a Cloudflare interstitial or the 404 page.
// Waiting on something the challenge page also has — `body`, say — would make
// the challenge look like a successful render, and the browser package keys
// its escalation to a visible window off exactly this selector timing out.
const mangaparkSeriesWait = `a[href*="/read/"]`

// mangaparkReaderWait is the equivalent for a reader page: its page images,
// which the server renders into the HTML.
const mangaparkReaderWait = `img.image-thumb[data-number]`

// mangaparkPlainBlocked is set once a plain HTTP request gets a 403, so the
// rest of the run goes straight to the browser instead of paying for a request
// the site refuses.
var mangaparkPlainBlocked atomic.Bool

// mangaparkSeriesURL returns the series page on mangapark1.com for any URL the
// grabber accepts: a series or chapter URL there, or an old-host URL.
func mangaparkSeriesURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	matches := mangaparkPathRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return "", fmt.Errorf("mangapark: could not find the series slug in %q", raw)
	}
	slug := matches[1]
	if slug == "" {
		if u.Hostname() == "mangapark1.com" {
			return "", fmt.Errorf("mangapark: could not find the series slug in %q", raw)
		}
		slug = matches[2]
	}

	return mangaparkBase + "/manga/" + slug, nil
}

// fetchMangaparkPage returns the HTML of a page on the site, rendered through
// the browser (see Mangapark). Plain HTTP is tried first, unless it was already
// refused, as it's all that's needed if the challenge is ever turned off.
func fetchMangaparkPage(pageURL, referer, wait string) (string, error) {
	if !mangaparkPlainBlocked.Load() {
		body, err := http.GetText(http.RequestParams{URL: pageURL, Referer: referer})
		if err == nil {
			return body, nil
		}

		var statusErr *http.StatusError
		if errors.As(err, &statusErr) && statusErr.Code == 403 {
			mangaparkPlainBlocked.Store(true)
		}
	}

	return browser.GetHTML(pageURL, wait, 0)
}

// Test returns true if the URL is a MangaPark URL
func (m *Mangapark) Test() (bool, error) {
	return mangaparkHostRe.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangapark) FetchTitle() (string, error) {
	if err := m.fetchSeriesData(); err != nil {
		return "", err
	}

	return m.title, nil
}

// FetchChapters returns the complete chapter list, read from the chapter
// dropdown of a reader page (the series page only lists its newest chapters).
func (m *Mangapark) FetchChapters() (Filterables, []error) {
	if err := m.fetchSeriesData(); err != nil {
		return nil, []error{err}
	}

	body, err := fetchMangaparkPage(m.readerURL, m.URL, mangaparkReaderWait)
	if err != nil {
		return nil, []error{err}
	}

	chapters, err := parseMangaparkDropdown(body)
	if err != nil {
		return nil, []error{err}
	}

	return chapters, nil
}

// parseMangaparkDropdown reads the chapter list out of a reader page's
// chapter dropdown. Each option's value is the chapter's reader URL, whose
// last segment is `chapter-N` (N may be decimal, like `chapter-23.5`).
func parseMangaparkDropdown(body string) (Filterables, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapters := Filterables{}
	var errs []error
	// the reader renders the dropdown twice (top and bottom bars), with the
	// same options: only the first one is the list
	doc.Find("select.chapter-dropdown").First().Find("option").Each(func(i int, s *goquery.Selection) {
		value := s.AttrOr("value", "")
		if value == "" {
			return
		}

		u, err := url.Parse(value)
		if err != nil {
			errs = append(errs, err)
			return
		}
		slug := path.Base(u.Path)
		number, err := strconv.ParseFloat(strings.TrimPrefix(slug, "chapter-"), 64)
		if !strings.HasPrefix(slug, "chapter-") || err != nil {
			errs = append(errs, fmt.Errorf("mangapark: unexpected chapter link %q", value))
			return
		}

		chapters = append(chapters, &MangaparkChapter{
			Chapter{
				Number: number,
				Title:  sanitizeTitle(s.Text()),
			},
			slug,
		})
	})
	if len(errs) > 0 {
		return nil, errs[0]
	}
	if len(chapters) == 0 {
		return nil, errors.New("mangapark: no chapters found in the reader page")
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Mangapark) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaparkChapter)

	uri, err := url.JoinPath(mangaparkBase, "read", m.slug, mchap.Slug)
	if err != nil {
		return nil, err
	}

	body, err := fetchMangaparkPage(uri, m.URL, mangaparkReaderWait)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	// every page image is a `.image-thumb` with a numeric `data-number`
	// attribute, holding the real URL in `data-src` (or `src`). The reader
	// also puts a `notice` banner in the same list, which is not a page
	doc.Find(`img.image-thumb[data-number]:not([data-number="notice"])`).Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("data-src", s.AttrOr("src", ""))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found in the chapter page")
	}

	return chapter, nil
}

// fetchSeriesData resolves the series page from the URL the user gave (a
// series, chapter or old-host URL), then caches its title, parsed out of the
// ComicSeries JSON-LD, and the reader URL of its first chapter. The chapter
// list is read from that reader page.
//
// The grabber's URL is replaced by the canonical series page, so every later
// request (the Referer, the chapter URLs) goes to mangapark1.com.
func (m *Mangapark) fetchSeriesData() error {
	if m.title != "" {
		return nil
	}

	seriesURL, err := mangaparkSeriesURL(m.URL)
	if err != nil {
		return err
	}
	m.URL = seriesURL
	m.slug = path.Base(seriesURL)

	html, err := fetchMangaparkPage(m.URL, "", mangaparkSeriesWait)
	if err != nil {
		return err
	}

	return m.parseSeriesData(html)
}

// parseSeriesData pulls the title and the reader URL out of a fetched series
// page. The reader URL is the first chapter link of this series, which the
// series page renders under the series slug.
func (m *Mangapark) parseSeriesData(body string) error {
	matches := mangaparkTitleRe.FindStringSubmatch(body)
	if len(matches) != 2 {
		return errors.New("could not find series data in the series page")
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return err
	}
	href, ok := doc.Find(fmt.Sprintf(`a[href*="/read/%s/"]`, m.slug)).First().Attr("href")
	if !ok {
		return errors.New("could not find a chapter in the series page")
	}

	m.title = sanitizeTitle(matches[1])
	m.readerURL = href

	return nil
}
