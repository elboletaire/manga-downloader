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

// LelScans is a grabber for lelscans.net (French, a small custom reader). The
// series page lists every chapter in a dropdown, and the reader shows one page
// per URL (/scan-{slug}/{chapter}/{page}), so each page is fetched for its image.
type LelScans struct {
	*Grabber
}

// lelScansReaderRe matches a reader URL: a chapter, or one of its pages
var lelScansReaderRe = regexp.MustCompile(`^https?://[^/]+/scan-[^/]+/[\d.]+(?:/\d+)?/?(?:[?#].*)?$`)

func NewLelScans(g *Grabber) *LelScans {
	return &LelScans{Grabber: g}
}

// LelScansChapter represents a LelScans chapter
type LelScansChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a lelscans.net URL
func (m *LelScans) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	return strings.TrimPrefix(u.Hostname(), "www.") == "lelscans.net", nil
}

// resolveSeriesURL turns a reader URL into its series URL, read from the
// reader's breadcrumb (one fetch, made only for reader URLs)
func (m *LelScans) resolveSeriesURL() error {
	if !lelScansReaderRe.MatchString(m.URL) {
		return nil
	}

	body, err := http.Get(http.RequestParams{URL: m.URL})
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}
	href := doc.Find(`a[itemprop="url"][href*="/lecture-"]`).First().AttrOr("href", "")
	if href == "" {
		return errSeriesNotFound(m.URL)
	}
	m.URL = href

	return nil
}

// fetchDoc fetches a page of the site as a parsed document
func (m *LelScans) fetchDoc(pageURL string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: pageURL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// FetchTitle returns the series title, from its heading ("Lecture en ligne One Piece scan")
func (m *LelScans) FetchTitle() (string, error) {
	if err := m.resolveSeriesURL(); err != nil {
		return "", err
	}

	doc, err := m.fetchDoc(m.URL)
	if err != nil {
		return "", err
	}

	title := sanitizeTitle(doc.Find("h1").First().Text())
	title = strings.TrimSuffix(strings.TrimPrefix(title, "Lecture en ligne "), " scan")
	return title, nil
}

// FetchChapters returns the chapters listed in the series dropdown
func (m *LelScans) FetchChapters() (chapters Filterables, errs []error) {
	if err := m.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	doc, err := m.fetchDoc(m.URL)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find("select option").Each(func(_ int, s *goquery.Selection) {
		href := s.AttrOr("value", "")
		if !lelScansReaderRe.MatchString(href) {
			return
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(s.Text()), 64)
		if err != nil {
			return
		}

		chapters = append(chapters, &LelScansChapter{
			Chapter{
				Number: number,
				ID:     chapterURLID(href),
				Label:  Label{Style: LabelNumbered},
			},
			href,
		})
	})

	return
}

// FetchChapter fetches each page of the chapter's reader, whose image is the
// one in the reader's #image box, and the page count from its pager
func (m *LelScans) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*LelScansChapter)

	first, err := m.fetchDoc(chap.URL)
	if err != nil {
		return nil, err
	}

	pages := 1
	first.Find("#navigation a").Each(func(_ int, s *goquery.Selection) {
		if n, err := strconv.Atoi(strings.TrimSpace(s.Text())); err == nil && n > pages {
			pages = n
		}
	})

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "fr",
		ID:       chap.ID,
	}

	for page := 1; page <= pages; page++ {
		doc := first
		if page > 1 {
			doc, err = m.fetchDoc(chap.URL + "/" + strconv.Itoa(page))
			if err != nil {
				return nil, err
			}
		}

		src := strings.TrimSpace(doc.Find("#image img").First().AttrOr("src", ""))
		if src == "" {
			continue
		}
		if !strings.HasPrefix(src, "http") {
			src = m.BaseUrl() + src
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(page),
			URL:    src,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
