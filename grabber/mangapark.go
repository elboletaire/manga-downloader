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

// Mangapark is a grabber for mangapark.page (mangapark.to's content, served
// from a mirror domain: mangapark.to itself returned Cloudflare 522s at
// implementation time). The series page only server-renders the newest ~20
// chapters (parsed out of a `window.seriesData` JS blob), but its "Load All
// Chapters" button hits a plain JSON API (`/get-chapter-list?slug=...`) that
// returns the complete list with no browser needed. Reader pages are static
// HTML with every page image already in an `<img data-src>`.
type Mangapark struct {
	*Grabber
	title string
	slug  string
}

func NewMangapark(g *Grabber) *Mangapark {
	return &Mangapark{Grabber: g}
}

// MangaparkChapter represents a Mangapark Chapter
type MangaparkChapter struct {
	Chapter
	Slug string
}

// mangaparkSeriesDataRe extracts the title and slug fields out of the
// `window.seriesData = {...}` JS object embedded in the series page
var mangaparkSeriesDataRe = regexp.MustCompile(`(?s)window\.seriesData\s*=\s*\{.*?title:\s*"([^"]*)".*?slug:\s*"([^"]*)"`)

// Test returns true if the URL is a mangapark URL
func (m *Mangapark) Test() (bool, error) {
	re := regexp.MustCompile(`mangapark\.(to|page)`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangapark) FetchTitle() (string, error) {
	if err := m.fetchSeriesData(); err != nil {
		return "", err
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangapark) FetchChapters() (Filterables, []error) {
	if err := m.fetchSeriesData(); err != nil {
		return nil, []error{err}
	}

	uri, err := url.Parse(m.BaseUrl())
	if err != nil {
		return nil, []error{err}
	}
	uri.Path = "/get-chapter-list"
	q := uri.Query()
	q.Set("slug", m.slug)
	uri.RawQuery = q.Encode()

	body, err := http.GetText(http.RequestParams{
		URL:     uri.String(),
		Referer: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}

	feed := mangaparkChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}
	if !feed.Success {
		return nil, []error{fmt.Errorf("mangapark: %s", feed.Error)}
	}

	chapters := make(Filterables, 0, len(feed.Data))
	for _, c := range feed.Data {
		chapters = append(chapters, &MangaparkChapter{
			Chapter{
				Number: c.Number,
				Title:  c.Name,
			},
			c.Slug,
		})
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Mangapark) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaparkChapter)

	uri, err := url.JoinPath(m.BaseUrl(), "series", m.slug, mchap.Slug)
	if err != nil {
		return nil, err
	}

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	// every page image is a `.image-thumb` with a `data-number` attribute
	// (this pairing is exclusive to reader pages: nothing else on the page
	// carries `data-number`), holding the real URL in `data-src`
	doc.Find("img.image-thumb[data-number]").Each(func(i int, s *goquery.Selection) {
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

// fetchSeriesData fetches the series page and caches its title and slug,
// parsed out of the `window.seriesData` JS blob. The slug is needed to hit
// the paginated chapters API, since the series page itself only renders the
// newest ~20 chapters.
func (m *Mangapark) fetchSeriesData() error {
	if m.slug != "" {
		return nil
	}

	body, err := http.GetText(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return err
	}

	matches := mangaparkSeriesDataRe.FindStringSubmatch(body)
	if len(matches) != 3 {
		return errors.New("could not find series data in the series page")
	}

	m.title = sanitizeTitle(matches[1])
	m.slug = matches[2]

	return nil
}

// mangaparkChaptersFeed is the JSON feed for the chapters list
type mangaparkChaptersFeed struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Data    []struct {
		Slug   string  `json:"chapter_slug"`
		Name   string  `json:"chapter_name"`
		Number float64 `json:"chapter_num"`
	} `json:"data"`
}
