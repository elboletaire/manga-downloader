// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangadenizi is a grabber for mangadenizi.net, a Turkish manga/manhwa site
// whose series and reader pages are Nuxt-rendered, but both are backed by a
// wide-open JSON API (no auth, no cloudflare). The reader additionally
// scrambles page images into shuffled tiles ("tiled-v1"); the method/grid/
// seed needed to undo it are handed to us in plaintext by the same API, so
// it's reversed in Go (see mangadenizi_scramble.go) instead of reaching for
// a browser.
type Mangadenizi struct {
	*Grabber
	manga *mangadeniziManga
}

func NewMangadenizi(g *Grabber) *Mangadenizi {
	return &Mangadenizi{Grabber: g}
}

// MangadeniziChapter represents a Mangadenizi Chapter
type MangadeniziChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a mangadenizi.net URL
func (m *Mangadenizi) Test() (bool, error) {
	re := regexp.MustCompile(`mangadenizi\.net`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangadenizi) FetchTitle() (string, error) {
	manga, err := m.fetchManga()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangadenizi) FetchChapters() (chapters Filterables, errs []error) {
	manga, err := m.fetchManga()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range manga.Chapters {
		title := c.Title
		if title == "" {
			title = "Bölüm " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		chapters = append(chapters, &MangadeniziChapter{
			Chapter{
				Number:   c.Number,
				Title:    title,
				Language: "tr",
			},
			c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (m *Mangadenizi) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangadeniziChapter)

	slug, err := m.slug()
	if err != nil {
		return nil, err
	}

	uri := m.BaseUrl() + "/api/v1/reader/" + slug + "/" + mchap.Slug
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := mangadeniziReaderFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(feed.Pages)),
		Language:   "tr",
	}

	for _, p := range feed.Pages {
		scramble := p.Scramble
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(p.PageNumber),
			URL:    p.ImageURL,
			Transform: func(data []byte) ([]byte, error) {
				return descrambleMangadeniziImage(data, scramble)
			},
		})
	}

	return chapter, nil
}

// fetchManga fetches and caches the series info from the manga JSON API
func (m *Mangadenizi) fetchManga() (*mangadeniziManga, error) {
	if m.manga != nil {
		return m.manga, nil
	}

	slug, err := m.slug()
	if err != nil {
		return nil, err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     m.BaseUrl() + "/api/v1/web/manga/" + slug,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Data struct {
			Manga mangadeniziManga `json:"manga"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	m.manga = &feed.Data.Manga

	return m.manga, nil
}

// slug returns the manga slug from the URL (i.e. "solo-leveling" for
// https://www.mangadenizi.net/manga/solo-leveling)
func (m Mangadenizi) slug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/?]+)`)
	matches := re.FindStringSubmatch(m.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", m.URL)
	}
	return matches[1], nil
}

// mangadeniziManga is the series info returned by the manga JSON API
type mangadeniziManga struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Chapters []struct {
		Number float64 `json:"number"`
		Title  string  `json:"title"`
		Slug   string  `json:"slug"`
	} `json:"chapters"`
}

// mangadeniziReaderFeed is the JSON feed for a chapter's reader page
type mangadeniziReaderFeed struct {
	Pages []struct {
		PageNumber int                 `json:"page_number"`
		ImageURL   string              `json:"image_url"`
		Scramble   mangadeniziScramble `json:"scramble"`
	} `json:"pages"`
}
