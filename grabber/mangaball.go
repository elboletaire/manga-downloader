// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangaball is a grabber for mangaball.com (mangaball.net redirects to it): the
// title comes from the series page's Next.js payload, the chapters list from a
// JSON API, and the chapter images are a "pages" array in the reader page's
// RSC payload
type Mangaball struct {
	*Grabber
	title string
	// languages are the languages of every chapter the series lists, before
	// the --language filter
	languages []string
}

// mangaballAPI is the site's base URL for its API (a var so tests can point it
// at a local server)
var mangaballAPI = "https://api.mangaball.com/api/v1"

// mangaballSite is the site's base URL, where the reader pages live (a var so
// tests can point it at a local server)
var mangaballSite = "https://mangaball.com"

func NewMangaball(g *Grabber) *Mangaball {
	return &Mangaball{Grabber: g}
}

// LanguageOptions lists the languages the series has translations in. The
// chapters are only fetched once FetchChapters has read them, so this is valid
// after it.
func (m *Mangaball) LanguageOptions() FilterOptions {
	return FilterOptions{
		Values: uniqueValues(m.languages),
		All:    true,
	}
}

// MangaballChapter represents a Mangaball chapter (one per translation, as
// the same chapter number can have multiple scanlation groups/languages)
type MangaballChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a mangaball.com or mangaball.net URL
func (m *Mangaball) Test() (bool, error) {
	re := regexp.MustCompile(`mangaball\.(com|net)`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangaball) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	if err := m.fetchTitle(); err != nil {
		return "", err
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangaball) FetchChapters() (chapters Filterables, errs []error) {
	tid, err := m.titleID()
	if err != nil {
		errs = append(errs, err)
		return
	}

	language := m.Settings.Language
	if language == "" {
		language = "en"
	}

	body, err := json.Marshal(map[string]string{"title_id": tid})
	if err != nil {
		errs = append(errs, err)
		return
	}

	rbody, err := http.Post(http.RequestParams{
		URL:     mangaballAPI + "/chapter/chapter-listing-by-title-id",
		Referer: m.URL,
		// the API only takes a JSON body, which the http package can't send
		// on its own: the body goes out url-encoded, and this header replaces
		// the content type it sets for it
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    string(body),
	})
	if err != nil {
		errs = append(errs, err)
		return
	}
	defer rbody.Close()

	feed := mangaballChaptersFeed{}
	if err = json.NewDecoder(rbody).Decode(&feed); err != nil {
		errs = append(errs, err)
		return
	}
	if feed.Status != "success" {
		errs = append(errs, fmt.Errorf("mangaball api returned status %q", feed.Status))
		return
	}

	for _, c := range feed.Data {
		m.languages = append(m.languages, c.Lang)
		if language != FilterAll && c.Lang != language {
			continue
		}
		chapters = append(chapters, &MangaballChapter{
			Chapter{
				Number:   c.Number,
				Title:    c.Name,
				Language: c.Lang,
			},
			mangaballSite + "/chapter-detail/" + c.ID,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (m Mangaball) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaballChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     mchap.URL,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	images, err := mangaballChapterImages(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
		PagesCount: int64(len(images)),
	}
	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    img,
		})
	}

	return chapter, nil
}

// titleID returns the title's internal id (a 24-char hex ObjectId suffixed
// to the series slug, e.g. "6a5ffe5d90273b5b995225d2" for
// https://mangaball.com/title-detail/baki-gaiden-shin-chiharu-6a5ffe5d90273b5b995225d2/)
func (m Mangaball) titleID() (string, error) {
	re := regexp.MustCompile(`[0-9a-f]{24}`)
	matches := re.FindAllString(m.URL, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("could not find title id in url %s", m.URL)
	}
	return matches[len(matches)-1], nil
}

// fetchTitle fetches the series page and caches the manga title, read off the
// `initialTitleData` object of its RSC payload
func (m *Mangaball) fetchTitle() error {
	body, err := http.GetText(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return err
	}

	stream, err := nextFlightStream(body)
	if err != nil {
		return err
	}

	raw, err := extractBalancedJSON(stream, `"initialTitleData":`)
	if err != nil {
		return err
	}

	data := struct {
		Name string `json:"name"`
	}{}
	if err = json.Unmarshal([]byte(raw), &data); err != nil {
		return err
	}
	if data.Name == "" {
		return errors.New("could not find the title on the series page")
	}
	m.title = sanitizeTitle(data.Name)

	return nil
}

// mangaballChapterImages extracts the page image URLs from a reader page: they
// sit in the `pages` array of the chapter in the page's RSC payload
func mangaballChapterImages(html string) (images []string, err error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"pages":`)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal([]byte(raw), &images); err != nil {
		return nil, err
	}

	return
}

// mangaballChaptersFeed is the JSON feed returned by the chapter listing API
type mangaballChaptersFeed struct {
	Status string `json:"status"`
	Data   []struct {
		ID     string  `json:"id"`
		Lang   string  `json:"lang"`
		Name   string  `json:"name"`
		Number float64 `json:"number"`
	} `json:"data"`
}
