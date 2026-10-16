// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// WeebCentral is a grabber for weebcentral.com. It's an HTMX-based site:
// series pages only render the newest chapter, so the full chapters list
// needs to be fetched from a separate HTML fragment endpoint
// (/series/{id}/full-chapter-list), and chapter reader pages load their
// images from another fragment endpoint that additionally requires a
// `reading_style=long_strip` query param to return all pages in one shot
// (found via tools/probe with PROBE_NETLOG, since a plain curl of the
// hx-get URL alone 400s).
type WeebCentral struct {
	*Grabber
	title string
}

func NewWeebCentral(g *Grabber) *WeebCentral {
	return &WeebCentral{Grabber: g}
}

// WeebCentralChapter represents a WeebCentral Chapter
type WeebCentralChapter struct {
	Chapter
	// URL is the chapter reader page URL
	URL string
}

// weebCentralHostRe matches weebcentral.com and its subdomains
var weebCentralHostRe = regexp.MustCompile(`(?i)(^|\.)weebcentral\.com$`)

// weebCentralSeriesRe extracts the series ULID from a series URL, i.e.
// "01J76XYDXH7KT6AABVG3JAT3ZP" from
// https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Some-Manga
var weebCentralSeriesRe = regexp.MustCompile(`/series/([A-Za-z0-9]+)`)

// weebCentralSeriesLinkRe matches a link to a series page by its ULID.
var weebCentralSeriesLinkRe = regexp.MustCompile(`/series/[0-9A-Za-z]{26}(?:/|$)`)

// weebCentralChapterRe matches chapter URLs like
// https://weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R (www. included)
var weebCentralChapterRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?weebcentral\.com/chapters/([A-Za-z0-9]+)`)

// weebCentralChapterID returns the chapter's ULID, the ID weebcentral gives
// each chapter, or "" when the URL isn't a chapter URL
func weebCentralChapterID(rawURL string) string {
	m := weebCentralChapterRe.FindStringSubmatch(rawURL)
	if m == nil {
		return ""
	}

	return m[1]
}

// weebCentralChapterURL checks if the URL is a chapter URL.
func weebCentralChapterURL(url string) bool {
	return weebCentralChapterRe.MatchString(url)
}

// weebCentralResolveSeriesURL finds the series URL from a chapter page.
func weebCentralResolveSeriesURL(chapterURL string) (string, error) {
	if !weebCentralChapterURL(chapterURL) {
		return "", nil
	}

	body, err := http.Get(http.RequestParams{
		URL: chapterURL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	// The nav also links /series/random and the like, so only a 26-character
	// ULID counts.
	base, _ := url.Parse(chapterURL)
	var seriesURL string
	doc.Find(`a[href*="/series/"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, _ := s.Attr("href")
		if !weebCentralSeriesLinkRe.MatchString(href) {
			return true
		}
		if u, err := base.Parse(href); err == nil {
			seriesURL = u.String()
		}
		return false
	})
	if seriesURL == "" {
		return "", errSeriesNotFound(chapterURL)
	}

	return seriesURL, nil
}

// Test returns true if the URL is a weebcentral.com URL. It only checks the
// hostname (no fetch) so it can be tried early without extra requests.
func (w *WeebCentral) Test() (bool, error) {
	u, err := url.Parse(w.URL)
	if err != nil {
		return false, nil
	}

	return weebCentralHostRe.MatchString(u.Hostname()), nil
}

// FetchTitle fetches and returns the manga title
func (w *WeebCentral) FetchTitle() (string, error) {
	if w.title != "" {
		return w.title, nil
	}

	if err := w.resolveSeriesURL(); err != nil {
		return "", err
	}

	body, err := http.Get(http.RequestParams{
		URL: w.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	w.title = sanitizeTitle(doc.Find("h1").First().Text())

	return w.title, nil
}

// resolveSeriesURL resolves a chapter URL to the series URL if needed.
func (w *WeebCentral) resolveSeriesURL() error {
	if !weebCentralChapterURL(w.URL) {
		return nil
	}

	seriesURL, err := weebCentralResolveSeriesURL(w.URL)
	if err != nil {
		return err
	}

	w.URL = seriesURL
	return nil
}

// FetchChapters returns the chapters of the manga
func (w *WeebCentral) FetchChapters() (chapters Filterables, errs []error) {
	if err := w.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	id, err := w.seriesID()
	if err != nil {
		return nil, []error{err}
	}

	uri, _ := url.JoinPath(w.BaseUrl(), "series", id, "full-chapter-list")
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: w.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find(`a[href*="/chapters/"]`).Each(func(i int, s *goquery.Selection) {
		href := strings.TrimSpace(s.AttrOr("href", ""))
		if href == "" {
			return
		}
		// the fragment switched from absolute to relative chapter hrefs
		if strings.HasPrefix(href, "/") {
			href, _ = url.JoinPath(w.BaseUrl(), href)
		}

		number, ok := parseChapterNumber(s.Text())
		if !ok {
			return
		}

		chapters = append(chapters, &WeebCentralChapter{
			Chapter{
				Number: number,
				ID:     weebCentralChapterID(href),
				Label:  Label{Style: LabelNumbered},
			},
			href,
		})
	})

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (w *WeebCentral) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WeebCentralChapter)

	uri := fmt.Sprintf("%s/images?is_prev=False&current_page=1&reading_style=long_strip", wchap.URL)
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: wchap.URL,
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
		Language: "en",
		ID:       weebCentralChapterID(wchap.URL),
	}

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// seriesID returns the series ULID from the URL (i.e.
// "01J76XYDXH7KT6AABVG3JAT3ZP" for
// https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Some-Manga)
func (w *WeebCentral) seriesID() (string, error) {
	matches := weebCentralSeriesRe.FindStringSubmatch(w.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series id in url %s", w.URL)
	}
	return matches[1], nil
}
