package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangadex is a grabber for mangadex.org
type Mangadex struct {
	*Grabber
	title string
}

// MangadexChapter represents a MangaDex Chapter
type MangadexChapter struct {
	Chapter
	Id string
}

// Test checks if the site is MangaDex
func (m *Mangadex) Test() (bool, error) {
	re := regexp.MustCompile(`mangadex\.org`)
	return re.MatchString(m.URL), nil
}

// GetTitle returns the title of the manga
func (m *Mangadex) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	id := getUuid(m.URL)

	rbody, err := http.Get(http.RequestParams{
		URL:     "https://api.mangadex.org/manga/" + id,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return "", err
	}
	defer rbody.Close()

	// decode json response
	body := mangadexManga{}
	if err = json.NewDecoder(rbody).Decode(&body); err != nil {
		return "", err
	}

	// fetch the title in the requested language
	if m.Settings.Language != "" {
		trans := body.Data.Attributes.AltTitles.GetTitleByLang(m.Settings.Language)

		if trans != "" {
			m.title = trans
			return m.title, nil
		}
	}

	// fallback to english
	m.title = body.Data.Attributes.Title["en"]

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Mangadex) FetchChapters() (chapters Filterables, errs []error) {
	id := getUuid(m.URL)

	baseOffset := 500
	var fetchChaps func(int)

	fetchChaps = func(offset int) {
		uri := fmt.Sprintf("https://api.mangadex.org/manga/%s/feed", id)
		params := url.Values{}
		params.Add("limit", fmt.Sprint(baseOffset))
		params.Add("order[volume]", "asc")
		params.Add("order[chapter]", "asc")
		params.Add("offset", fmt.Sprint(offset))
		if m.Settings.Language != "" {
			params.Add("translatedLanguage[]", m.Settings.Language)
		}
		uri = fmt.Sprintf("%s?%s", uri, params.Encode())

		rbody, err := http.Get(http.RequestParams{URL: uri})
		if err != nil {
			errs = append(errs, err)
			return
		}
		defer rbody.Close()
		// parse json body
		body := mangadexFeed{}
		if err = json.NewDecoder(rbody).Decode(&body); err != nil {
			errs = append(errs, err)
			return
		}

		for _, c := range body.Data {
			num, _ := strconv.ParseFloat(c.Attributes.Chapter, 64)
			chapters = append(chapters, &MangadexChapter{
				Chapter{
					Number:     num,
					Title:      c.Attributes.Title,
					Language:   c.Attributes.TranslatedLanguage,
					PagesCount: c.Attributes.Pages,
				},
				c.Id,
			})
		}

		if len(body.Data) > 0 {
			fetchChaps(offset + baseOffset)
		}
	}
	// initial call
	fetchChaps(0)

	return
}

// FetchChapter fetches a chapter and its pages
func (m Mangadex) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*MangadexChapter)
	// download json
	rbody, err := http.Get(http.RequestParams{
		URL: "https://api.mangadex.org/at-home/server/" + chap.Id,
	})
	if err != nil {
		return nil, err
	}
	// parse json body
	body := mangadexPagesFeed{}
	if err = json.NewDecoder(rbody).Decode(&body); err != nil {
		return nil, err
	}

	pcount := len(body.Chapter.Data)

	chapter := &Chapter{
		Title:      fmt.Sprintf("Chapter %04d %s", int64(f.GetNumber()), chap.Title),
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   chap.Language,
	}

	// create pages
	for i, p := range body.Chapter.Data {
		num := i + 1
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(num),
			URL:    body.BaseUrl + path.Join("/data", body.Chapter.Hash, p),
		})
	}

	return chapter, nil
}

// mangadexManga represents the Manga json object
type mangadexManga struct {
	Id   string
	Data struct {
		Attributes struct {
			Title     map[string]string
			AltTitles altTitles
		}
	}
}

// altTitles is a slice of maps with the language as key and the title as value
type altTitles []map[string]string

// GetTitleByLang returns the title in the given language (or empty if string is not found)
func (a altTitles) GetTitleByLang(lang string) string {
	for _, t := range a {
		val, ok := t[lang]
		if ok {
			return val
		}
	}
	return ""
}

// mangadexFeed represents the json object returned by the feed endpoint
type mangadexFeed struct {
	Data []struct {
		Id         string
		Attributes struct {
			Volume             string
			Chapter            string
			Title              string
			TranslatedLanguage string
			Pages              int64
		}
	}
}

// mangadexPagesFeed represents the json object returned by the pages endpoint
type mangadexPagesFeed struct {
	BaseUrl string
	Chapter struct {
		Hash      string
		Data      []string
		DataSaver []string
	}
}
