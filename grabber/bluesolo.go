// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Bluesolo is a grabber for bluesolo.org: a Vue SPA, but both the series
// details (title + chapters) and the chapter pages are served by their own
// open JSON api (API_BASE_URL + the route path), reachable with plain HTTP
// and no cookies/session at all. Note the series detail endpoint only
// returns the chapters that are still hosted (older ones get pulled once
// officially licensed in French), so there's nothing to paginate: the
// "chapters" array in the response already is the full available list.
type Bluesolo struct {
	*Grabber
	title string
	feed  *bluesoloComicFeed
}

func NewBluesolo(g *Grabber) *Bluesolo {
	return &Bluesolo{Grabber: g}
}

// BluesoloChapter represents a Bluesolo Chapter
type BluesoloChapter struct {
	Chapter
	// ApiUrl is the absolute URL of the /api/read/... endpoint serving this
	// chapter's pages
	ApiUrl string
}

// Test returns true if the URL is a bluesolo.org series URL
func (b *Bluesolo) Test() (bool, error) {
	re := regexp.MustCompile(`bluesolo\.org`)
	return re.MatchString(b.URL), nil
}

// FetchTitle fetches and returns the manga title
func (b *Bluesolo) FetchTitle() (string, error) {
	if b.title != "" {
		return b.title, nil
	}

	feed, err := b.fetchComic()
	if err != nil {
		return "", err
	}

	b.title = sanitizeTitle(feed.Comic.Title)

	return b.title, nil
}

// FetchChapters returns the chapters of the manga
func (b *Bluesolo) FetchChapters() (chapters Filterables, errs []error) {
	feed, err := b.fetchComic()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range feed.Comic.Chapters {
		if c.Licensed != 0 {
			// licensed chapters can't be read anymore, skip them
			continue
		}
		if b.Settings.Language != "" && c.Language != b.Settings.Language {
			continue
		}

		number := float64(c.Chapter)
		if c.Subchapter != nil {
			n, err := strconv.ParseFloat(fmt.Sprintf("%d.%d", c.Chapter, *c.Subchapter), 64)
			if err == nil {
				number = n
			}
		}

		title := c.Title
		if title == "" {
			title = c.FullTitle
		}

		chapters = append(chapters, &BluesoloChapter{
			Chapter{
				Number:   number,
				Title:    title,
				Language: c.Language,
			},
			b.apiUrl(c.Url),
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (b *Bluesolo) FetchChapter(f Filterable) (*Chapter, error) {
	bchap := f.(*BluesoloChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     bchap.ApiUrl,
		Referer: b.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := bluesoloReaderFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   bchap.Language,
		PagesCount: int64(len(feed.Chapter.Pages)),
	}
	for i, url := range feed.Chapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    url,
		})
	}

	return chapter, nil
}

// fetchComic fetches and caches the /api/comics/{slug} feed, which contains
// both the series title and its full chapters list
func (b *Bluesolo) fetchComic() (*bluesoloComicFeed, error) {
	if b.feed != nil {
		return b.feed, nil
	}

	slug, err := b.seriesSlug()
	if err != nil {
		return nil, err
	}

	uri := b.apiUrl("/comics/" + slug)

	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: b.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := bluesoloComicFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	b.feed = &feed

	return b.feed, nil
}

// apiUrl returns the absolute /api url for the given site-relative path
// (e.g. "/read/frieren/fr/ch/147" -> "https://bluesolo.org/api/read/frieren/fr/ch/147")
func (b *Bluesolo) apiUrl(path string) string {
	return b.BaseUrl() + "/api" + path
}

// seriesSlug returns the series slug from the URL (i.e. "frieren" for
// https://bluesolo.org/comics/frieren)
func (b *Bluesolo) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/comics/([^/?#]+)`)
	matches := re.FindStringSubmatch(b.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", b.URL)
	}
	return matches[1], nil
}

// bluesoloComicFeed is the JSON feed for the series details
type bluesoloComicFeed struct {
	Comic struct {
		Title    string `json:"title"`
		Chapters []struct {
			Chapter    int64  `json:"chapter"`
			Subchapter *int64 `json:"subchapter"`
			Title      string `json:"title"`
			FullTitle  string `json:"full_title"`
			Language   string `json:"language"`
			Url        string `json:"url"`
			Licensed   int    `json:"licensed"`
		} `json:"chapters"`
	} `json:"comic"`
}

// bluesoloReaderFeed is the JSON feed for a chapter's pages
type bluesoloReaderFeed struct {
	Chapter struct {
		Pages []string `json:"pages"`
	} `json:"chapter"`
}
