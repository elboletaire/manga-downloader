// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// LeerCapitulo is a grabber for leercapitulo.co (Spanish aggregator, popular
// with ex-TuMangaOnline readers). Series, chapter list and reader pages are
// all plain server-rendered HTML, the reader included: every page image is
// already in the markup as an <img data-src>.
type LeerCapitulo struct {
	*Grabber
	doc *goquery.Document
}

func NewLeerCapitulo(g *Grabber) *LeerCapitulo {
	return &LeerCapitulo{Grabber: g}
}

// LeerCapituloChapter represents a LeerCapitulo Chapter
type LeerCapituloChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a leercapitulo.co URL
func (m *LeerCapitulo) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	return strings.TrimPrefix(u.Hostname(), "www.") == "leercapitulo.co", nil
}

// fetchDoc fetches and caches the manga series page
func (m *LeerCapitulo) fetchDoc() (*goquery.Document, error) {
	if m.doc != nil {
		return m.doc, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	m.doc = doc

	return doc, nil
}

// FetchTitle fetches and returns the manga title
func (m *LeerCapitulo) FetchTitle() (string, error) {
	doc, err := m.fetchDoc()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find("h1.h3").Text()), nil
}

// FetchChapters returns the chapters of the manga
func (m *LeerCapitulo) FetchChapters() (chapters Filterables, errs []error) {
	doc, err := m.fetchDoc()
	if err != nil {
		return nil, []error{err}
	}

	doc.Find("#chapterList a.lc-chapter-row").Each(func(i int, a *goquery.Selection) {
		text := a.Find("span.n").Text()
		number, ok := parseChapterNumber(text)
		if !ok {
			return
		}

		href := a.AttrOr("href", "")
		if !strings.HasPrefix(href, "http") {
			href = m.BaseUrl() + href
		}

		chapters = append(chapters, &LeerCapituloChapter{
			Chapter{
				Number: number,
				Title:  sanitizeTitle(text),
			},
			href,
		})
	})

	return
}

// leerCapituloImageSelector matches every page of the reader, which the
// server renders in full (pages are lazy-loaded via data-src)
const leerCapituloImageSelector = "#lcPages img"

// FetchChapter fetches the chapter reader page over plain HTTP, which already
// contains every page image, and extracts its pages
func (m *LeerCapitulo) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*LeerCapituloChapter)

	body, err := http.Get(http.RequestParams{
		URL: chap.URL,
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
		Language: "es",
	}

	doc.Find(leerCapituloImageSelector).Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("data-src", "")
		if src == "" {
			src = s.AttrOr("data-original", "")
		}
		if src == "" {
			src = s.AttrOr("src", "")
		}
		src = strings.TrimSpace(src)
		if src == "" {
			return
		}

		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
