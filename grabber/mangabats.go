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

// Mangabats is a grabber for mangabats.com (former mangabat.com), which loads
// the chapters list from a JSON API and the chapter images from js variables
// in the reader page
type Mangabats struct {
	*Grabber
	title string
}

func NewMangabats(g *Grabber) *Mangabats {
	return &Mangabats{Grabber: g}
}

// MangabatsChapter represents a Mangabats Chapter
type MangabatsChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a mangabats.com URL
func (m *Mangabats) Test() (bool, error) {
	re := regexp.MustCompile(`mangabats\.com`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangabats) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find("h1").Text())

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Mangabats) FetchChapters() (Filterables, []error) {
	slug, err := m.slug()
	if err != nil {
		return nil, []error{err}
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "api", "manga", slug, "chapters")
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}

	feed := mangabatsChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(feed.Data.Chapters))
	for _, c := range feed.Data.Chapters {
		chapters = append(chapters, &MangabatsChapter{
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
func (m Mangabats) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangabatsChapter)
	slug, err := m.slug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "manga", slug, mchap.Slug)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	// images are defined in js variables: cdns hosts and relative image paths
	cdns, err := jsStringSlice(body, "cdns")
	if err != nil {
		return nil, err
	}
	if len(cdns) == 0 {
		return nil, errors.New("no image cdns found in the chapter page")
	}
	images, err := jsStringSlice(body, "chapterImages")
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(images)),
		Language:   "en",
	}

	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    strings.TrimRight(cdns[0], "/") + "/" + strings.TrimLeft(img, "/"),
		})
	}

	return chapter, nil
}

// slug returns the manga slug from the URL (i.e. "one-piece" for
// https://www.mangabats.com/manga/one-piece)
func (m Mangabats) slug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/]+)`)
	matches := re.FindStringSubmatch(m.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", m.URL)
	}
	return matches[1], nil
}

// jsStringSlice extracts a js string array variable from the passed html
func jsStringSlice(html, varname string) (values []string, err error) {
	re := regexp.MustCompile(`var ` + varname + ` = (\[[^\]]*\]);`)
	matches := re.FindStringSubmatch(html)
	if len(matches) != 2 {
		return nil, fmt.Errorf("could not find the %q variable in the chapter page", varname)
	}

	err = json.Unmarshal([]byte(matches[1]), &values)

	return
}

// mangabatsChaptersFeed is the JSON feed for the chapters list
type mangabatsChaptersFeed struct {
	Data struct {
		Chapters []struct {
			Name   string  `json:"chapter_name"`
			Slug   string  `json:"chapter_slug"`
			Number float64 `json:"chapter_num"`
		} `json:"chapters"`
	} `json:"data"`
}
