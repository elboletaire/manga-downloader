package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangafire is a grabber for mangafire.to: the site is a react SPA, but both
// the chapters list and the chapter pages are served by their open JSON api
type Mangafire struct {
	*Grabber
	title string
}

func NewMangafire(g *Grabber) *Mangafire {
	return &Mangafire{Grabber: g}
}

// MangafireChapter represents a Mangafire Chapter
type MangafireChapter struct {
	Chapter
	Id int64
}

// Test returns true if the URL is a mangafire.to series URL
func (m *Mangafire) Test() (bool, error) {
	re := regexp.MustCompile(`mangafire\.to/(title|manga)/`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangafire) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	hid, err := m.hid()
	if err != nil {
		return "", err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     "https://mangafire.to/api/titles/" + hid,
		Referer: m.URL,
	})
	if err != nil {
		return "", err
	}

	feed := struct {
		Data struct {
			Title string `json:"title"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return "", err
	}

	m.title = feed.Data.Title

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Mangafire) FetchChapters() (chapters Filterables, errs []error) {
	hid, err := m.hid()
	if err != nil {
		return nil, []error{err}
	}

	language := m.Settings.Language
	if language == "" {
		language = "en"
	}

	page := 1
	for {
		uri := fmt.Sprintf(
			"https://mangafire.to/api/titles/%s/chapters?language=%s&sort=number&order=asc&page=%d&limit=100",
			hid, language, page,
		)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: m.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := mangafireChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}

		for _, c := range feed.Items {
			title := c.Name
			if title == "" {
				title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
			}
			chapters = append(chapters, &MangafireChapter{
				Chapter{
					Number:   c.Number,
					Title:    title,
					Language: c.Language,
				},
				c.Id,
			})
		}

		if !feed.Meta.HasNext {
			return
		}
		page++
	}
}

// FetchChapter fetches a chapter and its pages
func (m Mangafire) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangafireChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     fmt.Sprintf("https://mangafire.to/api/chapters/%d", mchap.Id),
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Data struct {
			Pages []struct {
				Url string `json:"url"`
			} `json:"pages"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
		PagesCount: int64(len(feed.Data.Pages)),
	}
	for i, p := range feed.Data.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.Url,
		})
	}

	return chapter, nil
}

// hid returns the title id from the URL, e.g. "dkw" for both the current
// https://mangafire.to/title/dkw-one-piece format and the legacy
// https://mangafire.to/manga/one-piecee.dkw one
func (m Mangafire) hid() (string, error) {
	re := regexp.MustCompile(`/title/([^/-]+)-`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	re = regexp.MustCompile(`/manga/[^/]+\.([^/.]+)`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("could not find title id in url %s", m.URL)
}

// mangafireChaptersFeed is the JSON feed for the chapters list
type mangafireChaptersFeed struct {
	Items []struct {
		Id       int64   `json:"id"`
		Number   float64 `json:"number"`
		Name     string  `json:"name"`
		Language string  `json:"language"`
	} `json:"items"`
	Meta struct {
		HasNext bool `json:"hasNext"`
	} `json:"meta"`
}
