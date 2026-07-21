// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Qimanga is a grabber for qimanga.com: the reader pages are server side
// rendered, but the series page only renders the newest chapters, so the
// full chapters list needs to be fetched from their paginated JSON api
type Qimanga struct {
	*Grabber
	title string
}

func NewQimanga(g *Grabber) *Qimanga {
	return &Qimanga{Grabber: g}
}

// QimangaChapter represents a Qimanga Chapter
type QimangaChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a qimanga.com URL
func (q *Qimanga) Test() (bool, error) {
	re := regexp.MustCompile(`qimanga\.com`)
	return re.MatchString(q.URL), nil
}

// FetchTitle fetches and returns the manga title
func (q *Qimanga) FetchTitle() (string, error) {
	if q.title != "" {
		return q.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: q.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	q.title = sanitizeTitle(doc.Find("h1.series-title").First().Text())

	return q.title, nil
}

// FetchChapters returns the chapters of the manga
func (q Qimanga) FetchChapters() (chapters Filterables, errs []error) {
	slug, err := q.seriesSlug()
	if err != nil {
		return nil, []error{err}
	}

	page := 1
	for {
		uri := fmt.Sprintf("https://api.qimanga.com/api/v1/series/%s/chapters?page=%d", slug, page)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: q.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := qimangaChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if len(feed.Data) == 0 {
			return
		}

		for _, c := range feed.Data {
			title := c.Title
			if title == "" {
				title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
			}
			chapters = append(chapters, &QimangaChapter{
				Chapter{
					Number: c.Number,
					Title:  title,
				},
				c.Slug,
			})
		}

		if page >= feed.TotalPages {
			return
		}
		page++
	}
}

// FetchChapter fetches a chapter and its pages
func (q Qimanga) FetchChapter(f Filterable) (*Chapter, error) {
	qchap := f.(*QimangaChapter)
	slug, err := q.seriesSlug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(q.BaseUrl(), "series", slug, qchap.Slug)
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: q.URL,
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

	doc.Find("img.r-page-img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
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

// seriesSlug returns the series slug from the URL (i.e. "1234-some-manga"
// for https://qimanga.com/series/1234-some-manga)
func (q Qimanga) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/series/([^/]+)`)
	matches := re.FindStringSubmatch(q.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", q.URL)
	}
	return matches[1], nil
}

// qimangaChaptersFeed is the JSON feed for the chapters list
type qimangaChaptersFeed struct {
	Data []struct {
		Slug   string  `json:"slug"`
		Number float64 `json:"number"`
		Title  string  `json:"title"`
	} `json:"data"`
	TotalPages int `json:"totalPages"`
}
