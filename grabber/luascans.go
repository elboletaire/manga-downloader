// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Luascans is a grabber for the Next.js platform behind luacomic.org and omegascans.org:
// reader pages are plain HTML, and the chapters list comes from a JSON API keyed by a
// series id that the series page's React Server Components payload carries.
type Luascans struct {
	*Grabber
	// byMarkup also detects the hosts not in the domain list (NewLuascansByMarkup)
	byMarkup   bool
	title      string
	seriesID   string
	seriesPage string
}

// luascansMarkers are the keys a series page's React Server Components
// payload carries on the platform
var luascansMarkers = []string{"self.__next_f.push", `series_id\":`, `series_type\":\"Comic\"`}

// luascansSeriesRe is the URL shape of a series or chapter on the platform
var luascansSeriesRe = regexp.MustCompile(`^/series/[^/]+`)

func NewLuascans(g *Grabber) *Luascans {
	return &Luascans{Grabber: g}
}

// NewLuascansByMarkup returns a Luascans grabber that also matches a series or
// chapter URL on any host whose page has the platform's markup.
func NewLuascansByMarkup(g *Grabber) *Luascans {
	return &Luascans{Grabber: g, byMarkup: true}
}

// LuascansChapter represents a Luascans Chapter
type LuascansChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a luacomic.org URL. The by-markup instance
// also fetches a series or chapter URL on another host, once, to check its markup.
func (l *Luascans) Test() (bool, error) {
	re := regexp.MustCompile(`luacomic\.org`)
	if re.MatchString(l.URL) {
		return true, nil
	}
	if !l.byMarkup {
		return false, nil
	}
	seriesURL, ok := luascansMarkupURL(l.URL)
	if !ok {
		return false, nil
	}
	return pageHasMarkup(l.Grabber, seriesURL, luascansMarkers...), nil
}

// luascansMarkupURL returns the series page whose markup identifies the
// platform for a series or chapter URL, and false for any other URL
func luascansMarkupURL(rawURL string) (string, bool) {
	uri, err := url.Parse(rawURL)
	if err != nil || !luascansSeriesRe.MatchString(uri.Path) {
		return "", false
	}
	// a chapter page has no series payload, so the series page is checked
	luascansStripChapter(uri)
	uri.RawQuery = ""
	uri.Fragment = ""
	return uri.String(), true
}

// luascansStripChapter cuts a chapter URL back to its series, dropping the
// query and fragment too, and reports whether the URL was a chapter one
func luascansStripChapter(uri *url.URL) bool {
	idx := strings.Index(uri.Path, "/chapter-")
	if idx < 0 {
		return false
	}
	uri.Path = uri.Path[:idx]
	uri.RawQuery = ""
	uri.Fragment = ""
	return true
}

// apiBase returns the base URL of the chapters API, which every site of the
// platform serves from api.{host}
func (l *Luascans) apiBase() (string, error) {
	uri, err := url.Parse(l.URL)
	if err != nil {
		return "", err
	}
	return "https://api." + strings.TrimPrefix(uri.Host, "www."), nil
}

// resolveSeriesURL converts chapter URLs (/series/{slug}/chapter-{n}...) to series URLs (/series/{slug}).
func (l *Luascans) resolveSeriesURL() error {
	parsed, err := url.Parse(l.URL)
	if err != nil {
		return err
	}

	if luascansStripChapter(parsed) {
		l.URL = parsed.String()
	}

	return nil
}

// FetchTitle fetches and returns the manga title
func (l *Luascans) FetchTitle() (string, error) {
	if l.title != "" {
		return l.title, nil
	}

	if err := l.resolveSeriesURL(); err != nil {
		return "", err
	}

	doc, _, err := l.fetchSeriesPage()
	if err != nil {
		return "", err
	}

	l.title = sanitizeTitle(doc.Find("h1").First().Text())

	return l.title, nil
}

// FetchChapters returns the chapters of the manga, paginating via the API's
// meta.last_page since a single perPage=200 call would silently truncate
// series with 200+ chapters.
func (l *Luascans) FetchChapters() (Filterables, []error) {
	if err := l.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	api, err := l.apiBase()
	if err != nil {
		return nil, []error{err}
	}

	seriesID, err := l.fetchSeriesID()
	if err != nil {
		return nil, []error{err}
	}

	return walkLuascansChapters(func(page int) (string, error) {
		return http.GetText(http.RequestParams{
			URL: fmt.Sprintf(
				"%s/chapter/query?page=%d&perPage=200&query=&order=desc&series_id=%s",
				api,
				page,
				seriesID,
			),
			Referer: l.URL,
		})
	})
}

// walkLuascansChapters requests the chapters feed page by page until the api
// says there's nothing left. It takes the fetcher as an argument so the
// pagination itself is testable without hitting the network.
func walkLuascansChapters(fetchPage func(page int) (string, error)) (chapters Filterables, errs []error) {
	// dedup by slug: the feed is desc-ordered, so a chapter published
	// between requests shifts the window and re-serves rows we already
	// have; also guards against an api that stops honouring `page`, which
	// would otherwise repeat rows last_page times.
	seen := map[string]bool{}

	for page := 1; ; page++ {
		body, err := fetchPage(page)
		if err != nil {
			return chapters, append(errs, err)
		}

		pageChapters, feed, err := parseLuascansChaptersPage(body)
		if err != nil {
			return chapters, append(errs, err)
		}
		// an empty page also stops the loop, in case the api ever reports a
		// last_page beyond the actual data
		if len(feed.Data) == 0 {
			return chapters, errs
		}

		// computed from raw feed rows, not pageChapters, so a page whose
		// names all fail to parse still counts as progress
		fresh := make(map[string]bool, len(feed.Data))
		for _, c := range feed.Data {
			if !seen[c.Slug] {
				seen[c.Slug] = true
				fresh[c.Slug] = true
			}
		}
		if len(fresh) == 0 {
			return chapters, errs
		}

		for _, c := range pageChapters {
			if fresh[c.Slug] {
				chapters = append(chapters, c)
			}
		}

		if page >= feed.Meta.LastPage {
			return chapters, errs
		}
	}
}

// parseLuascansChaptersPage parses one page of the chapters feed, returning
// its chapters plus the decoded feed (meta.last_page drives pagination)
func parseLuascansChaptersPage(body string) (chapters []*LuascansChapter, feed luascansChaptersFeed, err error) {
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, feed, err
	}

	for _, c := range feed.Data {
		number, ok := parseChapterNumber(c.Name)
		if !ok {
			continue
		}
		chapters = append(chapters, &LuascansChapter{
			Chapter{
				Number: number,
				Title:  c.Name,
				ID:     c.Slug,
				Locked: luascansLocked(c.Price, c.FreeAt),
			},
			c.Slug,
		})
	}

	return chapters, feed, nil
}

// luascansLocked reports a paid chapter still before its free date. A site
// that lists no free date locks every priced chapter
func luascansLocked(price float64, freeAt *time.Time) bool {
	return price > 0 && (freeAt == nil || freeAt.After(time.Now()))
}

// FetchChapter fetches a chapter and its pages
func (l *Luascans) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LuascansChapter)
	slug, err := l.seriesSlug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(l.BaseUrl(), "series", slug, lchap.Slug)
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: l.URL,
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
		Number:   f.GetNumber(),
		Language: "en",
		ID:       lchap.Slug,
	}

	for i, src := range luascansPageURLs(doc) {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for chapter %q (it may be a premium/paywalled chapter)", lchap.Slug)
	}

	return chapter, nil
}

// luascansPageURLs returns the page images of a reader page, in order. It's
// scoped under div.container so the footer logo isn't matched; omegascans.org
// marks its pages with data-fallback-index instead
func luascansPageURLs(doc *goquery.Document) []string {
	var srcs []string
	doc.Find("div.container > div.flex.flex-col.justify-center.items-center > img, img[data-fallback-index]").Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" {
			src = s.AttrOr("data-src", "")
		}
		if src != "" {
			srcs = append(srcs, src)
		}
	})

	return srcs
}

// seriesSlug returns the series slug from the URL (i.e. "one-piece" for
// https://luacomic.org/series/one-piece)
func (l *Luascans) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/series/([^/]+)`)
	matches := re.FindStringSubmatch(l.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", l.URL)
	}
	return matches[1], nil
}

// fetchSeriesPage fetches and parses the series page HTML, returning both the
// goquery document (for the title) and the raw HTML (for the series id,
// which lives inside an inline React Server Components payload script, not a
// regular DOM attribute)
func (l *Luascans) fetchSeriesPage() (*goquery.Document, string, error) {
	if l.seriesPage == "" {
		body, err := http.GetText(http.RequestParams{
			URL: l.URL,
		})
		if err != nil {
			return nil, "", err
		}
		l.seriesPage = body
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(l.seriesPage))
	if err != nil {
		return nil, "", err
	}

	return doc, l.seriesPage, nil
}

// seriesIDRe extracts the numeric series id from the series page's inline
// `self.__next_f.push(...)` React Server Components payload, e.g.
// `{\"series_id\":312,\"series_type\":\"Comic\"`
var seriesIDRe = regexp.MustCompile(`series_id\\":(\d+)`)

// fetchSeriesID fetches and caches the numeric series id used by the
// chapters JSON API
func (l *Luascans) fetchSeriesID() (string, error) {
	if l.seriesID != "" {
		return l.seriesID, nil
	}

	_, body, err := l.fetchSeriesPage()
	if err != nil {
		return "", err
	}

	matches := seriesIDRe.FindStringSubmatch(body)
	if len(matches) != 2 {
		return "", errors.New("could not find the series id in the series page")
	}

	l.seriesID = matches[1]

	return l.seriesID, nil
}

// luascansChaptersFeed is the JSON feed for the paginated chapters list
type luascansChaptersFeed struct {
	Meta struct {
		Total    int `json:"total"`
		LastPage int `json:"last_page"`
	} `json:"meta"`
	Data []struct {
		Name string `json:"chapter_name"`
		Slug string `json:"chapter_slug"`
		// coin price, 0 once the chapter is free
		Price float64 `json:"price"`
		// when a priced chapter becomes free, null or absent on sites that don't list it
		FreeAt *time.Time `json:"free_at"`
	} `json:"data"`
}
