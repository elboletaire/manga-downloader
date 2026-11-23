// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// meowingSites lists the newer Meowing theme's hosts (Luna Toons). Add a domain
// here plus makefile and README to support a new site.
var meowingSites = []string{
	"lunatoons.net",
}

// meowingMarker is the JSON block every page of the newer theme carries, series
// and reader pages alike; the older theme (genztoons, writerscans) has none
const meowingMarker = `id="series-offline-data"`

// meowingDataRe captures the JSON block of a page
var meowingDataRe = regexp.MustCompile(`(?s)<script id="series-offline-data"[^>]*>(.*?)</script>`)

// meowingChapterPathRe matches a reader path, /{slug}-chapter-{n}
var meowingChapterPathRe = regexp.MustCompile(`(?i)^/[^/]+-chapter-[^/]+$`)

// Meowing is a grabber for the meowing.org platform's newer theme. The series
// page renders an empty chapter list that its script fills from a fragment
// endpoint, and the reader's pages are plain <img> tags.
type Meowing struct {
	*Grabber
	// byMarkup also matches the hosts not in meowingSites (NewMeowingByMarkup)
	byMarkup bool
	series   *meowingSeries
}

// meowingSeries is what the series page gives: its title, and the uid and slug
// its chapter list is requested with
type meowingSeries struct {
	title string
	uid   string
	slug  string
}

// NewMeowing creates a new Meowing grabber, matching the listed hosts
func NewMeowing(g *Grabber) *Meowing {
	return &Meowing{Grabber: g}
}

// NewMeowingByMarkup returns a Meowing grabber that also matches a page on any
// host carrying the newer theme's markup
func NewMeowingByMarkup(g *Grabber) *Meowing {
	return &Meowing{Grabber: g, byMarkup: true}
}

// MeowingChapter is a chapter listed by the Meowing grabber
type MeowingChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a listed host (no fetch). The NewMeowingByMarkup
// instance also fetches any other URL once and checks its markup.
func (m *Meowing) Test() (bool, error) {
	for _, d := range meowingSites {
		if hostMatchesDomain(m.URL, d) {
			return true, nil
		}
	}
	if !m.byMarkup {
		return false, nil
	}
	return pageHasMarkup(m.Grabber, m.URL, meowingMarker), nil
}

// meowingData is the JSON block of a page: its series' slug and title
type meowingData struct {
	SeriesSlug  string `json:"seriesSlug"`
	SeriesTitle string `json:"seriesTitle"`
}

// meowingDataOf decodes the JSON block of a page
func meowingDataOf(body string) (meowingData, error) {
	var data meowingData
	m := meowingDataRe.FindStringSubmatch(body)
	if m == nil {
		return data, errors.New("no series data on the page")
	}
	err := json.Unmarshal([]byte(m[1]), &data)
	return data, err
}

// resolveSeriesURL rewrites a reader URL to its series URL, from the slug the
// reader's JSON block names. A series URL is left as it is, without a fetch.
func (m *Meowing) resolveSeriesURL() error {
	u, err := url.Parse(m.URL)
	if err != nil || !meowingChapterPathRe.MatchString(u.Path) {
		return nil
	}

	body, err := http.GetText(http.RequestParams{URL: m.URL, Referer: m.BaseUrl() + "/"})
	if err != nil {
		return fmt.Errorf("meowing reader %s: %w", m.URL, err)
	}
	data, err := meowingDataOf(body)
	if err != nil || data.SeriesSlug == "" {
		return errSeriesNotFound(m.URL)
	}

	m.URL = m.BaseUrl() + "/" + data.SeriesSlug
	return nil
}

// fetchSeries fetches the series page once, for the title and the uid and slug
// its chapter list is requested with
func (m *Meowing) fetchSeries() error {
	if m.series != nil {
		return nil
	}
	if err := m.resolveSeriesURL(); err != nil {
		return err
	}

	body, err := http.GetText(http.RequestParams{URL: m.URL, Referer: m.BaseUrl() + "/"})
	if err != nil {
		return fmt.Errorf("meowing series %s: %w", m.URL, err)
	}
	data, err := meowingDataOf(body)
	if err != nil {
		return fmt.Errorf("meowing series %s: %w", m.URL, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return err
	}

	title := data.SeriesTitle
	if title == "" {
		title = doc.Find("h1").First().Text()
	}
	uid := doc.Find("#chapters-list").AttrOr("data-series-uid", "")
	if uid == "" {
		return fmt.Errorf("meowing series %s: no chapter list on the page", m.URL)
	}

	slug := data.SeriesSlug
	if slug == "" {
		slug = doc.Find("#chapters-list").AttrOr("data-series-slug", "")
	}

	m.series = &meowingSeries{
		title: sanitizeTitle(title),
		uid:   uid,
		slug:  slug,
	}
	return nil
}

// FetchTitle returns the manga title
func (m *Meowing) FetchTitle() (string, error) {
	if err := m.fetchSeries(); err != nil {
		return "", err
	}
	return m.series.title, nil
}

// meowingChapterList is the chapter list fragment, whose rows are HTML
type meowingChapterList struct {
	RowsHTML string `json:"rows_html"`
}

// FetchChapters returns the chapters of the manga. Locked rows are listed too,
// marked Locked, so they can be left out before anything is downloaded.
func (m *Meowing) FetchChapters() (Filterables, []error) {
	if err := m.fetchSeries(); err != nil {
		return nil, []error{err}
	}

	query := url.Values{
		"_chapters_html": {"1"},
		"series_uid":     {m.series.uid},
		"series_slug":    {m.series.slug},
	}
	body, err := http.GetText(http.RequestParams{
		URL:     m.BaseUrl() + "/?" + query.Encode(),
		Referer: m.URL,
	})
	if err != nil {
		return nil, []error{fmt.Errorf("meowing chapters %s: %w", m.URL, err)}
	}

	var list meowingChapterList
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil, []error{fmt.Errorf("meowing chapters %s: %w", m.URL, err)}
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(list.RowsHTML))
	if err != nil {
		return nil, []error{err}
	}

	base, err := url.Parse(m.BaseUrl())
	if err != nil {
		return nil, []error{err}
	}

	var chapters Filterables
	var errs []error
	doc.Find("a.chapter-row").Each(func(_ int, row *goquery.Selection) {
		label := row.AttrOr("data-ch-label", "")
		number, ok := parseChapterNumber(label)
		if !ok {
			errs = append(errs, fmt.Errorf("meowing chapters %s: no chapter number in %q", m.URL, label))
			return
		}

		href := row.AttrOr("href", "")
		if href == "" {
			href = row.AttrOr("data-ch-url", "")
		}
		ref, err := url.Parse(href)
		if err != nil {
			errs = append(errs, fmt.Errorf("meowing chapters %s: bad link %q: %w", m.URL, href, err))
			return
		}

		chapters = append(chapters, &MeowingChapter{
			Chapter: Chapter{
				Number:   number,
				Language: "en",
				ID:       row.AttrOr("data-ch-uid", ""),
				Label:    Label{Style: LabelNumbered},
				Locked:   row.AttrOr("data-ch-locked", "") == "1",
			},
			URL: base.ResolveReference(ref).String(),
		})
	})

	return chapters, errs
}

// FetchChapter fetches a chapter and returns its pages
func (m *Meowing) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MeowingChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     mchap.URL,
		Referer: m.URL,
	})
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
		Label:    labelOf(f),
		Language: mchap.Language,
		ID:       mchap.ID,
	}
	doc.Find("#strip-reader img.reader-page").Each(func(_ int, img *goquery.Selection) {
		src := img.AttrOr("src", "")
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: src})
	})
	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterName(f))
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
