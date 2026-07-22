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

// Stonescape is a grabber for stonescape.xyz: a Vue SPA (the series/reader
// pages render an empty `#app` div), but chapters and page images are served
// by an open JSON API requiring no authentication for free chapters
// (found via tools/probe with PROBE_NETLOG):
//   - GET /api/series/by-slug/{slug}            series metadata (title)
//   - GET /api/series/by-slug/{slug}/chapters    full chapter list, unpaginated
//   - GET /api/chapters/{chapterId}/pages        page image URLs for a chapter
type Stonescape struct {
	*Grabber
	title string
}

func NewStonescape(g *Grabber) *Stonescape {
	return &Stonescape{Grabber: g}
}

// StonescapeChapter represents a Stonescape chapter
type StonescapeChapter struct {
	Chapter
	// Id is the chapter's UUID, used to fetch its pages
	Id string
}

// stonescapeHostRe matches stonescape.xyz and its subdomains
var stonescapeHostRe = regexp.MustCompile(`(?i)(^|\.)stonescape\.xyz$`)

// stonescapeSeriesRe extracts the series slug from a series URL, i.e.
// "only-see-you" from https://stonescape.xyz/series/only-see-you
var stonescapeSeriesRe = regexp.MustCompile(`/series/([^/?#]+)`)

// Test returns true if the URL is a stonescape.xyz URL. It only checks the
// hostname (no fetch) so it can be tried early without extra requests.
func (s *Stonescape) Test() (bool, error) {
	u, err := url.Parse(s.URL)
	if err != nil {
		return false, nil
	}

	return stonescapeHostRe.MatchString(u.Hostname()), nil
}

// FetchTitle fetches and returns the manga title
func (s *Stonescape) FetchTitle() (string, error) {
	if s.title != "" {
		return s.title, nil
	}

	slug, err := s.slug()
	if err != nil {
		return "", err
	}

	uri, _ := url.JoinPath(s.BaseUrl(), "api", "series", "by-slug", slug)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: s.URL,
	})
	if err != nil {
		return "", err
	}

	feed := struct {
		Title string `json:"title"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return "", err
	}

	s.title = sanitizeTitle(feed.Title)

	return s.title, nil
}

// FetchChapters returns the chapters of the manga
func (s Stonescape) FetchChapters() (chapters Filterables, errs []error) {
	slug, err := s.slug()
	if err != nil {
		return nil, []error{err}
	}

	uri, _ := url.JoinPath(s.BaseUrl(), "api", "series", "by-slug", slug, "chapters")
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: s.URL,
	})
	if err != nil {
		return nil, []error{err}
	}

	feed := stonescapeChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}

	for _, c := range feed.Chapters {
		number, err := strconv.ParseFloat(c.ChapterNumber, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not parse chapter number %q: %w", c.ChapterNumber, err))
			continue
		}

		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(number, 'f', -1, 64)
		}

		chapters = append(chapters, &StonescapeChapter{
			Chapter{
				Number:   number,
				Title:    title,
				Language: "en",
			},
			c.ChapterId,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (s Stonescape) FetchChapter(f Filterable) (*Chapter, error) {
	schap := f.(*StonescapeChapter)

	uri, _ := url.JoinPath(s.BaseUrl(), "api", "chapters", schap.Id, "pages")
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: s.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Pages []struct {
			PageNumber int64  `json:"pageNumber"`
			Url        string `json:"url"`
		} `json:"pages"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   "en",
		PagesCount: int64(len(feed.Pages)),
	}
	for _, p := range feed.Pages {
		pageUrl := p.Url
		if parsed, err := url.Parse(p.Url); err == nil && !parsed.IsAbs() {
			pageUrl = s.BaseUrl() + p.Url
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: p.PageNumber,
			URL:    pageUrl,
		})
	}

	return chapter, nil
}

// slug returns the series slug from the URL, e.g. "only-see-you" for
// https://stonescape.xyz/series/only-see-you
func (s Stonescape) slug() (string, error) {
	matches := stonescapeSeriesRe.FindStringSubmatch(s.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", s.URL)
	}
	return matches[1], nil
}

// stonescapeChaptersFeed is the JSON feed for the chapters list
type stonescapeChaptersFeed struct {
	Chapters []struct {
		ChapterId     string  `json:"chapterId"`
		ChapterNumber string  `json:"chapterNumber"`
		Title         *string `json:"title"`
	} `json:"chapters"`
}
