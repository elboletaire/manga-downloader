// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Komikmanga reads komikmanga.com (SvelteKit) through its /api/v1 JSON API;
// the series page renders only the first chapters.
type Komikmanga struct {
	*Grabber
}

// komikmangaURLRe matches series (/manga/{slug}) and chapter (/chapter/{id})
// URLs, with an optional www.
var komikmangaURLRe = regexp.MustCompile(`^https?://(www\.)?komikmanga\.com/(manga|chapter)/[^/?#]+`)

// komikmangaChapterRe matches chapter URLs, which resolve to their series
var komikmangaChapterRe = regexp.MustCompile(`^https?://[^/]+/chapter/([^/?#]+)`)

// komikmangaSlugRe captures the slug of a series URL
var komikmangaSlugRe = regexp.MustCompile(`/manga/([^/?#]+)`)

func NewKomikmanga(g *Grabber) *Komikmanga {
	return &Komikmanga{Grabber: g}
}

// KomikmangaChapter is a chapter of Komikmanga, its ID the chapter's id
type KomikmangaChapter struct {
	Chapter
}

// Test returns true if the URL is a komikmanga.com series or chapter URL
func (k *Komikmanga) Test() (bool, error) {
	return komikmangaURLRe.MatchString(k.URL), nil
}

// resolveSeriesURL rewrites a chapter URL (/chapter/{id}) to the series URL
// (/manga/{slug}) the API names for it. No-op for series URLs.
func (k *Komikmanga) resolveSeriesURL() error {
	matches := komikmangaChapterRe.FindStringSubmatch(k.URL)
	if matches == nil {
		return nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     k.BaseUrl() + "/api/v1/chapters/" + matches[1],
		Referer: k.BaseUrl(),
	})
	if err != nil {
		return err
	}

	chapter := struct {
		Chapter struct {
			Manga struct {
				Slug string `json:"slug"`
			} `json:"manga"`
		} `json:"chapter"`
	}{}
	if err = json.Unmarshal([]byte(body), &chapter); err != nil {
		return err
	}
	if chapter.Chapter.Manga.Slug == "" {
		return errSeriesNotFound(k.URL)
	}

	k.URL = k.BaseUrl() + "/manga/" + url.PathEscape(chapter.Chapter.Manga.Slug)
	return nil
}

// FetchTitle fetches and returns the manga title
func (k *Komikmanga) FetchTitle() (string, error) {
	if err := k.resolveSeriesURL(); err != nil {
		return "", err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     k.apiURL(),
		Referer: k.URL,
	})
	if err != nil {
		return "", err
	}

	manga := struct {
		Title string `json:"title"`
	}{}
	if err = json.Unmarshal([]byte(body), &manga); err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Title), nil
}

// komikmangaChaptersFeed is one page of the series' chapter list
type komikmangaChaptersFeed struct {
	Items []struct {
		ID          int64  `json:"id"`
		Number      string `json:"number"`
		IsAvailable bool   `json:"is_available"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

// FetchChapters returns the chapters of the manga, following the cursor
// until the list has no more pages
func (k *Komikmanga) FetchChapters() (chapters Filterables, errs []error) {
	if err := k.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	cursor := ""
	for {
		uri := k.apiURL() + "/chapters"
		if cursor != "" {
			uri += "?cursor=" + url.QueryEscape(cursor)
		}

		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: k.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := komikmangaChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}

		for _, item := range feed.Items {
			num, err := strconv.ParseFloat(item.Number, 64)
			if err != nil {
				continue
			}
			id := strconv.FormatInt(item.ID, 10)
			chapters = append(chapters, &KomikmangaChapter{
				Chapter{
					Number: num,
					ID:     id,
					Locked: !item.IsAvailable,
					Label:  Label{Style: LabelNumbered, Number: item.Number},
				},
			})
		}

		if !feed.HasMore || feed.NextCursor == "" {
			return
		}
		cursor = feed.NextCursor
	}
}

// FetchChapter fetches a chapter and its pages
func (k *Komikmanga) FetchChapter(f Filterable) (*Chapter, error) {
	kchap := f.(*KomikmangaChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     k.BaseUrl() + "/api/v1/chapters/" + url.PathEscape(kchap.ID),
		Referer: k.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Pages []struct {
			URL string `json:"url"`
		} `json:"pages"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	if len(feed.Pages) == 0 {
		return nil, fmt.Errorf("komikmanga chapter %s has no pages", kchap.ID)
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Label:  labelOf(f),
		Number: f.GetNumber(),
		ID:     kchap.ID,
	}
	for i, page := range feed.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    k.BaseUrl() + page.URL,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// apiURL is the API endpoint of the series in k.URL
func (k *Komikmanga) apiURL() string {
	slug := komikmangaSlug(k.URL)
	return k.BaseUrl() + "/api/v1/manga/" + url.PathEscape(slug)
}

// komikmangaSlug is the series slug of a /manga/{slug} URL
func komikmangaSlug(rawURL string) string {
	matches := komikmangaSlugRe.FindStringSubmatch(rawURL)
	if matches == nil {
		return ""
	}
	return matches[1]
}
