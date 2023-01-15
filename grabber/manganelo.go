package grabber

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// Manganelo is a grabber for manganelo and similar pages
type Manganelo struct {
	*Grabber
	doc  *goquery.Document
	rows *goquery.Selection
}

// ManganeloChapter represents a Manganelo Chapter
type ManganeloChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a valid Manganelo URL
func (m *Manganelo) Test() (bool, error) {
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

	// manganelo style
	m.rows = m.doc.Find("div.panel-story-chapter-list .row-content-chapter li")
	if m.rows.Length() > 0 {
		return true, nil
	}
	// mangakakalot style
	m.rows = m.doc.Find("div.chapter-list div.row")

	return m.rows.Length() > 0, nil
}

// Ttitle returns the manga title
func (m Manganelo) FetchTitle() (string, error) {
	return m.doc.Find("h1").Text(), nil
}

// FetchChapters returns a slice of chapters
func (m Manganelo) FetchChapters() (chapters Filterables, errs []error) {
	m.rows.Each(func(i int, s *goquery.Selection) {
		re := regexp.MustCompile(`(\d+\.?\d*)`)
		num := re.FindString(s.Find("a").Text())
		number, err := strconv.ParseFloat(num, 64)
		if err != nil {
			errs = append(errs, err)
			return
		}
		u := s.Find("a").AttrOr("href", "")
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}
		chapter := &ManganeloChapter{
			Chapter{
				Number: number,
				Title:  s.Find("a").Text(),
			},
			u,
		}

		chapters = append(chapters, chapter)
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m Manganelo) FetchChapter(f Filterable) (*Chapter, error) {
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

	pimages := doc.Find("div.container-chapter-reader img")
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pimages.Length()),
		Language:   "en",
	}

	// get the chapter pages
	doc.Find("div.container-chapter-reader img").Each(func(i int, s *goquery.Selection) {
		u := s.AttrOr("src", "")
		n := int64(i)
		if u == "" {
			// this error is not critical and is not from our side, so just log it out
			color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", n, chapter.GetTitle())
			return
		}
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}
		page := Page{
			Number: n,
			URL:    u,
		}
		chapter.Pages = append(chapter.Pages, page)
	})

	return chapter, nil
}
