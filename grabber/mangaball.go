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

// Mangaball is a grabber for mangaball.net: the chapters list is served by a
// CSRF-protected JSON API (the CSRF token and session cookie are read off
// the series page and reused for the API call), while the chapter images are
// a plain JSON array embedded straight in the reader page's HTML
type Mangaball struct {
	*Grabber
	title string
	csrf  string
}

func NewMangaball(g *Grabber) *Mangaball {
	return &Mangaball{Grabber: g}
}

// MangaballChapter represents a Mangaball chapter (one per translation, as
// the same chapter number can have multiple scanlation groups/languages)
type MangaballChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a mangaball.net URL
func (m *Mangaball) Test() (bool, error) {
	re := regexp.MustCompile(`mangaball\.net`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangaball) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	if _, err := m.fetchCSRF(); err != nil {
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

	csrf, err := m.fetchCSRF()
	if err != nil {
		errs = append(errs, err)
		return
	}

	language := m.Settings.Language
	if language == "" {
		language = "en"
	}

	form := url.Values{}
	form.Set("title_id", tid)
	form.Set("userSettingsEnabled", "false")

	rbody, err := http.Post(http.RequestParams{
		URL:     "https://mangaball.net/api/v1/chapter/chapter-listing-by-title-id/",
		Referer: m.URL,
		Headers: map[string]string{"X-CSRF-TOKEN": csrf},
		Form:    form,
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
	if feed.Code != 200 {
		errs = append(errs, fmt.Errorf("mangaball api returned code %d: %s", feed.Code, feed.Message))
		return
	}

	for _, c := range feed.AllChapters {
		for _, t := range c.Translations {
			if t.Language != language {
				continue
			}
			title := t.Name
			if title == "" {
				title = c.Title
			}
			chapters = append(chapters, &MangaballChapter{
				Chapter{
					Number:     c.NumberFloat,
					Title:      title,
					Language:   t.Language,
					PagesCount: t.Pages,
				},
				t.URL,
			})
		}
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
// https://mangaball.net/title-detail/baki-gaiden-shin-chiharu-6a5ffe5d90273b5b995225d2/)
func (m Mangaball) titleID() (string, error) {
	re := regexp.MustCompile(`[0-9a-f]{24}`)
	matches := re.FindAllString(m.URL, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("could not find title id in url %s", m.URL)
	}
	return matches[len(matches)-1], nil
}

// fetchCSRF fetches the series page, caching both the manga title and the
// CSRF token needed to call the chapter-listing API (the API validates the
// token against the PHPSESSID cookie set by this same request, which the
// http package harvests automatically)
func (m *Mangaball) fetchCSRF() (string, error) {
	if m.csrf != "" {
		return m.csrf, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find(".comic-detail-card h6").First().Text())

	csrf, ok := doc.Find(`meta[name="csrf-token"]`).Attr("content")
	if !ok || csrf == "" {
		return "", errors.New("could not find csrf token on the series page")
	}
	m.csrf = csrf

	return m.csrf, nil
}

// mangaballChapterImagesRe extracts the `chapterImages` JSON array embedded
// in the reader page, e.g.:
// const chapterImages = JSON.parse(`["https://.../001.webp", ...]`);
var mangaballChapterImagesRe = regexp.MustCompile("const chapterImages = JSON\\.parse\\(`(\\[.*?\\])`\\);")

// mangaballChapterImages extracts the page image URLs from a reader page
func mangaballChapterImages(html string) (images []string, err error) {
	matches := mangaballChapterImagesRe.FindStringSubmatch(html)
	if len(matches) != 2 {
		return nil, errors.New("could not find the chapterImages variable in the reader page")
	}

	if err = json.Unmarshal([]byte(matches[1]), &images); err != nil {
		return nil, err
	}

	return
}

// mangaballChaptersFeed is the JSON feed returned by the chapter listing API
type mangaballChaptersFeed struct {
	Code        int    `json:"code"`
	Message     string `json:"message"`
	AllChapters []struct {
		NumberFloat float64 `json:"number_float"`
		Title       string  `json:"title"`

		Translations []struct {
			Name     string `json:"name"`
			Language string `json:"language"`
			Pages    int64  `json:"pages"`
			URL      string `json:"url"`
		} `json:"translations"`
	} `json:"ALL_CHAPTERS"`
}
