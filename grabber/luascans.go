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

// Luascans is a grabber for luacomic.org: it's a Next.js site whose reader
// pages are server-rendered plain HTML (page images are regular <img> tags),
// but the series page only ships the newest chapters and lazy-loads the full
// chapters list client-side from a JSON API, keyed by a numeric series id
// embedded in the series page's React Server Components payload.
type Luascans struct {
	*Grabber
	title      string
	seriesID   string
	seriesPage string
}

func NewLuascans(g *Grabber) *Luascans {
	return &Luascans{Grabber: g}
}

// LuascansChapter represents a Luascans Chapter
type LuascansChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a luacomic.org URL
func (l *Luascans) Test() (bool, error) {
	re := regexp.MustCompile(`luacomic\.org`)
	return re.MatchString(l.URL), nil
}

// FetchTitle fetches and returns the manga title
func (l *Luascans) FetchTitle() (string, error) {
	if l.title != "" {
		return l.title, nil
	}

	doc, _, err := l.fetchSeriesPage()
	if err != nil {
		return "", err
	}

	l.title = sanitizeTitle(doc.Find("h1").First().Text())

	return l.title, nil
}

// FetchChapters returns the chapters of the manga
func (l *Luascans) FetchChapters() (chapters Filterables, errs []error) {
	seriesID, err := l.fetchSeriesID()
	if err != nil {
		return nil, []error{err}
	}

	uri := fmt.Sprintf(
		"https://api.luacomic.org/chapter/query?page=1&perPage=200&query=&order=desc&series_id=%s",
		seriesID,
	)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: l.URL,
	})
	if err != nil {
		return nil, []error{err}
	}

	feed := luascansChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
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
			},
			c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (l Luascans) FetchChapter(f Filterable) (*Chapter, error) {
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
	}

	// scoped under the reader's own div.container (the only element in the
	// page with that exact class) to avoid also matching the site's footer
	// logo, which sits in an unrelated div sharing the same flex/center
	// utility classes
	doc.Find("div.container > div.flex.flex-col.justify-center.items-center > img").Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" {
			src = s.AttrOr("data-src", "")
		}
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for chapter %q (it may be a premium/paywalled chapter)", lchap.Slug)
	}

	return chapter, nil
}

// seriesSlug returns the series slug from the URL (i.e. "one-piece" for
// https://luacomic.org/series/one-piece)
func (l Luascans) seriesSlug() (string, error) {
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

// luascansChaptersFeed is the JSON feed for the chapters list
type luascansChaptersFeed struct {
	Data []struct {
		Name string `json:"chapter_name"`
		Slug string `json:"chapter_slug"`
	} `json:"data"`
}
