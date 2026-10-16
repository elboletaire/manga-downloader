// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangitto is a grabber for mangtto.com (aka mangitto.com): the reader pages
// are server side rendered with the page images already in the HTML, but the
// series page only renders its newest chapters, so the full chapters list
// needs to be fetched from their open JSON api
type Mangitto struct {
	*Grabber
	title string
}

func NewMangitto(g *Grabber) *Mangitto {
	return &Mangitto{Grabber: g}
}

// MangittoChapter represents a Mangitto Chapter
type MangittoChapter struct {
	Chapter
}

// Test returns true if the URL is a mangtto.com/mangitto.com URL
func (m *Mangitto) Test() (bool, error) {
	re := regexp.MustCompile(`mangi?tto\.com`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangitto) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	slug, err := m.slug()
	if err != nil {
		return "", err
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "api", "manga", slug)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return "", err
	}

	feed := struct {
		Title string `json:"title"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return "", err
	}

	m.title = sanitizeTitle(feed.Title)

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Mangitto) FetchChapters() (chapters Filterables, errs []error) {
	slug, err := m.slug()
	if err != nil {
		return nil, []error{err}
	}

	skip := 0
	for {
		uri := fmt.Sprintf("%s/api/manga/%s/chapters?skip=%d&take=%d", m.BaseUrl(), slug, skip, mangittoChaptersPerPage)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: m.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := mangittoChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}

		for _, c := range feed.Chapters {
			chapters = append(chapters, &MangittoChapter{
				Chapter{
					Number:   c.Number,
					Language: "tr",
					Label:    Label{Style: LabelNumbered},
				},
			})
		}

		skip += len(feed.Chapters)
		if len(feed.Chapters) == 0 || skip >= feed.Total {
			return
		}
	}
}

// FetchChapter fetches a chapter and its pages
func (m Mangitto) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangittoChapter)
	slug, err := m.slug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "manga", slug, strconv.FormatFloat(mchap.Number, 'f', -1, 64))
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
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: mchap.Language,
	}

	doc.Find("img[data-page-number]").Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// slug returns the manga slug from the URL (i.e. "chainsaw-man" for
// https://mangtto.com/manga/chainsaw-man)
func (m Mangitto) slug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/]+)`)
	matches := re.FindStringSubmatch(m.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", m.URL)
	}
	return matches[1], nil
}

// mangittoChaptersPerPage is the page size the chapters feed uses by default
const mangittoChaptersPerPage = 20

// mangittoChaptersFeed is the JSON feed for the paginated chapters list
type mangittoChaptersFeed struct {
	Chapters []struct {
		Number float64 `json:"chapter"`
	} `json:"chapters"`
	Total int `json:"total"`
}
