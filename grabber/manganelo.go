package grabber

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

type Manganelo struct {
	Grabber
	doc  *goquery.Document
	rows *goquery.Selection
}

// Test returns true if the URL is a valid Manganelo URL
func (m *Manganelo) Test() bool {
	body, err := http.Get(http.GetParams{
		URL: m.URL,
	})
	if err != nil {
		panic(err)
	}
	m.doc, err = goquery.NewDocumentFromReader(body)
	if err != nil {
		panic(err)
	}

	// manganelo style
	rows := m.doc.Find("div.panel-story-chapter-list .row-content-chapter li")
	if rows.Length() > 0 {
		m.rows = rows
		return true
	}
	// mangakakalot style
	rows = m.doc.Find("div.chapter-list div.row")
	if rows.Length() > 0 {
		m.rows = rows
		return true
	}

	return false
}

// Ttitle returns the manga title
func (m Manganelo) GetTitle(language string) string {
	return m.doc.Find("h1").Text()
}

// FetchChapters returns a slice of chapters
func (m Manganelo) FetchChapters(language string) Filterables {
	chapters := Filterables{}
	m.rows.Each(func(i int, s *goquery.Selection) {
		re := regexp.MustCompile(`(\d+\.?\d*)`)
		num := re.FindString(s.Find("a").Text())
		number, _ := strconv.ParseFloat(num, 64)
		u := s.Find("a").AttrOr("href", "")
		if !strings.HasPrefix(u, "http") {
			u = m.GetBaseUrl() + u
		}
		chapter := &ManganeloChapter{
			Number: number,
			URL:    u,
			Title:  s.Find("a").Text(),
		}
		if chapter.URL == "" {
			color.Red("chapter %f has no URL to fetch from 😕", chapter.Number)
			return
		}

		chapters = append(chapters, chapter)
	})

	return chapters
}

// FetchChapter fetches a chapter and its pages
func (m Manganelo) FetchChapter(f Filterable) Chapter {
	mchap := f.(*ManganeloChapter)
	body, err := http.Get(http.GetParams{
		URL: mchap.URL,
	})
	if err != nil {
		panic(err)
	}
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		panic(err)
	}

	pimages := doc.Find("div.container-chapter-reader img")
	chapter := Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pimages.Length()),
		Language:   "en",
	}
	var pages Pages
	// get the chapter pages
	doc.Find("div.container-chapter-reader img").Each(func(i int, s *goquery.Selection) {
		u := s.AttrOr("src", "")
		if !strings.HasPrefix(u, "http") {
			u = m.GetBaseUrl() + u
		}
		page := Page{
			Number: int64(i),
			URL:    u,
		}
		if page.URL == "" {
			color.Red("page %d has no URL to fetch from 😕 (will be ignored)", page.Number)
			return
		}
		pages = append(pages, page)
	})

	chapter.Pages = pages
	return chapter
}

type ManganeloChapter struct {
	Number float64
	Title  string
	URL    string
}

func (m *ManganeloChapter) GetNumber() float64 {
	return m.Number
}

func (m *ManganeloChapter) GetTitle() string {
	return m.Title
}
