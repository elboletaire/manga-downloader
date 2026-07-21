package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
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

// FetchChapters returns the chapters of the manga
func (w WeebCentral) FetchChapters() (chapters Filterables, errs []error) {
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

		number, ok := parseChapterNumber(s.Text())
		if !ok {
			return
		}

		chapters = append(chapters, &WeebCentralChapter{
			Chapter{
				Number: number,
				Title:  "Chapter " + strconv.FormatFloat(number, 'f', -1, 64),
			},
			href,
		})
	})

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (w WeebCentral) FetchChapter(f Filterable) (*Chapter, error) {
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
		Number:   f.GetNumber(),
		Language: "en",
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
func (w WeebCentral) seriesID() (string, error) {
	matches := weebCentralSeriesRe.FindStringSubmatch(w.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series id in url %s", w.URL)
	}
	return matches[1], nil
}
