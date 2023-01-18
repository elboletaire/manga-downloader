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

	// order is important, since some sites have very similar selectors
	selectors := []string{
		// manganelo/manganato style
		"div.panel-story-chapter-list .row-content-chapter li",
		// manganelos style (not using the parent id returns more stuff)
		"#examples div.chapter-list .row",
		// mangakakalot style
		"div.chapter-list .row",
	}

	// for the same priority reasons, we need to iterate over the selectors
	// using a simple `,` joining all selectors would resturn missmatches
	for _, selector := range selectors {
		rows := m.doc.Find(selector)
		if rows.Length() > 0 {
			m.rows = rows
			break
		}
	}

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

	pimages := getImageUrls(doc)

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

func getImageUrls(doc *goquery.Document) []string {
	// some sites store a plain text array with the urls into a hidden layer
	pimages := doc.Find("#arraydata")
	if pimages.Length() == 1 {
		return strings.Split(pimages.Text(), ",")
	}

	// others just have the images
	pimages = doc.Find("div.container-chapter-reader img")
	imgs := []string{}
	pimages.Each(func(i int, s *goquery.Selection) {
		imgs = append(imgs, s.AttrOr("src", s.AttrOr("data-src", "")))
	})

	return imgs
}
