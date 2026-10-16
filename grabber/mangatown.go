// Copyright (C) 2023-2026 Òscar Casajuana Alonso

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

// Mangatown is a grabber for mangatown.com. It shares its reader with
// mangahere.cc, but without the chapterfun.ashx round trip: every reader page
// is its own HTML document (<chapter>/<n>.html) with the page's image in
// <img id="image">, and the page count sits in a "total_pages" script
// variable of the chapter's first page. Each page is therefore one plain GET.
type Mangatown struct {
	*Grabber
}

func NewMangatown(g *Grabber) *Mangatown {
	return &Mangatown{Grabber: g}
}

// MangatownChapter represents a Mangatown chapter
type MangatownChapter struct {
	Chapter
	URL string
}

var mangatownURLRe = regexp.MustCompile(`^https?://(?:www\.|m\.)?mangatown\.com(/.*)?$`)

// mangatownSeriesPathRe takes the series path out of a series, chapter or
// reader page path: /manga/<slug>/[v<volume>/]c<number>/[<page>.html]
var mangatownSeriesPathRe = regexp.MustCompile(`^(/manga/[^/?#]+)(?:/|$)`)

// mangahereHostRe matches any mangahere.cc host: bare, www., m., newm., etc.
var mangahereHostRe = regexp.MustCompile(`(?i)(?:^|\.)mangahere\.cc$`)

// mangahereChapterURL reports whether rawURL is a chapter/reader URL on
// mangahere.cc or fanfox.net. It extracts the /manga/<slug> prefix via
// mangatownSeriesPathRe and checks that more path follows, so volume segments
// (/v98/) and decimal chapter numbers (/c001.1/) are accepted.
func mangahereChapterURL(rawURL string) bool {
	u, _ := url.Parse(rawURL)
	if u == nil {
		return false
	}
	m := mangatownSeriesPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return false
	}
	return strings.TrimPrefix(u.Path[len(m[1]):], "/") != ""
}

// mangahereSeriesURL extracts the series URL from a chapter URL, trimming the
// path to /manga/<slug>/ and stripping query and fragment. Any *.mangahere.cc
// host (m., newm., bare) is rewritten to www.mangahere.cc. Returns ("", false)
// for series URLs (nothing after the slug).
func mangahereSeriesURL(rawURL string) (string, bool) {
	u, _ := url.Parse(rawURL)
	if u == nil {
		return "", false
	}
	if mangahereHostRe.MatchString(u.Hostname()) {
		u.Host = "www.mangahere.cc"
	}
	m := mangatownSeriesPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	if strings.TrimPrefix(u.Path[len(m[1]):], "/") == "" {
		return "", false
	}
	u.Path = m[1] + "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), true
}

// Test returns true if the URL is a mangatown.com URL. The mobile host serves
// the same paths, but its markup differs, so the URL is moved to www. A chapter
// or reader page URL is cut down to its series, which is what the grabber lists.
func (m *Mangatown) Test() (bool, error) {
	match := mangatownURLRe.FindStringSubmatch(m.URL)
	if match == nil {
		return false, nil
	}

	path := match[1]
	if series := mangatownSeriesPathRe.FindStringSubmatch(path); series != nil {
		path = series[1] + "/"
	}
	m.URL = "https://www.mangatown.com" + path
	return true, nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangatown) FetchTitle() (string, error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find("h1.title-top").Text()), nil
}

// mangatownChapterNumberRe takes the chapter number from the URL rather than
// the link text, which is not always a number: the path is
// /manga/<slug>/[v<volume>/]c<number>[.<part>]/, e.g. v98/c1196 or c009.5.
var mangatownChapterNumberRe = regexp.MustCompile(`/c(\d+(?:\.\d+)?)/?$`)

// urlVolumeRe matches the volume segment of a mangatown, mangahere or fanfox
// chapter URL: the v98 of /manga/one_piece/v98/c1195/. Chapters not yet in a
// volume are filed under v00 or vTBD, which parseVolume reads as unknown.
var urlVolumeRe = regexp.MustCompile(`/(v[^/]+)/c\d+(?:\.\d+)?(?:/|$)`)

// urlVolume returns the volume of a chapter URL, 0 when it has none
func urlVolume(href string) int {
	match := urlVolumeRe.FindStringSubmatch(href)
	if match == nil {
		return 0
	}
	return parseVolume(match[1])
}

// mangatownChapterNumber returns the chapter number of a chapter URL
func mangatownChapterNumber(href string) (float64, bool) {
	match := mangatownChapterNumberRe.FindStringSubmatch(href)
	if len(match) != 2 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// mangatownChapterTitleNumberRe matches a link text that is only the number
var mangatownChapterTitleNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

// mangatownChapterTitle returns the chapter title from its link text, which
// repeats the series title before the number ("<series> 10"). The series prefix
// is dropped, and a title left with only the number is empty.
func mangatownChapterTitle(seriesTitle, text string) string {
	title := sanitizeTitle(text)
	if seriesTitle != "" && len(title) >= len(seriesTitle) && strings.EqualFold(title[:len(seriesTitle)], seriesTitle) {
		title = strings.TrimSpace(title[len(seriesTitle):])
	}

	if mangatownChapterTitleNumberRe.MatchString(title) {
		return ""
	}

	return title
}

// FetchChapters returns the chapters of the manga
func (m Mangatown) FetchChapters() (chapters Filterables, errs []error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	seriesTitle := sanitizeTitle(doc.Find("h1.title-top").Text())
	doc.Find("ul.chapter_list li a").Each(func(i int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		number, ok := mangatownChapterNumber(href)
		if !ok {
			return
		}

		if !strings.HasPrefix(href, "http") {
			href = m.BaseUrl() + href
		}

		chapters = append(chapters, &MangatownChapter{
			Chapter{
				Number: number,
				Title:  mangatownChapterTitle(seriesTitle, s.Text()),
				Volume: urlVolume(href),
				Label:  Label{Style: LabelNumbered},
				ID:     chapterURLID(href),
			},
			href,
		})
	})

	return
}

var mangatownPagesCountRe = regexp.MustCompile(`total_pages\s*=\s*(\d+)`)

// FetchChapter fetches a chapter and its pages
func (m Mangatown) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangatownChapter)

	// the chapter URL is the first reader page, so its image is taken from
	// this response and the loop starts at the second one
	body, err := http.GetText(http.RequestParams{
		URL:     mchap.URL,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	countMatch := mangatownPagesCountRe.FindStringSubmatch(body)
	if len(countMatch) != 2 {
		return nil, fmt.Errorf("mangatown: could not find total_pages in %s", mchap.URL)
	}
	count, err := strconv.Atoi(countMatch[1])
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(count),
		Language:   "en",
		ID:         chapterURLID(mchap.URL),
	}

	first, err := mangatownPageImage(body)
	if err != nil {
		return nil, fmt.Errorf("%w in %s", err, mchap.URL)
	}
	chapter.Pages = append(chapter.Pages, Page{
		Number: 1,
		URL:    first,
	})

	base := strings.TrimSuffix(mchap.URL, "/")
	for page := 2; page <= count; page++ {
		uri := fmt.Sprintf("%s/%d.html", base, page)
		res, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: mchap.URL,
		})
		if err != nil {
			return nil, err
		}

		img, err := mangatownPageImage(res)
		if err != nil {
			return nil, fmt.Errorf("%w in %s", err, uri)
		}

		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(page),
			URL:    img,
		})
	}

	return chapter, nil
}

// mangatownPageImage returns the image URL of a reader page
func mangatownPageImage(res string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res))
	if err != nil {
		return "", err
	}

	img := doc.Find("img#image").AttrOr("src", "")
	if img == "" {
		return "", fmt.Errorf("mangatown: could not find the page image")
	}
	if strings.HasPrefix(img, "//") {
		img = "https:" + img
	}

	return img, nil
}
