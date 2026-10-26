// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// pizzaReaderMarker is the inline script every PizzaReader page carries, which
// sets the API base the reader's app calls
const pizzaReaderMarker = "API_BASE_URL = BASE_URL + 'api'"

// PizzaReader is a grabber for sites running the PizzaReader CMS, whose Vue
// SPA reads series and chapters from an open JSON api under /api
type PizzaReader struct {
	*Grabber
	title string
	feed  *pizzaReaderFeed
}

// NewPizzaReaderByMarkup creates a PizzaReader grabber matching any host whose
// page carries the PizzaReader markup
func NewPizzaReaderByMarkup(g *Grabber) *PizzaReader {
	return &PizzaReader{Grabber: g}
}

// PizzaReaderChapter is a chapter listed by a PizzaReader site
type PizzaReaderChapter struct {
	Chapter
	// Path is the reader path (/read/{slug}/{lang}/...); its pages are at /api + Path
	Path string
}

// Test returns true if the page carries the PizzaReader markup (one GET, shared
// with the other markup testers)
func (p *PizzaReader) Test() (bool, error) {
	return pageHasMarkup(p.Grabber, p.URL, pizzaReaderMarker), nil
}

// pizzaReaderReadRe matches a chapter URL's path, /read/{slug}/...
var pizzaReaderReadRe = regexp.MustCompile(`(?i)^/read/([^/]+)`)

// resolveSeriesURL rewrites a chapter URL (/read/{slug}/{lang}/...) to the
// series URL (/comics/{slug}). No-op for series URLs.
func (p *PizzaReader) resolveSeriesURL() {
	u, err := url.Parse(p.URL)
	if err != nil {
		return
	}
	m := pizzaReaderReadRe.FindStringSubmatch(u.Path)
	if m == nil {
		return
	}
	u.Path = "/comics/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	p.URL = u.String()
}

// pizzaReaderComicRe matches a series URL's path, /comics/{slug}
var pizzaReaderComicRe = regexp.MustCompile(`^/comics/([^/]+)`)

// fetchComic fetches and caches the series feed. Licensed chapters are listed
// only when asked for, as the reader asks, so they can be marked locked.
func (p *PizzaReader) fetchComic() (*pizzaReaderFeed, error) {
	if p.feed != nil {
		return p.feed, nil
	}

	p.resolveSeriesURL()
	u, err := url.Parse(p.URL)
	if err != nil {
		return nil, err
	}
	m := pizzaReaderComicRe.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, errSeriesNotFound(p.URL)
	}

	body, err := http.GetText(http.RequestParams{
		URL:     p.apiUrl("/comics/" + m[1] + "?licensed=true"),
		Referer: p.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := pizzaReaderFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	p.feed = &feed

	return p.feed, nil
}

// FetchTitle returns the manga title
func (p *PizzaReader) FetchTitle() (string, error) {
	feed, err := p.fetchComic()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(feed.Comic.Title), nil
}

// LanguageOptions lists the languages the series' chapters are in
func (p *PizzaReader) LanguageOptions() FilterOptions {
	feed, err := p.fetchComic()
	if err != nil {
		return FilterOptions{}
	}

	var languages []string
	for _, c := range feed.Comic.Chapters {
		languages = append(languages, c.Language)
	}

	return FilterOptions{
		Values: uniqueValues(languages),
		All:    true,
	}
}

// FetchChapters returns the chapters of the manga
func (p *PizzaReader) FetchChapters() (Filterables, []error) {
	feed, err := p.fetchComic()
	if err != nil {
		return nil, []error{err}
	}

	var chapters Filterables
	for _, c := range feed.Comic.Chapters {
		if p.Settings.Language != "" && p.Settings.Language != FilterAll && c.Language != p.Settings.Language {
			continue
		}

		number, numbered := pizzaReaderNumber(c)
		label := Label{Style: LabelNumbered}
		if !numbered {
			label = Label{Style: LabelUnnumbered}
		}

		chap := &PizzaReaderChapter{
			Chapter: Chapter{
				Title:    sanitizeTitle(c.Title),
				Number:   number,
				Language: c.Language,
				ID:       c.URL,
				Volume:   c.Volume,
				Locked:   c.Licensed != 0,
				Label:    label,
			},
			Path: c.URL,
		}
		chap.SetGroups(teamNames(c.Teams)...)
		chapters = append(chapters, chap)
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (p *PizzaReader) FetchChapter(f Filterable) (*Chapter, error) {
	pchap := f.(*PizzaReaderChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     p.apiUrl(pchap.Path),
		Referer: p.BaseUrl() + pchap.Path,
	})
	if err != nil {
		return nil, err
	}

	feed := pizzaReaderReaderFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	if len(feed.Chapter.Pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterName(f))
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		Language:   pchap.Language,
		ID:         pchap.ID,
		Volume:     pchap.Volume,
		PagesCount: int64(len(feed.Chapter.Pages)),
	}
	chapter.SetGroups(pchap.GetGroups()...)
	for i, src := range feed.Chapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	}

	return chapter, nil
}

// apiUrl returns the absolute api url for a site-relative path
// (e.g. "/comics/frieren" -> "https://example.com/api/comics/frieren")
func (p *PizzaReader) apiUrl(path string) string {
	return p.BaseUrl() + "/api" + path
}

// pizzaReaderNumber returns the chapter's number, with a subchapter as its
// decimal (257 and 5 is 257.5), or false for a row with no chapter number
func pizzaReaderNumber(c pizzaReaderChapter) (float64, bool) {
	if c.Chapter == nil {
		return 0, false
	}
	number := float64(*c.Chapter)
	if c.Subchapter != nil {
		if n, err := strconv.ParseFloat(fmt.Sprintf("%d.%d", *c.Chapter, *c.Subchapter), 64); err == nil {
			number = n
		}
	}

	return number, true
}

// pizzaReaderFeed is the JSON feed for the series details
type pizzaReaderFeed struct {
	Comic struct {
		Title    string               `json:"title"`
		Chapters []pizzaReaderChapter `json:"chapters"`
	} `json:"comic"`
}

// pizzaReaderChapter is a chapter row of the series feed
type pizzaReaderChapter struct {
	Title      string `json:"title"`
	Volume     int    `json:"volume"`
	Chapter    *int64 `json:"chapter"`
	Subchapter *int64 `json:"subchapter"`
	Language   string `json:"language"`
	URL        string `json:"url"`
	Licensed   int    `json:"licensed"`
	// Teams are the groups that made the chapter; the list can hold nulls
	Teams []*feedTeam `json:"teams"`
}

// pizzaReaderReaderFeed is the JSON feed for a chapter's pages
type pizzaReaderReaderFeed struct {
	Chapter struct {
		Pages []string `json:"pages"`
	} `json:"chapter"`
}
