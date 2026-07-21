package grabber

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// LeerCapitulo is a grabber for leercapitulo.co (Spanish aggregator, popular
// with ex-TuMangaOnline readers). Its series/chapter-list pages are plain
// HTML, but the reader page hides the real page image URLs behind an
// obfuscated blob that's only decoded by the site's own javascript, and by
// default only decodes/shows one page at a time. A real browser is used just
// for the reader page: it toggles the site's "load every page at once"
// client-side preference (stored in localStorage, checked on load) before
// reading the resulting <img> tags, so no cipher reverse-engineering is
// needed.
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

	return sanitizeTitle(doc.Find("h1.title-manga").Text()), nil
}

// FetchChapters returns the chapters of the manga
func (m *LeerCapitulo) FetchChapters() (chapters Filterables, errs []error) {
	doc, err := m.fetchDoc()
	if err != nil {
		return nil, []error{err}
	}

	doc.Find(".chapter-list li.row").Each(func(i int, s *goquery.Selection) {
		a := s.Find("a.xanh")
		text := a.Text()
		number, ok := parseChapterNumber(text)
		if !ok {
			// section announcements and similar rows without a chapter number
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

// leerCapituloStorageKey/Value is the localStorage flag the reader's own
// javascript checks on load to decide whether to decode & display every page
// of the chapter at once ("Todo en uno") instead of one page at a time (the
// default "Uno por uno" mode). Without setting it, only the first page ever
// makes it into the DOM.
const (
	leerCapituloStorageKey    = "display_mode"
	leerCapituloStorageValue  = "1"
	leerCapituloImageSelector = ".comic_wraCon img"
)

// FetchChapter renders the chapter reader page in a real browser (the page
// images only get decoded client-side from an obfuscated blob) and extracts
// its pages
func (m *LeerCapitulo) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*LeerCapituloChapter)

	html, err := browser.GetHTMLWithLocalStorage(
		chap.URL, leerCapituloStorageKey, leerCapituloStorageValue,
		leerCapituloImageSelector, 0,
	)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "es",
	}

	doc.Find(leerCapituloImageSelector).Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("data-original", "")
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
