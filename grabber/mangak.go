// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangak is a grabber for mangak.io (the mangabuddy rebrand), a Next.js site
// that embeds both the full chapter list and the chapter page images in the
// __NEXT_DATA__ JSON blob of its server-rendered pages (the visible HTML only
// contains a handful of chapters, so selectors are not enough here)
type Mangak struct {
	*Grabber
	manga *mangakManga
}

func NewMangak(g *Grabber) *Mangak {
	return &Mangak{Grabber: g}
}

// MangakChapter represents a Mangak Chapter
type MangakChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a mangak.io URL
func (m *Mangak) Test() (bool, error) {
	re := regexp.MustCompile(`mangak\.io`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangak) FetchTitle() (string, error) {
	manga, err := m.fetchManga()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Name), nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangak) FetchChapters() (chapters Filterables, errs []error) {
	manga, err := m.fetchManga()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range manga.Chapters {
		// the JSON "number" field is a 1-based ordinal (Chapter 0 is 1), so
		// parse the real number from the chapter name instead
		number, ok := parseChapterNumber(c.Name)
		if !ok {
			continue
		}
		chapters = append(chapters, &MangakChapter{
			Chapter{
				Number: number,
				Title:  c.Name,
			},
			c.URL,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (m Mangak) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangakChapter)
	uri := mchap.URL
	if !strings.HasPrefix(uri, "http") {
		uri = m.BaseUrl() + uri
	}

	data, err := m.fetchNextData(uri)
	if err != nil {
		return nil, err
	}
	if data.Props.PageProps.InitialChapter == nil {
		return nil, errors.New("no chapter data found in the chapter page")
	}

	images := data.Props.PageProps.InitialChapter.Images
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(images)),
		Language:   "en",
	}

	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    img,
		})
	}

	return chapter, nil
}

// fetchManga fetches and caches the series info from the manga index page
func (m *Mangak) fetchManga() (*mangakManga, error) {
	if m.manga != nil {
		return m.manga, nil
	}

	data, err := m.fetchNextData(m.URL)
	if err != nil {
		return nil, err
	}
	if data.Props.PageProps.InitialManga == nil {
		return nil, errors.New("no manga data found in the page (is the URL a series page?)")
	}

	m.manga = data.Props.PageProps.InitialManga

	return m.manga, nil
}

// fetchNextData fetches the given URL and decodes its __NEXT_DATA__ JSON blob
func (m Mangak) fetchNextData(uri string) (*mangakNextData, error) {
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	raw := doc.Find("script#__NEXT_DATA__").Text()
	if raw == "" {
		return nil, errors.New("no __NEXT_DATA__ found in the page")
	}

	data := &mangakNextData{}
	if err = json.Unmarshal([]byte(raw), data); err != nil {
		return nil, err
	}

	return data, nil
}

// mangakManga is the series info embedded in the series page
type mangakManga struct {
	Name     string `json:"name"`
	Chapters []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"chapters"`
}

// mangakNextData is the __NEXT_DATA__ JSON payload of mangak.io pages
type mangakNextData struct {
	Props struct {
		PageProps struct {
			InitialManga   *mangakManga `json:"initialManga"`
			InitialChapter *struct {
				Images []string `json:"images"`
			} `json:"initialChapter"`
		} `json:"pageProps"`
	} `json:"props"`
}
