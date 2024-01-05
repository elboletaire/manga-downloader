package grabber

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
	"golang.org/x/net/html"
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

	// mangajar has ajax pagination
	if m.doc.Find(".chapters-infinite-pagination .pagination .page-item").Length() > 0 {
		var err error
		var fetchChaps func(page int)
		rows := &goquery.Selection{
			Nodes: []*html.Node{},
		}

		fetchChaps = func(page int) {
			rbody, err := http.Get(http.RequestParams{
				URL: fmt.Sprintf("%s/chaptersList?page=%d", m.URL, page),
			})
			if err != nil {
				return
			}
			defer rbody.Close()

			doc, err := goquery.NewDocumentFromReader(rbody)
			if err != nil {
				return
			}

			rows = rows.AddNodes(doc.Find(".chapter-list-container .chapter-item").Nodes...)

			if doc.Find("ul.pagination .page-item:not(.disabled):last-child").Length() > 0 {
				fetchChaps(page + 1)
			}
		}

		fetchChaps(1)
		if err != nil {
			return false, err
		}

		m.rows = rows

		return m.rows.Length() > 0, nil
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
func (m Manganelo) FetchTitle() (string, error) {
	title := m.doc.Find("h1")

	// mangajar has the name inside span.post-name
	if title.Children().HasClass("post-name") {
		title = title.Find(".post-name")
	}

	return title.Text(), nil
}

// FetchChapters returns a slice of chapters
func (m Manganelo) FetchChapters() (chapters Filterables, errs []error) {
	m.rows.Each(func(i int, s *goquery.Selection) {
		re := regexp.MustCompile(`Chapter\s*(\d+\.?\d*)`)
		chap := re.FindStringSubmatch(s.Find("a").Text())
		// if the chapter has no number, we skip it (usually it's an announcement from the site)
		if len(chap) == 0 {
			return
		}

		num := chap[1]
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
	pimages = doc.Find("div.container-chapter-reader img, .chapter-images img")
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
