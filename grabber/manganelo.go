// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Manganelo is a grabber for the manganelo/manganato family of sites:
// mangabats.com (former mangabat.com), mangakakalot.gg and natomanga.com
// (former manganato.com/manganelo.com). They all run the same platform:
// the series page only renders the newest 50 chapter rows (plus a
// "SHOW MORE" button), so scraping the DOM silently truncates long series —
// the full list comes from the /api/manga/{slug}/chapters JSON API, itself
// paginated at 50 per call unless a bigger limit is requested. Reader pages
// define their images in js variables (cdn hosts + relative image paths).
//
// mangakakalot.gg and natomanga.com additionally sit behind a Cloudflare
// challenge on their HTML pages (though not on the chapters API), so the
// series page fetch tries plain HTTP first and falls back to a single
// browser render, which harvests the clearance cookies into the shared http
// session; the reader pages and images then keep downloading over plain
// HTTP (mangapark's fetchSeriesData pattern). mangabats.com serves plain
// HTTP unchallenged today, so it never touches the browser.
type Manganelo struct {
	*Grabber
	title string
}

func NewManganelo(g *Grabber) *Manganelo {
	return &Manganelo{Grabber: g}
}

// ManganeloChapter represents a Manganelo Chapter
type ManganeloChapter struct {
	Chapter
	Slug string
}

// manganeloHostRe matches the family domains and their subdomains
var manganeloHostRe = regexp.MustCompile(`(?i)(^|\.)(mangabats\.com|mangakakalot\.gg|natomanga\.com)$`)

// manganeloSeriesWait is a selector that exists only on a rendered series
// page, never on a Cloudflare interstitial: waiting on something the
// challenge page also has (e.g. `body`) would make the challenge look like a
// successful render and the browser package's headless→visible escalation
// would never fire.
const manganeloSeriesWait = ".chapter-list a"

// manganeloChaptersLimit is the per-request page size asked of the chapters
// API. The API defaults to 50 (what the series page shows) but honors much
// bigger limits, so even the longest series comes back in a single call; the
// pagination loop is only a fallback in case a cap is ever enforced
// server-side. Not a round 1000: a WAF rule on these sites 403s exactly
// `limit=1000` (999, 2000 and 9999 all pass — verified on all three domains).
const manganeloChaptersLimit = 9999

// Test returns true if the URL hostname is one of the manganelo family
// domains. It only checks the hostname (no fetch) so it can be tried early
// without extra requests.
func (m *Manganelo) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, nil
	}

	return manganeloHostRe.MatchString(u.Hostname()), nil
}

// FetchTitle fetches and returns the manga title. This is the only request
// in the grabber that may need a browser, so it is also what warms up the
// Cloudflare clearance for the plain HTTP requests that follow.
func (m *Manganelo) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	// plain HTTP first: it's a single fast request, and it's all that's
	// needed whenever the challenge happens to be off (always, on mangabats)
	body, err := http.GetText(http.RequestParams{URL: m.URL})
	if err != nil {
		// blocked (403 "Just a moment..."): render it, which also harvests
		// the clearance cookies into http/session.go for the reader pages
		// FetchChapter fetches over plain HTTP afterwards
		body, err = browser.GetHTML(m.URL, manganeloSeriesWait, 0)
		if err != nil {
			return "", err
		}
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find("h1").First().Text())
	if m.title == "" {
		return "", errors.New("could not find the title in the series page")
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga, fetched from the
// paginated chapters API (the series page DOM only carries the newest 50)
func (m *Manganelo) FetchChapters() (Filterables, []error) {
	slug, err := m.slug()
	if err != nil {
		return nil, []error{err}
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "api", "manga", slug, "chapters")

	chapters := Filterables{}
	for offset := 0; ; {
		body, err := http.GetText(http.RequestParams{
			URL:     fmt.Sprintf("%s?limit=%d&offset=%d", uri, manganeloChaptersLimit, offset),
			Referer: m.URL,
		})
		if err != nil {
			return nil, []error{err}
		}

		page, hasMore, err := parseManganeloChaptersPage(body)
		if err != nil {
			return nil, []error{err}
		}
		chapters = append(chapters, page...)

		if !hasMore {
			break
		}
		if len(page) == 0 {
			// defensive: don't loop forever on a has_more that never ends
			return nil, []error{errors.New("chapters API reported more pages but returned none")}
		}
		offset += len(page)
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Manganelo) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*ManganeloChapter)
	slug, err := m.slug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(m.BaseUrl(), "manga", slug, mchap.Slug)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	// images are defined in js variables: cdns hosts and relative image paths
	cdns, err := jsStringSlice(body, "cdns")
	if err != nil {
		return nil, err
	}
	if len(cdns) == 0 {
		return nil, errors.New("no image cdns found in the chapter page")
	}
	images, err := jsStringSlice(body, "chapterImages")
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(images)),
		Language:   "en",
	}

	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    strings.TrimRight(cdns[0], "/") + "/" + strings.TrimLeft(img, "/"),
		})
	}

	return chapter, nil
}

// slug returns the manga slug from the URL (i.e. "one-piece" for
// https://www.mangabats.com/manga/one-piece)
func (m Manganelo) slug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/]+)`)
	matches := re.FindStringSubmatch(m.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", m.URL)
	}
	return matches[1], nil
}

// jsStringSlice extracts a js string array variable from the passed html
func jsStringSlice(html, varname string) (values []string, err error) {
	re := regexp.MustCompile(`var ` + varname + ` = (\[[^\]]*\]);`)
	matches := re.FindStringSubmatch(html)
	if len(matches) != 2 {
		return nil, fmt.Errorf("could not find the %q variable in the chapter page", varname)
	}

	err = json.Unmarshal([]byte(matches[1]), &values)

	return
}

// parseManganeloChaptersPage parses one chapters API response into chapters
// and reports whether more pages remain after it
func parseManganeloChaptersPage(body string) (chapters Filterables, hasMore bool, err error) {
	feed := manganeloChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, false, err
	}
	if !feed.Success {
		msg := feed.Message
		if msg == "" {
			msg = "chapters API reported a failure"
		}
		return nil, false, errors.New(msg)
	}

	chapters = make(Filterables, 0, len(feed.Data.Chapters))
	for _, c := range feed.Data.Chapters {
		title := c.Name
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		chapters = append(chapters, &ManganeloChapter{
			Chapter{
				Number: c.Number,
				Title:  title,
			},
			c.Slug,
		})
	}

	return chapters, feed.Data.Pagination.HasMore, nil
}

// manganeloChaptersFeed is the JSON feed for the chapters list
type manganeloChaptersFeed struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Chapters []struct {
			Name   string  `json:"chapter_name"`
			Slug   string  `json:"chapter_slug"`
			Number float64 `json:"chapter_num"`
		} `json:"chapters"`
		Pagination struct {
			Total   int  `json:"total"`
			Limit   int  `json:"limit"`
			Offset  int  `json:"offset"`
			HasMore bool `json:"has_more"`
		} `json:"pagination"`
	} `json:"data"`
}
