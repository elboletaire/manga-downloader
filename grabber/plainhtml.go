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
	selectors := []string{
		// tcbscans.com
		"main .mx-auto .grid .col-span-2 a",
	}

	// for the same priority reasons, we need to iterate over the selectors
	// using a simple `,` joining all selectors would return missmatches
	for _, selector := range selectors {
		rows := m.doc.Find(selector)
		if rows.Length() > 0 {
			m.rows = rows
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
	title := m.doc.Find("h1")

	return title.Text(), nil
}

// FetchChapters returns a slice of chapters
func (m PlainHTML) FetchChapters() (chapters Filterables, errs []error) {
	m.rows.Each(func(i int, s *goquery.Selection) {
		// we need to get the chapter number from the title
		re := regexp.MustCompile(`Chapter\s*(\d+\.?\d*)`)
		chap := re.FindStringSubmatch(s.Find(".font-bold").Text())
		// if the chapter has no number, we skip it (these are usually site announcements)
		if len(chap) == 0 {
			return
		}

		num := chap[1]
		number, err := strconv.ParseFloat(num, 64)
		if err != nil {
			errs = append(errs, err)
			return
		}
		u := s.AttrOr("href", "")
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}
		chapter := &ManganeloChapter{
			Chapter{
				Number: number,
				Title:  s.Find(".text-gray-500").Text(),
			},
			u,
		}

		chapters = append(chapters, chapter)
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m PlainHTML) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*ManganeloChapter)
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

	pimages := getPlainHTMLImageURL(doc)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pimages)),
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

	return chapter, nil
}

func getPlainHTMLImageURL(doc *goquery.Document) []string {
	// images are inside picture objects
	pimages := doc.Find("picture img")
	imgs := []string{}
	pimages.Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" || strings.HasPrefix(src, "data:image") {
			src = s.AttrOr("data-src", "")
		}
		imgs = append(imgs, src)
	})

	return imgs
}
