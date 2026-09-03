// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// mangakDefaultAPIURL is the fallback API base when the page doesn't
// declare one in its site config
const mangakDefaultAPIURL = "https://api.mangak.io"

// Mangak is a grabber for mangak.io (the mangabuddy rebrand), a Next.js site
// whose series pages embed only the 50 newest chapters in the __NEXT_DATA__
// JSON blob (the field is literally named *initial*Manga); the full list is
// served by the site's own API at titles/{mangaId}/chapters
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

	// the series page embeds only the newest chapters, so fetching fewer raw
	// entries than the declared total means the API fetch fell back to that
	// partial blob (or the site truncated the list some new way); compared
	// against the raw list because number-less entries (site announcements
	// like "Notice.110") are skipped above on purpose
	if count := manga.Stats.ChaptersCount; count > 0 && len(manga.Chapters) < count {
		color.Yellow("found %d chapters but the site declares %d: the list may be incomplete", len(manga.Chapters), count)
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

// fetchManga fetches and caches the series info from the manga index page,
// completing the (truncated) embedded chapter list through the site's API
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

	manga := data.Props.PageProps.InitialManga
	if chapters, err := m.fetchAllChapters(data.Props.PageProps.SiteConfig.APIURL, manga.ID, manga.CV); err != nil {
		// keep the (newest-50) embedded list rather than failing outright;
		// FetchChapters warns about the truncation via chaptersCount
		color.Yellow("could not fetch the full chapter list from the api: %s", err.Error())
	} else {
		manga.Chapters = chapters
	}

	m.manga = manga

	return m.manga, nil
}

// fetchAllChapters fetches the complete chapter list from the mangak API
// (titles/{mangaId}/chapters); cv is the site's cache-version token — without
// it the CDN happily serves a stale list missing the newest chapters
func (m Mangak) fetchAllChapters(apiURL, mangaID string, cv int64) ([]mangakChapterEntry, error) {
	if mangaID == "" {
		return nil, errors.New("the series data has no manga id")
	}
	if apiURL == "" {
		apiURL = mangakDefaultAPIURL
	}

	uri := fmt.Sprintf("%s/titles/%s/chapters", strings.TrimSuffix(apiURL, "/"), mangaID)
	if cv > 0 {
		uri = fmt.Sprintf("%s?cv=%d", uri, cv)
	}

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	return parseMangakChaptersList(raw)
}

// parseMangakChaptersList decodes a chapters list API response
func parseMangakChaptersList(raw []byte) ([]mangakChapterEntry, error) {
	var resp struct {
		Success bool `json:"success"`
		Data    *struct {
			Chapters []mangakChapterEntry `json:"chapters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if !resp.Success || resp.Data == nil {
		return nil, errors.New("unsuccessful api response")
	}
	if len(resp.Data.Chapters) == 0 {
		return nil, errors.New("no chapters in the api response")
	}

	return resp.Data.Chapters, nil
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

// mangakChapterEntry is a chapter as listed both in the series page blob and
// in the chapters list API
type mangakChapterEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// mangakManga is the series info embedded in the series page
type mangakManga struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	CV       int64                `json:"cv"`
	Chapters []mangakChapterEntry `json:"chapters"`
	Stats    struct {
		ChaptersCount int `json:"chaptersCount"`
	} `json:"stats"`
}

// mangakNextData is the __NEXT_DATA__ JSON payload of mangak.io pages
type mangakNextData struct {
	Props struct {
		PageProps struct {
			SiteConfig struct {
				APIURL string `json:"apiUrl"`
			} `json:"siteConfig"`
			InitialManga   *mangakManga `json:"initialManga"`
			InitialChapter *struct {
				Images []string `json:"images"`
			} `json:"initialChapter"`
		} `json:"pageProps"`
	} `json:"props"`
}
