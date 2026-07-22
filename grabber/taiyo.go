// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Taiyo is a grabber for taiyo.moe, a Next.js Portuguese scanlation site. Its
// series page loads the chapter list from a public tRPC JSON endpoint
// (api/trpc/chapters.getByMediaId), and its reader page embeds the full
// ordered page-image list (as {id, extension} pairs) inside the React
// Server Components "flight" payload streamed in a <script> tag of the
// plain, server-rendered HTML - no browser is needed for either page, and
// neither endpoint requires auth or cookies.
type Taiyo struct {
	*Grabber
	mediaId string
	title   string
}

func NewTaiyo(g *Grabber) *Taiyo {
	return &Taiyo{Grabber: g}
}

// TaiyoChapter represents a Taiyo Chapter
type TaiyoChapter struct {
	Chapter
	ID string
}

// Test returns true if the URL is a taiyo.moe URL
func (t *Taiyo) Test() (bool, error) {
	re := regexp.MustCompile(`taiyo\.moe`)
	return re.MatchString(t.URL), nil
}

// FetchTitle fetches and returns the manga title
func (t *Taiyo) FetchTitle() (string, error) {
	if t.title != "" {
		return t.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: t.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	t.title = sanitizeTitle(doc.Find("title").First().Text())

	return t.title, nil
}

// FetchChapters returns the chapters of the manga, paginating the tRPC
// chapters.getByMediaId endpoint until it runs out of pages
func (t *Taiyo) FetchChapters() (chapters Filterables, errs []error) {
	mediaId, err := t.mediaID()
	if err != nil {
		return nil, []error{err}
	}

	const perPage = 30
	for page := 1; ; page++ {
		uri, err := taiyoChaptersApiUrl(t.BaseUrl(), mediaId, page, perPage)
		if err != nil {
			errs = append(errs, err)
			return
		}

		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: t.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		var feed taiyoChaptersFeed
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if len(feed) == 0 {
			return
		}

		result := feed[0].Result.Data.Json
		for _, c := range result.Chapters {
			chapters = append(chapters, &TaiyoChapter{
				Chapter{
					Number: c.Number,
					Title:  c.Title,
				},
				c.ID,
			})
		}

		if page >= result.TotalPages {
			return
		}
	}
}

// FetchChapter fetches a chapter and its pages
func (t *Taiyo) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*TaiyoChapter)
	mediaId, err := t.mediaID()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(t.BaseUrl(), "chapter", tchap.ID, "1")
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: t.URL,
	})
	if err != nil {
		return nil, err
	}

	pages, err := taiyoChapterPages(body)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, errors.New("no pages found in the chapter page")
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "pt",
	}

	for i, p := range pages {
		imgUrl, _ := url.JoinPath("https://cdn.taiyo.moe", "medias", mediaId, "chapters", tchap.ID, p.ID+"."+p.Extension)
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    imgUrl,
		})
	}

	return chapter, nil
}

// mediaID returns (and caches) the media UUID parsed from the series URL,
// i.e. "000bdf97-407f-4ca8-95a1-ee2a3114e73a" for
// https://taiyo.moe/media/000bdf97-407f-4ca8-95a1-ee2a3114e73a
func (t *Taiyo) mediaID() (string, error) {
	if t.mediaId != "" {
		return t.mediaId, nil
	}

	id := getUuid(t.URL)
	if id == "" {
		return "", fmt.Errorf("could not find media id in url %s", t.URL)
	}
	t.mediaId = id

	return id, nil
}

// taiyoChaptersApiUrl builds the tRPC batch request URL for the
// chapters.getByMediaId procedure
func taiyoChaptersApiUrl(baseUrl, mediaId string, page, perPage int) (string, error) {
	input := map[string]any{
		"0": map[string]any{
			"json": map[string]any{
				"mediaId": mediaId,
				"page":    page,
				"perPage": perPage,
			},
		},
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}

	uri, err := url.JoinPath(baseUrl, "api", "trpc", "chapters.getByMediaId")
	if err != nil {
		return "", err
	}

	q := url.Values{}
	q.Set("batch", "1")
	q.Set("input", string(raw))

	return uri + "?" + q.Encode(), nil
}

// mediaChapterPagesRe finds the "mediaChapter" object embedded (with escaped
// quotes, since it lives inside a JS string literal in a <script> tag) in the
// reader page's React Server Components payload, capturing its "pages" array,
// e.g. \"mediaChapter\":{...,\"pages\":[{\"id\":\"..\",\"extension\":\"jpg\"},...]}
var mediaChapterPagesRe = regexp.MustCompile(`\\"mediaChapter\\":\{.*?\\"pages\\":\[(.*?)\]`)

// taiyoPageRe matches each individual {id, extension} pair within the pages
// array captured by mediaChapterPagesRe
var taiyoPageRe = regexp.MustCompile(`\\"id\\":\\"([a-f0-9-]{36})\\",\\"extension\\":\\"(\w+)\\"`)

// taiyoPage is a single reader page's image id and file extension
type taiyoPage struct {
	ID        string
	Extension string
}

// taiyoChapterPages extracts the ordered page list from a reader page's HTML
func taiyoChapterPages(html string) ([]taiyoPage, error) {
	m := mediaChapterPagesRe.FindStringSubmatch(html)
	if len(m) != 2 {
		return nil, errors.New("no page list found in the chapter page")
	}

	matches := taiyoPageRe.FindAllStringSubmatch(m[1], -1)
	pages := make([]taiyoPage, 0, len(matches))
	for _, mm := range matches {
		pages = append(pages, taiyoPage{ID: mm[1], Extension: mm[2]})
	}

	return pages, nil
}

// taiyoChaptersFeed is the tRPC batch response shape for
// chapters.getByMediaId
type taiyoChaptersFeed []struct {
	Result struct {
		Data struct {
			Json struct {
				Chapters []struct {
					ID     string  `json:"id"`
					Title  string  `json:"title"`
					Number float64 `json:"number"`
				} `json:"chapters"`
				TotalPages int `json:"totalPages"`
			} `json:"json"`
		} `json:"data"`
	} `json:"result"`
}
