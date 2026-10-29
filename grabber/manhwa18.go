// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Manhwa18 is a grabber for manhwa18.net, an Inertia.js site: the series and reader
// pages carry their chapters and page images as JSON in the `#app` root's data-page attribute.
type Manhwa18 struct {
	*Grabber
	series *manhwa18Page
}

func NewManhwa18(g *Grabber) *Manhwa18 {
	return &Manhwa18{Grabber: g}
}

// Manhwa18Chapter represents a Manhwa18 chapter
type Manhwa18Chapter struct {
	Chapter
	URL string
}

// manhwa18SeriesRe is the URL shape of a series; a chapter URL extends it
var manhwa18SeriesRe = regexp.MustCompile(`^/manga/[^/]+`)

// Test returns true if the URL is a manhwa18.net URL
func (m *Manhwa18) Test() (bool, error) {
	return hostMatchesDomain(m.URL, "manhwa18.net"), nil
}

// resolveSeriesURL converts chapter URLs (/manga/{slug}/{chapter}) to series URLs (/manga/{slug})
func (m *Manhwa18) resolveSeriesURL() error {
	uri, err := url.Parse(m.URL)
	if err != nil {
		return err
	}

	if series := manhwa18SeriesRe.FindString(uri.Path); series != "" && series != uri.Path {
		uri.Path = series
		uri.RawQuery = ""
		uri.Fragment = ""
		m.URL = uri.String()
	}

	return nil
}

// FetchTitle fetches and returns the manga title
func (m *Manhwa18) FetchTitle() (string, error) {
	series, err := m.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Props.Manga.Name), nil
}

// FetchChapters returns the chapters of the manga
func (m *Manhwa18) FetchChapters() (chapters Filterables, errs []error) {
	series, err := m.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range series.Props.Chapters {
		number, ok := parseChapterNumber(c.Name)
		if !ok {
			continue
		}
		chapters = append(chapters, &Manhwa18Chapter{
			Chapter{
				Number: number,
				ID:     c.Slug,
				Label:  Label{Style: LabelNumbered},
			},
			m.chapterURL(series.Props.Manga.Slug, c.Slug),
		})
	}

	return chapters, errs
}

// FetchChapter fetches a chapter and its pages
func (m *Manhwa18) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*Manhwa18Chapter)

	page, err := m.fetchInertia(mchap.URL)
	if err != nil {
		return nil, err
	}

	// a chapter the reader hides (paid or not released) comes back with no images
	if len(page.Props.ChapterImages) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %q (it may be a paywalled chapter)", mchap.ID)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   "en",
		ID:         mchap.ID,
		PagesCount: int64(len(page.Props.ChapterImages)),
	}
	for i, image := range page.Props.ChapterImages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    image.Src,
		})
	}

	return chapter, nil
}

// chapterURL returns the reader URL of a chapter of the series
func (m *Manhwa18) chapterURL(seriesSlug, chapterSlug string) string {
	uri, _ := url.JoinPath(m.BaseUrl(), "manga", seriesSlug, chapterSlug)
	return uri
}

// fetchSeries fetches and caches the series page's data
func (m *Manhwa18) fetchSeries() (*manhwa18Page, error) {
	if m.series != nil {
		return m.series, nil
	}

	if err := m.resolveSeriesURL(); err != nil {
		return nil, err
	}

	series, err := m.fetchInertia(m.URL)
	if err != nil {
		return nil, err
	}
	if series.Props.Manga.Slug == "" {
		return nil, errors.New("the page has no manga data (is the URL a series page?)")
	}

	m.series = series

	return series, nil
}

// fetchInertia fetches a page and decodes the JSON in its `#app` root's data-page attribute
func (m *Manhwa18) fetchInertia(uri string) (*manhwa18Page, error) {
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	raw, ok := doc.Find("#app").Attr("data-page")
	if !ok || raw == "" {
		return nil, fmt.Errorf("could not find the page data in %s", uri)
	}

	page := &manhwa18Page{}
	if err := json.Unmarshal([]byte(raw), page); err != nil {
		return nil, err
	}

	return page, nil
}

// manhwa18Page is the subset of an Inertia page's data-page JSON that the
// series page (manga, chapters) and the reader page (chapterImages) share
type manhwa18Page struct {
	Props struct {
		Manga struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"manga"`
		Chapters []struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"chapters"`
		ChapterImages []struct {
			Src string `json:"src"`
		} `json:"chapterImages"`
	} `json:"props"`
}
