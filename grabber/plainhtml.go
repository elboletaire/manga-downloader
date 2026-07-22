// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// PlainHTML is a grabber for any plain HTML page (with no ajax pagination whatsoever)
type PlainHTML struct {
	*Grabber
	doc  *goquery.Document
	rows *goquery.Selection
	site SiteSelector
}

func NewPlainHTML(g *Grabber) *PlainHTML {
	return &PlainHTML{Grabber: g}
}

type SiteSelector struct {
	Title        string
	Rows         string
	Link         string
	Chapter      string
	ChapterTitle string
	Image        string
}

// PlainHTMLChapter represents a PlainHTML Chapter
type PlainHTMLChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a valid grabber URL
func (m *PlainHTML) Test() (bool, error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return false, err
	}
	m.doc, err = goquery.NewDocumentFromReader(body)
	if err != nil {
		return false, err
	}

	// order is important, since some sites have very similar selectors
	selectors := []SiteSelector{
		// tcbonepiecechapters.com (former tcbscans.com)
		{
			Title:        "h1",
			Rows:         "main .mx-auto .grid .col-span-2 a",
			Chapter:      ".font-bold",
			ChapterTitle: ".text-gray-500",
			Image:        "picture img",
		},
		// asurascans.com (former asuratoon.com)
		{
			Title:        "h1",
			Rows:         `a[data-astro-prefetch][href*="/chapter/"]`,
			Chapter:      "span.font-medium",
			ChapterTitle: "span.font-medium",
			Image:        "img[data-page-index]",
		},
		// zonatmo.org (TuMangaOnline, former zonatmo.com)
		{
			Title:        "h1.element-title",
			Rows:         "li.upload-link",
			Chapter:      ".chapter-number",
			ChapterTitle: ".chapter-number",
			Link:         ".chapter-detail a.btn-primary",
			Image:        "img.reader-image",
		},
		// mangapill.com: each chapter row is a plain <a> with the chapter
		// number as its own text (no dedicated child selector), hence the
		// empty Chapter/ChapterTitle (see FetchChapters)
		{
			Title: "h1",
			Rows:  "#chapters [data-filter-list] a",
			Image: "img.js-page",
		},
	}

	// for the same priority reasons, we need to iterate over the selectors
	// using a simple `,` joining all selectors would return missmatches
	for _, selector := range selectors {
		rows := m.doc.Find(selector.Rows)
		if rows.Length() > 0 {
			m.rows = rows
			m.site = selector
			break
		}
	}

	if m.rows == nil {
		return false, nil
	}

	return m.rows.Length() > 0, nil
}

// Ttitle returns the manga title
func (m PlainHTML) FetchTitle() (string, error) {
	title := m.doc.Find(m.site.Title).Clone()
	// strip noise commonly nested inside a site's title heading: <small>
	// blocks of alternate-script/alternate-language titles (mangahub.io
	// lists dozens of translations here, long enough to blow past the
	// filesystem's filename length limit) and badge-like labels (e.g.
	// mangahub's "Hot" tag).
	title.Find("small, .manga-label").Remove()

	return sanitizeTitle(title.Text()), nil
}

// chapterNumberRe matches a chapter number in a chapter title, accepting
// "Chapter 10", "Ch. 10", "C. 10", the Spanish "Capítulo 10" and the French
// "Chapitre 10" (case insensitive).
var chapterNumberRe = regexp.MustCompile(`(?i)\b(?:chapter|chapitre|cap[ií]tulo|ch|c)\.?\s*(\d+\.?\d*)`)

// volumeNumberRe matches a volume number ("Volume 18", "Vol. 2", the Spanish
// "Volumen 3"), used as a fallback for sites that list volumes instead of
// chapters (e.g. sushiscan).
var volumeNumberRe = regexp.MustCompile(`(?i)\bvol(?:ume|umen)?\.?\s*(\d+\.?\d*)`)

// urlChapterNumberRe is a last-resort fallback for rows whose visible text
// carries no recognizable chapter/volume keyword (e.g. mangahub.io, whose
// latest-chapter row often just repeats the manga name and number, like
// "One Piece 1187" with no "Chapter" word) but whose reader URL always
// contains "chapter-<number>".
var urlChapterNumberRe = regexp.MustCompile(`(?i)chapter[-_](\d+\.?\d*)`)

// parseChapterNumber extracts the chapter number from a chapter title, falling
// back to a volume number. Returns false if no number could be found (these
// are usually site announcements rather than actual chapters).
func parseChapterNumber(text string) (float64, bool) {
	match := chapterNumberRe.FindStringSubmatch(text)
	if len(match) == 0 {
		// only checked as fallback so "Vol.2 Chapter 15" still prefers the chapter
		match = volumeNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// FetchChapters returns a slice of chapters
func (m PlainHTML) FetchChapters() (chapters Filterables, errs []error) {
	m.rows.Each(func(i int, s *goquery.Selection) {
		// an empty Chapter/ChapterTitle selector means the row itself carries
		// the text (i.e. mangapill, where each chapter row is a plain <a>
		// with no dedicated child element for the chapter number)
		chapterText := s.Text()
		if m.site.Chapter != "" {
			chapterText = s.Find(m.site.Chapter).Text()
		}

		u := s.AttrOr("href", "")
		if m.site.Link != "" {
			u = s.Find(m.site.Link).AttrOr("href", "")
		}

		number, ok := parseChapterNumber(chapterText)
		if !ok {
			// fall back to extracting the number from the reader URL itself
			match := urlChapterNumberRe.FindStringSubmatch(u)
			if len(match) > 0 {
				if n, err := strconv.ParseFloat(match[1], 64); err == nil {
					number = n
					ok = true
				}
			}
		}
		if !ok {
			return
		}

		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}
		title := chapterText
		if m.site.ChapterTitle != "" {
			title = s.Find(m.site.ChapterTitle).Text()
		}
		chapter := &PlainHTMLChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			u,
		}

		chapters = append(chapters, chapter)
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m PlainHTML) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*PlainHTMLChapter)
	body, err := http.Get(http.RequestParams{
		URL: mchap.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	return m.chapterFromDoc(f, doc), nil
}

// chapterFromDoc builds a Chapter from an already parsed reader page
func (m PlainHTML) chapterFromDoc(f Filterable, doc *goquery.Document) *Chapter {
	pimages := getPlainHTMLImageURL(m.site.Image, doc)
	pcount := len(pimages)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   "en",
	}

	for i, img := range pimages {
		if img == "" {
			// this error is not critical and is not from our side, so just log it out
			color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", i, chapter.GetTitle())
			continue
		}
		if !strings.HasPrefix(img, "http") {
			img = m.BaseUrl() + img
		}

		page := Page{
			Number: int64(i),
			URL:    img,
		}
		chapter.Pages = append(chapter.Pages, page)
	}

	return chapter
}

func getPlainHTMLImageURL(selector string, doc *goquery.Document) []string {
	// Check for chapImages JavaScript variable first

	html, _ := doc.Html()
	re := regexp.MustCompile(`var chapImages = '([^']+)'`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		// Found chapImages variable, split by comma
		return strings.Split(matches[1], ",")
	}
	// some sites store a plain text array with the urls into a hidden layer
	pimages := doc.Find("#arraydata")
	if pimages.Length() == 1 {
		return strings.Split(pimages.Text(), ",")
	}

	// mangastream/themesia based readers (e.g. sushiscan) embed all the pages
	// of the chapter in a ts_reader javascript call
	re = regexp.MustCompile(`(?s)ts_reader\.run\(.*?"images":\s*\[(.*?)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, strings.ReplaceAll(u[1], `\/`, `/`))
		}
		return imgs
	}

	// images are inside picture objects
	pimages = doc.Find(selector)

	imgs := []string{}
	pimages.Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" || strings.HasPrefix(src, "data:image") {
			src = s.AttrOr("data-src", "")
		}
		imgs = append(imgs, strings.Trim(src, " \n\r\t")) // trim whitespaces
	})

	return imgs
}

// sanitizeTitle sanitizes titles, trimming and removing extra spaces from titles
func sanitizeTitle(title string) string {
	spaces := regexp.MustCompile(`\s+`)
	title = spaces.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	return title
}
