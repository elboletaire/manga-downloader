// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangaball reads everything from mangaball.com's JSON API, as its HTML pages
// sit behind a Turnstile challenge. Series URLs stay on .com: the image CDN
// wants a mangaball.com Referer, which is the downloader's default one.
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

// mangaballChapterRe matches chapter URLs like
// https://mangaball.com/chapter-detail/6a6711d656b91853f2dc9927 (www. and
// hex case handled inline via (?:www\.)? / (?i))
var mangaballChapterRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?mangaball\.(com|net)/chapter-detail/([a-f0-9]+)`)

// mangaballNetRe matches the host of mangaball.net URLs
var mangaballNetRe = regexp.MustCompile(`(?i)//(?:www\.)?mangaball\.net`)

// mangaballChapterURL checks if the URL is a chapter URL.
func mangaballChapterURL(url string) bool {
	return mangaballChapterRe.MatchString(url)
}

// mangaballMaxConcurrency caps the user's concurrency: the image CDN answers
// 429 to bursts, and these values downloaded whole series cleanly.
var mangaballMaxConcurrency = MaxConcurrency{Chapters: 1, Pages: 2}

// mangaballPagePacer spaces page requests evenly. It's shared by every
// Mangaball instance because the limit is per IP, not per download.
var mangaballPagePacer = &pacer{interval: time.Second / 4}

// GetMaxConcurrency caps the configured concurrency at what the image CDN
// tolerates; lower settings are kept.
func (m *Mangaball) GetMaxConcurrency() MaxConcurrency {
	mc := m.Grabber.GetMaxConcurrency()
	mc.Chapters = min(mc.Chapters, mangaballMaxConcurrency.Chapters)
	mc.Pages = min(mc.Pages, mangaballMaxConcurrency.Pages)
	return mc
}

// WaitPage paces page downloads to the image CDN's rate limit (PagePacer)
func (m *Mangaball) WaitPage() {
	mangaballPagePacer.wait()
}

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

// resolveSeriesURL resolves a chapter URL to the series URL if needed, and
// moves mangaball.net URLs to mangaball.com, whose Referer the image CDN wants.
func (m *Mangaball) resolveSeriesURL() error {
	if match := mangaballChapterRe.FindStringSubmatch(m.URL); match != nil {
		detail, err := m.chapterDetail(match[2])
		if err != nil {
			return err
		}
		if detail.TitleID == "" {
			return errSeriesNotFound(m.URL)
		}
		m.URL = mangaballSite + "/title-detail/" + detail.TitleID
		return nil
	}

	m.URL = mangaballNetRe.ReplaceAllString(m.URL, "//mangaball.com")
	return nil
}

// mangaballChapterDetail is the part of the chapter detail API we use
type mangaballChapterDetail struct {
	TitleID string   `json:"title_id"`
	Pages   []string `json:"pages"`
}

// chapterDetail asks the API for a chapter, which lists its title's id and
// its page image URLs
func (m *Mangaball) chapterDetail(id string) (*mangaballChapterDetail, error) {
	body, err := json.Marshal(map[string]string{"chapter_id": id})
	if err != nil {
		return nil, err
	}

	rbody, err := http.Post(http.RequestParams{
		URL:     mangaballAPI + "/chapter/detail",
		Referer: mangaballSite + "/",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    string(body),
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	res := struct {
		Status string `json:"status"`
		Data   struct {
			Chapter mangaballChapterDetail `json:"chapter"`
		} `json:"data"`
	}{}
	if err = json.NewDecoder(rbody).Decode(&res); err != nil {
		return nil, err
	}
	if res.Status != "success" {
		return nil, fmt.Errorf("mangaball api returned status %q", res.Status)
	}

	return &res.Data.Chapter, nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangaball) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	if err := m.resolveSeriesURL(); err != nil {
		return "", err
	}

	if err := m.fetchTitle(); err != nil {
		return "", err
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangaball) FetchChapters() (chapters Filterables, errs []error) {
	if err := m.resolveSeriesURL(); err != nil {
		errs = append(errs, err)
		return
	}

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
		listed := &MangaballChapter{
			Chapter{
				Number:   c.Number,
				Title:    c.Name,
				Language: c.Lang,
				ID:       c.ID,
			},
			mangaballSite + "/chapter-detail/" + c.ID,
		}
		listed.SetGroups(c.GroupName)
		chapters = append(chapters, listed)
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (m Mangaball) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaballChapter)

	detail, err := m.chapterDetail(mchap.ID)
	if err != nil {
		return nil, err
	}
	images := detail.Pages
	if len(images) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s", mchap.ID)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
		ID:         mchap.ID,
		PagesCount: int64(len(images)),
	}
	chapter.SetGroups(mchap.GetGroups()...)
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

// fetchTitle fetches the title from the API (the series page is challenged)
// and caches it
func (m *Mangaball) fetchTitle() error {
	tid, err := m.titleID()
	if err != nil {
		return err
	}

	rbody, err := http.Get(http.RequestParams{
		URL:     mangaballAPI + "/title/" + tid,
		Referer: m.URL,
	})
	if err != nil {
		return err
	}
	defer rbody.Close()

	data := struct {
		Status string `json:"status"`
		Data   struct {
			Name string `json:"name"`
		} `json:"data"`
	}{}
	if err = json.NewDecoder(rbody).Decode(&data); err != nil {
		return err
	}
	if data.Data.Name == "" {
		return errors.New("could not find the title in the API response")
	}
	m.title = sanitizeTitle(data.Data.Name)

	return nil
}

// mangaballChaptersFeed is the JSON feed returned by the chapter listing API
type mangaballChaptersFeed struct {
	Status string `json:"status"`
	Data   []struct {
		ID        string  `json:"id"`
		Lang      string  `json:"lang"`
		Name      string  `json:"name"`
		Number    float64 `json:"number"`
		GroupName string  `json:"group_name"`
	} `json:"data"`
}
