// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Zinmanga is a grabber for zinmanga.net, a custom PHP site that looks like
// Madara. The series page only holds the title; the chapters come from a
// paginated JSON api, and each reader page has its images as data-src.
type Zinmanga struct {
	*Grabber
	title string
}

func NewZinmanga(g *Grabber) *Zinmanga {
	return &Zinmanga{Grabber: g}
}

// ZinmangaChapter represents a Zinmanga Chapter
type ZinmangaChapter struct {
	Chapter
	// Slug is the chapter's path segment under the series ("chapter-24")
	Slug string
}

// zinmangaChapterRe matches the path of a chapter URL, /manga/{slug}/{chapter}
var zinmangaChapterRe = regexp.MustCompile(`(?i)^(/manga/[^/]+)/[^/]+/?$`)

// zinmangaSlugRe captures the series slug from a series or chapter URL
var zinmangaSlugRe = regexp.MustCompile(`(?i)/manga/([^/?#]+)`)

// Test returns true if the URL is a zinmanga.net URL
func (z *Zinmanga) Test() (bool, error) {
	return hostMatchesDomain(z.URL, "zinmanga.net"), nil
}

// resolveSeriesURL rewrites a chapter URL (/manga/{slug}/{chapter}) to the
// series URL (/manga/{slug}). No-op for series URLs.
func (z *Zinmanga) resolveSeriesURL() {
	u, err := url.Parse(z.URL)
	if err != nil {
		return
	}
	matches := zinmangaChapterRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return
	}
	u.Path = matches[1]
	u.RawQuery = ""
	u.Fragment = ""
	z.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (z *Zinmanga) FetchTitle() (string, error) {
	z.resolveSeriesURL()
	if z.title != "" {
		return z.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: z.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	title := sanitizeTitle(doc.Find("h1").First().Text())
	if title == "" {
		return "", fmt.Errorf("could not find the title in %s", z.URL)
	}
	z.title = title

	return z.title, nil
}

// FetchChapters returns the chapters of the manga, walking every page of the
// chapters api
func (z *Zinmanga) FetchChapters() (chapters Filterables, errs []error) {
	z.resolveSeriesURL()
	slug, err := z.slug()
	if err != nil {
		return nil, []error{err}
	}

	for page := 1; ; page++ {
		uri := fmt.Sprintf("%s/api/comics/%s/chapters?page=%d", z.BaseUrl(), slug, page)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: z.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := zinmangaChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if !feed.Success {
			errs = append(errs, fmt.Errorf("zinmanga chapters api returned an unsuccessful response for %s", z.URL))
			return
		}

		chapters = append(chapters, zinmangaChapters(feed.Data.Chapters)...)

		if len(feed.Data.Chapters) == 0 || page >= feed.Data.LastPage {
			return
		}
	}
}

// zinmangaChapters turns one page of the chapters api into chapters
func zinmangaChapters(items []zinmangaChapterItem) Filterables {
	var chapters Filterables
	for _, c := range items {
		chapters = append(chapters, &ZinmangaChapter{
			Chapter: Chapter{
				Number:   c.Number,
				Language: "en",
				Label:    Label{Style: LabelNumbered},
			},
			Slug: c.Slug,
		})
	}

	return chapters
}

// FetchChapter fetches a chapter and its pages
func (z *Zinmanga) FetchChapter(f Filterable) (*Chapter, error) {
	zchap := f.(*ZinmangaChapter)
	slug, err := z.slug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(z.BaseUrl(), "manga", slug, zchap.Slug)
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: z.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: zchap.Language,
		Pages:    zinmangaPages(doc),
	}
	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// zinmangaPages returns the images of a reader page, in order. Each image's
// URL is in data-src.
func zinmangaPages(doc *goquery.Document) (pages []Page) {
	doc.Find("img.wp-manga-chapter-img").Each(func(_ int, s *goquery.Selection) {
		src := s.AttrOr("data-src", "")
		if src == "" {
			return
		}
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	return pages
}

// slug returns the series slug from the URL (i.e. "academy-idolmaster-gold-rush"
// for https://www.zinmanga.net/manga/academy-idolmaster-gold-rush)
func (z *Zinmanga) slug() (string, error) {
	matches := zinmangaSlugRe.FindStringSubmatch(z.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", z.URL)
	}
	return matches[1], nil
}

// zinmangaChaptersFeed is one page of the chapters api
type zinmangaChaptersFeed struct {
	Success bool `json:"success"`
	Data    struct {
		Chapters []zinmangaChapterItem `json:"chapters"`
		LastPage int                   `json:"last_page"`
	} `json:"data"`
}

// zinmangaChapterItem is one chapter in the chapters api
type zinmangaChapterItem struct {
	Number float64 `json:"chapter_num"`
	Slug   string  `json:"chapter_slug"`
}
