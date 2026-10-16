// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

var (
	// inmangaChapterRe matches chapter URLs of the form
	// /ver/manga/<slug>/<chapter-num>/<chapter-uuid> where chapter-num may be
	// a decimal like "912-5" (inmanga's notation for chapter 912, part 5).
	// The chapter UUID segment distinguishes chapter URLs from series URLs,
	// which only have a single UUID after the slug.
	inmangaChapterRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?inmanga\.com/ver/manga/[^/]+/[^/]+/[a-f0-9-]+/?(?:[?#].*)?$`)
	inmangaMidRe     = regexp.MustCompile(`(?i)var\s+mid\s*=\s*'([a-f0-9-]+)'`)
	inmangaSlugRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?inmanga\.com/ver/manga/([^/]+)/`)
)

// Inmanga is a grabber for inmanga.com
type Inmanga struct {
	*Grabber
	title string
}

func NewInmanga(g *Grabber) *Inmanga {
	return &Inmanga{Grabber: g}
}

// inmangaChapterURL checks if the URL is a chapter URL (not a series URL).
func inmangaChapterURL(url string) bool {
	return inmangaChapterRe.MatchString(url)
}

// inmangaSeriesURL extracts the series URL from a chapter URL, given HTML containing the manga ID.
func inmangaSeriesURL(chapterURL, html string) (string, bool) {
	if !inmangaChapterURL(chapterURL) {
		return "", false
	}

	// Extract mid from html: var mid = '<uuid>';
	matches := inmangaMidRe.FindStringSubmatch(html)
	if len(matches) < 2 {
		return "", false
	}
	mid := matches[1]

	slugMatches := inmangaSlugRe.FindStringSubmatch(chapterURL)
	if len(slugMatches) < 2 {
		return "", false
	}
	slug := slugMatches[1]

	return fmt.Sprintf("https://inmanga.com/ver/manga/%s/%s", slug, mid), true
}

// InmangaChapter is a chapter representation from InManga
type InmangaChapter struct {
	Chapter
	Id string
}

// Test checks if the site is InManga
func (i *Inmanga) Test() (bool, error) {
	re := regexp.MustCompile(`inmanga\.com`)
	return re.MatchString(i.URL), nil
}

// resolveSeriesURL resolves a chapter URL to the series URL if needed.
func (i *Inmanga) resolveSeriesURL() error {
	if !inmangaChapterURL(i.URL) {
		return nil
	}

	body, err := http.Get(http.RequestParams{
		URL: i.URL,
	})
	if err != nil {
		return err
	}
	defer body.Close()

	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	html := string(htmlBytes)

	seriesURL, ok := inmangaSeriesURL(i.URL, html)
	if !ok {
		return errSeriesNotFound(i.URL)
	}

	i.URL = seriesURL
	return nil
}

// FetchTitle fetches the manga title
func (i *Inmanga) FetchTitle() (string, error) {
	if i.title != "" {
		return i.title, nil
	}

	if err := i.resolveSeriesURL(); err != nil {
		return "", err
	}

	body, err := http.Get(http.RequestParams{
		URL: i.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	i.title = doc.Find("h1").Text()

	return i.title, nil
}

// FetchChapters returns the chapters of the manga
func (i *Inmanga) FetchChapters() (Filterables, []error) {
	if err := i.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	id := getUuid(i.URL)

	body, err := http.GetText(http.RequestParams{
		URL: "https://inmanga.com/chapter/getall?mangaIdentification=" + id,
	})
	if err != nil {
		return nil, []error{err}
	}

	raw := struct {
		Data string
	}{}

	if err = json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, []error{err}
	}

	feed := inmangaChapterFeed{}
	if err = json.Unmarshal([]byte(raw.Data), &feed); err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(feed.Result))
	for _, c := range feed.Result {
		chapters = append(chapters, newInmangaChapter(c))
	}

	return chapters, nil
}

// FetchChapter fetches the chapter with its pages
func (i *Inmanga) FetchChapter(chap Filterable) (*Chapter, error) {
	ichap := chap.(*InmangaChapter)
	body, err := http.Get(http.RequestParams{
		URL: "https://inmanga.com/chapter/chapterIndexControls?identification=" + ichap.Id,
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
		Title:      chap.GetTitle(),
		Label:      labelOf(chap),
		Number:     chap.GetNumber(),
		PagesCount: int64(ichap.PagesCount),
		// Inmanga only hosts spanish mangas
		Language: "es",
		ID:       ichap.Id,
	}

	// get pages from select, deduplicating by page id (the select is
	// duplicated in the page: top and bottom navigation)
	seen := map[string]bool{}
	doc.Find("select.PageListClass").First().Children().Each(func(i int, s *goquery.Selection) {
		id := s.AttrOr("value", "")
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		num, _ := strconv.ParseInt(s.Text(), 10, 64)
		chapter.Pages = append(chapter.Pages, Page{
			Number: num,
			URL:    "https://pack-yak.intomanga.com/images/manga/ms/chapter/ch/page/p/" + id,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// newInmangaChapter creates an InMangaChapter from an InMangaChapterFeedResult
func newInmangaChapter(c inmangaChapterFeedResult) *InmangaChapter {
	return &InmangaChapter{
		Chapter{
			Number:     c.Number,
			PagesCount: int64(c.PagesCount),
			Label:      Label{Style: LabelNumbered, Lang: "es"},
			ID:         c.Id,
		},
		c.Id,
	}
}

// inmangaChapterFeed is the JSON feed for the chapters list
type inmangaChapterFeed struct {
	Result []inmangaChapterFeedResult
}

// inmangaChapterFeedResult is the JSON feed for a single chapter result
type inmangaChapterFeedResult struct {
	Id         string `json:"identification"`
	Number     float64
	PagesCount float64
}
