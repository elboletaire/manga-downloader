// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// GenzToon is a grabber for genztoons.org (Genz Toon, formerly
// genzupdates.com). The chapters list is plain, static HTML (all chapters are rendered server-side in a
// `#chapters` container, no pagination/ajax), but the reader page's page
// images are lazy-loaded: each `<img class="myImage">` only carries a `uid`
// attribute and a placeholder `src`, and the site's own inline JS builds the
// real URL client-side as `https://cdn.meowing.org/uploads/{uid}` (found by
// grepping the reader page's inline `<script>` for the `uid` variable). No
// browser is needed: the images are plain, direct-fetchable WebPs once that
// URL is built, without cookies or a referer. Newest 1-2 chapters can be
// "early access" (locked behind coins, empty `#chapters_panel` reader with
// no `.myImage` tags) while older ones are free.
type GenzToon struct {
	*Grabber
	title string
}

func NewGenzToon(g *Grabber) *GenzToon {
	return &GenzToon{Grabber: g}
}

// GenzToonChapter represents a GenzToon Chapter
type GenzToonChapter struct {
	Chapter
	// URL is the chapter reader page URL
	URL string
}

// genzToonHostRe matches genztoons.org and its former domain genzupdates.com
// (which redirects there), subdomains included
var genzToonHostRe = regexp.MustCompile(`(?i)(^|\.)(genztoons\.org|genzupdates\.com)$`)

// genzToonLockSelector matches the lock icon a paid row overlays on its
// thumbnail. The `c` attribute isn't a reliable price, and the lock-open
// icon is a different file.
const genzToonLockSelector = `img[src*="material-symbols:lock.svg"]`

// genzToonChapterPathRe matches the /chapter/ path prefix of reader pages.
var genzToonChapterPathRe = regexp.MustCompile(`(?i)^/chapter/`)

// genzToonSeriesPathRe matches the /series/ path prefix of series pages.
var genzToonSeriesPathRe = regexp.MustCompile(`(?i)^/series/`)

// genzToonUidRe extracts the uid used to build a page's image URL from the
// inline reader script, i.e. `https://cdn.meowing.org/uploads/${uid}`
var genzToonImageBaseRe = regexp.MustCompile(`https://([\w.-]+)/uploads/\$\{uid\}`)

// Test returns true if the URL is a genztoons.org URL. It only checks the
// hostname (no fetch) so it can be tried early without extra requests.
func (g *GenzToon) Test() (bool, error) {
	u, err := url.Parse(g.URL)
	if err != nil {
		return false, nil
	}

	return genzToonHostRe.MatchString(u.Hostname()), nil
}

// resolveSeriesURL rewrites g.URL from a chapter URL to its series URL.
// Reader pages nest the series title in `<h1><span><a href="/series/…">`.
// The title is extracted from the link text so FetchTitle needs no extra fetch.
// For series URLs it is a no-op.
func (g *GenzToon) resolveSeriesURL() error {
	u, err := url.Parse(g.URL)
	if err != nil || !genzToonChapterPathRe.MatchString(u.Path) {
		return nil
	}

	body, err := http.Get(http.RequestParams{URL: g.URL})
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}

	link := doc.Find("h1 a[href]").First()
	href, ok := link.Attr("href")
	if !ok || !genzToonSeriesPathRe.MatchString(href) {
		return errSeriesNotFound(g.URL)
	}

	// Cache the title from the link text (just the series name, not "- Chapter N").
	g.title = sanitizeTitle(link.Text())

	base, _ := url.Parse(g.URL)
	seriesURL, _ := base.Parse(href)
	g.URL = seriesURL.String()
	return nil
}

// FetchTitle fetches and returns the manga title
func (g *GenzToon) FetchTitle() (string, error) {
	if g.title != "" {
		return g.title, nil
	}

	if err := g.resolveSeriesURL(); err != nil {
		return "", err
	}

	if g.title != "" {
		return g.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: g.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	g.title = sanitizeTitle(doc.Find("h1").First().Text())

	return g.title, nil
}

// FetchChapters returns the chapters of the manga
func (g *GenzToon) FetchChapters() (chapters Filterables, errs []error) {
	if err := g.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	body, err := http.Get(http.RequestParams{
		URL: g.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find("#chapters a[href]").Each(func(i int, s *goquery.Selection) {
		href := strings.TrimSpace(s.AttrOr("href", ""))
		if href == "" {
			return
		}

		title := strings.TrimSpace(s.AttrOr("title", ""))
		if title == "" {
			title = strings.TrimSpace(s.Text())
		}

		number, ok := parseChapterNumber(title)
		if !ok {
			return
		}

		if u, err := url.Parse(href); err == nil && !u.IsAbs() {
			href, _ = url.JoinPath(g.BaseUrl(), href)
		}

		chapters = append(chapters, &GenzToonChapter{
			Chapter{
				Number: number,
				Title:  title,
				ID:     chapterURLID(href),
				Locked: s.Find(genzToonLockSelector).Length() > 0,
			},
			href,
		})
	})

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (g GenzToon) FetchChapter(f Filterable) (*Chapter, error) {
	gchap := f.(*GenzToonChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     gchap.URL,
		Referer: g.URL,
	})
	if err != nil {
		return nil, err
	}

	// the reader page renders every page as `<img class="myImage" uid="...">`
	// with a placeholder src; the real image url is built client-side by an
	// inline script as `https://cdn.meowing.org/uploads/${uid}`
	imageBase := "cdn.meowing.org"
	if match := genzToonImageBaseRe.FindStringSubmatch(body); len(match) == 2 {
		imageBase = match[1]
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       chapterURLID(gchap.URL),
	}

	doc.Find("img.myImage[uid]").Each(func(i int, s *goquery.Selection) {
		uid := strings.TrimSpace(s.AttrOr("uid", ""))
		if uid == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    "https://" + imageBase + "/uploads/" + uid,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, &genzToonLockedChapterError{number: chapter.Number}
	}

	return chapter, nil
}

// genzToonLockedChapterError signals a chapter with no page images, which on
// GenzToon means it's an "early access" chapter still locked behind coins
type genzToonLockedChapterError struct {
	number float64
}

func (e *genzToonLockedChapterError) Error() string {
	return "chapter " + strconv.FormatFloat(e.number, 'f', -1, 64) + " has no pages (likely locked behind early-access coins)"
}
